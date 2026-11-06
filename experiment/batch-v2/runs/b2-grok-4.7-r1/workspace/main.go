// redis-mini is a small RESP2 server implementing a subset of Redis 7.2
// string commands (PING, ECHO, SET, GET, DEL) with lazy key expiry.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	protoInlineMaxSize = 1024 * 64
	protoMaxBulkLen    = 512 * 1024 * 1024
	maxMultibulkLen    = 1 << 20
)

func main() {
	port := flag.Int("port", 6380, "TCP port to listen on")
	flag.Parse()

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	log.Printf("redis-mini listening on %s", ln.Addr())

	db := newStore()
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConn(conn, db)
	}
}

// ---------- key-value store (lazy expiry) ----------

type entry struct {
	value    []byte
	expireAt int64 // absolute unix milliseconds; 0 means no expiry
}

type store struct {
	mu   sync.Mutex
	data map[string]*entry
}

func newStore() *store {
	return &store{data: make(map[string]*entry)}
}

// lookup returns the live entry for key, deleting it if it has expired.
// now is unix milliseconds. Caller must hold s.mu.
func (s *store) lookup(key string, now int64) *entry {
	e, ok := s.data[key]
	if !ok {
		return nil
	}
	if e.expireAt > 0 && now > e.expireAt {
		delete(s.data, key)
		return nil
	}
	return e
}

func (s *store) get(key string, now int64) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.lookup(key, now)
	if e == nil {
		return nil, false
	}
	return append([]byte(nil), e.value...), true
}

func (s *store) set(key string, value []byte, expireAt int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = &entry{value: append([]byte(nil), value...), expireAt: expireAt}
}

func (s *store) del(keys []string, now int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if _, dup := seen[k]; dup {
			// A repeated key in one DEL is already gone after the first hit.
			continue
		}
		seen[k] = struct{}{}
		if s.lookup(k, now) == nil {
			continue
		}
		delete(s.data, k)
		n++
	}
	return n
}

// ---------- connection handling ----------

type protocolError string

func (e protocolError) Error() string { return string(e) }

func handleConn(conn net.Conn, db *store) {
	defer conn.Close()
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("client handler panic: %v", rec)
		}
	}()

	r := bufio.NewReaderSize(conn, 64*1024)
	w := bufio.NewWriterSize(conn, 64*1024)

	for {
		args, err := readCommand(r)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return
			}
			if pe, ok := err.(protocolError); ok {
				writeError(w, string(pe))
				_ = w.Flush()
				return
			}
			// Network error or truncated request: drop the connection.
			return
		}
		if len(args) == 0 {
			continue
		}
		execCommand(w, db, args)
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func execCommand(w *bufio.Writer, db *store, args [][]byte) {
	name := asciiLower(args[0])
	switch name {
	case "ping":
		cmdPing(w, args)
	case "echo":
		cmdEcho(w, args)
	case "set":
		cmdSet(w, db, args)
	case "get":
		cmdGet(w, db, args)
	case "del":
		cmdDel(w, db, args)
	default:
		writeError(w, unknownCommandMessage(args))
	}
}

func cmdPing(w *bufio.Writer, args [][]byte) {
	switch len(args) {
	case 1:
		writeSimple(w, "PONG")
	case 2:
		writeBulk(w, args[1])
	default:
		writeError(w, "wrong number of arguments for 'ping' command")
	}
}

func cmdEcho(w *bufio.Writer, args [][]byte) {
	if len(args) != 2 {
		writeError(w, "wrong number of arguments for 'echo' command")
		return
	}
	writeBulk(w, args[1])
}

func cmdGet(w *bufio.Writer, db *store, args [][]byte) {
	if len(args) != 2 {
		writeError(w, "wrong number of arguments for 'get' command")
		return
	}
	val, ok := db.get(string(args[1]), time.Now().UnixMilli())
	if !ok {
		writeNullBulk(w)
		return
	}
	writeBulk(w, val)
}

func cmdDel(w *bufio.Writer, db *store, args [][]byte) {
	if len(args) < 2 {
		writeError(w, "wrong number of arguments for 'del' command")
		return
	}
	keys := make([]string, len(args)-1)
	for i, a := range args[1:] {
		keys[i] = string(a)
	}
	n := db.del(keys, time.Now().UnixMilli())
	writeInt(w, n)
}

func cmdSet(w *bufio.Writer, db *store, args [][]byte) {
	if len(args) < 3 {
		writeError(w, "wrong number of arguments for 'set' command")
		return
	}
	expireRaw, unitMS, err := parseSetArgs(args)
	if err != nil {
		writeError(w, err.Error())
		return
	}
	var expireAt int64
	if expireRaw != nil {
		at, err := expireMilliseconds(expireRaw, unitMS, time.Now().UnixMilli())
		if err != nil {
			writeError(w, err.Error())
			return
		}
		expireAt = at
	}
	db.set(string(args[1]), args[2], expireAt)
	writeSimple(w, "OK")
}

// parseSetArgs accepts SET key value [EX seconds | PX milliseconds], repeated
// options of the same unit (last wins), matching Redis. Any other option,
// a missing option argument, or mixing EX and PX is a syntax error.
// A nil expireRaw means no expiry was requested.
func parseSetArgs(args [][]byte) (expireRaw []byte, unitMS bool, err error) {
	hasEX, hasPX := false, false
	for i := 3; i < len(args); i++ {
		opt := args[i]
		var next []byte
		hasNext := i+1 < len(args)
		if hasNext {
			next = args[i+1]
		}
		switch {
		case eqFoldASCII(opt, "EX") && !hasPX && hasNext:
			hasEX = true
			expireRaw = next
			unitMS = false
			i++
		case eqFoldASCII(opt, "PX") && !hasEX && hasNext:
			hasPX = true
			expireRaw = next
			unitMS = true
			i++
		default:
			return nil, false, protoErr("syntax error")
		}
	}
	return expireRaw, unitMS, nil
}

type cmdError string

func (e cmdError) Error() string { return string(e) }

func protoErr(msg string) cmdError { return cmdError(msg) }

// expireMilliseconds converts a user-supplied TTL into an absolute unix-ms
// timestamp, following Redis getExpireMillisecondsOrReply for EX/PX.
func expireMilliseconds(raw []byte, unitMS bool, now int64) (int64, error) {
	v, ok := string2ll(string(raw))
	if !ok {
		return 0, protoErr("value is not an integer or out of range")
	}
	if v <= 0 || (!unitMS && v > math.MaxInt64/1000) {
		return 0, protoErr("invalid expire time in 'set' command")
	}
	ms := v
	if !unitMS {
		ms = v * 1000
	}
	if ms > math.MaxInt64-now {
		return 0, protoErr("invalid expire time in 'set' command")
	}
	ms += now
	if ms <= 0 {
		return 0, protoErr("invalid expire time in 'set' command")
	}
	return ms, nil
}

func unknownCommandMessage(args [][]byte) string {
	// Redis 7: unknown command `<name>`, with args beginning with: `<a>`, `<b>`,
	var b strings.Builder
	b.WriteString("unknown command `")
	b.Write(sanitize(args[0]))
	b.WriteString("`, with args beginning with: ")
	accLen := 0
	for i := 1; i < len(args) && accLen < 128; i++ {
		// "`%.*s`, " with precision 128-accLen, matching sdscatprintf usage.
		remain := 128 - accLen
		chunk := args[i]
		if len(chunk) > remain {
			chunk = chunk[:remain]
		}
		b.WriteByte('`')
		b.Write(sanitize(chunk))
		b.WriteString("`, ")
		accLen += 1 + len(chunk) + 3
	}
	return b.String()
}

func sanitize(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if c == '\r' || c == '\n' {
			out[i] = ' '
		} else {
			out[i] = c
		}
	}
	return out
}

// ---------- replies ----------

func writeSimple(w *bufio.Writer, s string) {
	w.WriteString("+")
	w.WriteString(s)
	w.WriteString("\r\n")
}

func writeError(w *bufio.Writer, msg string) {
	// addReplyError prefixes "-ERR " unless the caller already supplied a code.
	w.WriteString("-ERR ")
	w.Write(sanitize([]byte(msg)))
	w.WriteString("\r\n")
}

func writeInt(w *bufio.Writer, n int64) {
	w.WriteByte(':')
	w.WriteString(strconv.FormatInt(n, 10))
	w.WriteString("\r\n")
}

func writeBulk(w *bufio.Writer, b []byte) {
	w.WriteByte('$')
	w.WriteString(strconv.Itoa(len(b)))
	w.WriteString("\r\n")
	w.Write(b)
	w.WriteString("\r\n")
}

func writeNullBulk(w *bufio.Writer) {
	w.WriteString("$-1\r\n")
}

// ---------- RESP reader ----------

func readCommand(r *bufio.Reader) ([][]byte, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if b == '*' {
		return readMultibulk(r)
	}
	if err := r.UnreadByte(); err != nil {
		return nil, err
	}
	return readInline(r)
}

func readMultibulk(r *bufio.Reader) ([][]byte, error) {
	line, err := readCRLFLine(r, protoInlineMaxSize, "Protocol error: too big mbulk count string")
	if err != nil {
		return nil, err
	}
	n, ok := string2ll(line)
	if !ok || n > math.MaxInt32 || n > maxMultibulkLen {
		return nil, protocolError("Protocol error: invalid multibulk length")
	}
	// Redis ignores a non-positive multibulk length (no reply).
	if n <= 0 {
		return nil, nil
	}
	args := make([][]byte, 0, n)
	for i := int64(0); i < n; i++ {
		prefix, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if prefix != '$' {
			return nil, protocolError(fmt.Sprintf("Protocol error: expected '$', got '%c'", prefix))
		}
		blen, err := readCRLFLine(r, protoInlineMaxSize, "Protocol error: too big bulk count string")
		if err != nil {
			return nil, err
		}
		ll, ok := string2ll(blen)
		if !ok || ll < 0 || ll > protoMaxBulkLen {
			return nil, protocolError("Protocol error: invalid bulk length")
		}
		buf := make([]byte, ll)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		cr, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		lf, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if cr != '\r' || lf != '\n' {
			return nil, protocolError("Protocol error: invalid bulk string terminator")
		}
		args = append(args, buf)
	}
	return args, nil
}

func readInline(r *bufio.Reader) ([][]byte, error) {
	line, err := readInlineLine(r)
	if err != nil {
		return nil, err
	}
	args, err := splitInlineArgs(line)
	if err != nil {
		return nil, protocolError("Protocol error: unbalanced quotes in request")
	}
	return args, nil
}

// readCRLFLine reads up to and including CRLF and returns the line without it.
// max is the maximum number of bytes allowed before the CRLF (Redis rejects
// an unterminated count string longer than PROTO_INLINE_MAX_SIZE).
func readCRLFLine(r *bufio.Reader, max int, tooBig string) (string, error) {
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '\n' {
			if len(buf) == 0 || buf[len(buf)-1] != '\r' {
				return "", protocolError("Protocol error: invalid multibulk length")
			}
			return string(buf[:len(buf)-1]), nil
		}
		buf = append(buf, b)
		if len(buf) > max {
			return "", protocolError(tooBig)
		}
	}
}

func readInlineLine(r *bufio.Reader) (string, error) {
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '\n' {
			if len(buf) > 0 && buf[len(buf)-1] == '\r' {
				buf = buf[:len(buf)-1]
			}
			return string(buf), nil
		}
		buf = append(buf, b)
		if len(buf) > protoInlineMaxSize {
			return "", protocolError("Protocol error: too big inline request")
		}
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// splitInlineArgs is a port of Redis sdssplitargs.
func splitInlineArgs(line string) ([][]byte, error) {
	p := 0
	n := len(line)
	var argv [][]byte
	for {
		for p < n && isSpace(line[p]) {
			p++
		}
		if p >= n {
			return argv, nil
		}
		var current []byte
		inq, insq := false, false
		done := false
		for !done {
			if p >= n {
				if inq || insq {
					return nil, fmt.Errorf("unbalanced")
				}
				break
			}
			c := line[p]
			if inq {
				if c == '\\' && p+3 < n && line[p+1] == 'x' && isHex(line[p+2]) && isHex(line[p+3]) {
					current = append(current, hexVal(line[p+2])*16+hexVal(line[p+3]))
					p += 3
				} else if c == '\\' && p+1 < n {
					p++
					var ch byte
					switch line[p] {
					case 'n':
						ch = '\n'
					case 'r':
						ch = '\r'
					case 't':
						ch = '\t'
					case 'b':
						ch = '\b'
					case 'a':
						ch = '\a'
					default:
						ch = line[p]
					}
					current = append(current, ch)
				} else if c == '"' {
					if p+1 < n && !isSpace(line[p+1]) {
						return nil, fmt.Errorf("unbalanced")
					}
					done = true
				} else {
					current = append(current, c)
				}
			} else if insq {
				if c == '\\' && p+1 < n && line[p+1] == '\'' {
					p++
					current = append(current, '\'')
				} else if c == '\'' {
					if p+1 < n && !isSpace(line[p+1]) {
						return nil, fmt.Errorf("unbalanced")
					}
					done = true
				} else {
					current = append(current, c)
				}
			} else {
				switch c {
				case ' ', '\n', '\r', '\t', '\f', '\v':
					done = true
				case '"':
					inq = true
				case '\'':
					insq = true
				default:
					current = append(current, c)
				}
			}
			if p < n {
				p++
			}
		}
		argv = append(argv, current)
	}
}

// string2ll ports Redis string2ll: no leading '+', no leading zeros, base 10.
func string2ll(s string) (int64, bool) {
	if len(s) == 0 {
		return 0, false
	}
	if len(s) == 1 && s[0] == '0' {
		return 0, true
	}
	i := 0
	neg := false
	if s[0] == '-' {
		neg = true
		i++
		if i == len(s) {
			return 0, false
		}
	}
	if s[i] < '1' || s[i] > '9' {
		return 0, false
	}
	var v uint64
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		d := uint64(s[i] - '0')
		if v > math.MaxUint64/10 {
			return 0, false
		}
		v *= 10
		if v > math.MaxUint64-d {
			return 0, false
		}
		v += d
	}
	if neg {
		// LLONG_MIN == -9223372036854775808, whose magnitude is 1<<63.
		if v > 1<<63 {
			return 0, false
		}
		if v == 1<<63 {
			return math.MinInt64, true
		}
		return -int64(v), true
	}
	if v > uint64(math.MaxInt64) {
		return 0, false
	}
	return int64(v), true
}

func asciiLower(b []byte) string {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

func eqFoldASCII(b []byte, upper string) bool {
	if len(b) != len(upper) {
		return false
	}
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c != upper[i] {
			return false
		}
	}
	return true
}
