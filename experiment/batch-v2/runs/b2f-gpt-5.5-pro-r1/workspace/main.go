package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type entry struct {
	value    []byte
	expireAt int64 // absolute unix time in milliseconds; 0 means no expiry
}

type database struct {
	mu   sync.Mutex
	data map[string]entry
}

func newDatabase() *database {
	return &database{data: make(map[string]entry)}
}

func nowMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func (db *database) get(key []byte) ([]byte, bool) {
	ks := string(key)

	db.mu.Lock()
	defer db.mu.Unlock()
	now := nowMillis()

	e, ok := db.data[ks]
	if !ok {
		return nil, false
	}
	if e.expireAt > 0 && e.expireAt <= now {
		delete(db.data, ks)
		return nil, false
	}
	v := make([]byte, len(e.value))
	copy(v, e.value)
	return v, true
}

func (db *database) set(key, value []byte, expireAt int64) {
	ks := string(key)
	v := make([]byte, len(value))
	copy(v, value)

	db.mu.Lock()
	db.data[ks] = entry{value: v, expireAt: expireAt}
	db.mu.Unlock()
}

func (db *database) del(keys [][]byte) int64 {
	var deleted int64

	db.mu.Lock()
	defer db.mu.Unlock()
	now := nowMillis()

	for _, key := range keys {
		ks := string(key)
		e, ok := db.data[ks]
		if !ok {
			continue
		}
		if e.expireAt > 0 && e.expireAt <= now {
			delete(db.data, ks)
			continue
		}
		delete(db.data, ks)
		deleted++
	}
	return deleted
}

type server struct {
	db *database
}

type protocolError struct {
	msg string
}

func (e protocolError) Error() string { return e.msg }

func main() {
	log.SetFlags(0)
	log.SetOutput(os.Stderr)

	port := flag.Int("port", 6380, "TCP port to listen on")
	flag.Parse()

	if *port < 1 || *port > 65535 {
		log.Fatalf("invalid port %d", *port)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	defer ln.Close()

	s := &server{db: newDatabase()}
	for {
		conn, err := ln.Accept()
		if err != nil {
			// A closed listener is the only expected permanent error. Otherwise keep
			// the process alive like Redis does for transient accept failures.
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("accept: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *server) handleConn(conn net.Conn) {
	defer conn.Close()

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	for {
		argv, err := readRequest(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			var pe protocolError
			if errors.As(err, &pe) {
				_ = writeError(w, "ERR "+pe.msg)
				_ = w.Flush()
			}
			return
		}

		// Redis accepts empty / null multibulk requests by doing nothing.
		if len(argv) == 0 {
			continue
		}

		if err := s.dispatch(w, argv); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func readRequest(r *bufio.Reader) ([][]byte, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if b != '*' {
		return nil, protocolError{msg: fmt.Sprintf("Protocol error: expected '*', got '%c'", printableByte(b))}
	}

	line, err := readLineAfterFirstByte(r, b)
	if err != nil {
		return nil, err
	}
	n, ok := parseRespInt(line[1:])
	if !ok || n > math.MaxInt32 {
		return nil, protocolError{msg: "Protocol error: invalid multibulk length"}
	}
	if n <= 0 {
		return [][]byte{}, nil
	}

	argvCap := n
	if argvCap > 1024 {
		argvCap = 1024
	}
	argv := make([][]byte, 0, int(argvCap))
	for i := int64(0); i < n; i++ {
		prefix, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if prefix != '$' {
			return nil, protocolError{msg: fmt.Sprintf("Protocol error: expected '$', got '%c'", printableByte(prefix))}
		}
		line, err := readLineAfterFirstByte(r, prefix)
		if err != nil {
			return nil, err
		}
		bulkLen, ok := parseRespInt(line[1:])
		if !ok || bulkLen < 0 {
			return nil, protocolError{msg: "Protocol error: invalid bulk length"}
		}
		if bulkLen > int64(maxBulkLen()) {
			return nil, protocolError{msg: "Protocol error: invalid bulk length"}
		}

		buf := make([]byte, bulkLen+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		if buf[bulkLen] != '\r' || buf[bulkLen+1] != '\n' {
			return nil, protocolError{msg: "Protocol error: invalid bulk length"}
		}
		arg := make([]byte, bulkLen)
		copy(arg, buf[:bulkLen])
		argv = append(argv, arg)
	}
	return argv, nil
}

func maxBulkLen() int {
	// Redis' default proto-max-bulk-len is 512 MiB. Keeping the same ceiling
	// prevents accidental huge allocations while remaining far beyond the tests.
	return 512 * 1024 * 1024
}

func readLineAfterFirstByte(r *bufio.Reader, first byte) ([]byte, error) {
	rest, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(rest) < 2 || rest[len(rest)-2] != '\r' {
		return nil, protocolError{msg: "Protocol error: expected CRLF"}
	}
	line := make([]byte, 1+len(rest)-2)
	line[0] = first
	copy(line[1:], rest[:len(rest)-2])
	return line, nil
}

func parseRespInt(b []byte) (int64, bool) {
	// RESP lengths in Redis are parsed with string2ll: no whitespace, no '+',
	// and no leading zeroes except for the number zero itself.
	return parseRedisInt64(b)
}

func parseRedisInt64(b []byte) (int64, bool) {
	if len(b) == 0 {
		return 0, false
	}
	if len(b) == 1 && b[0] == '0' {
		return 0, true
	}

	neg := false
	i := 0
	var limit uint64 = math.MaxInt64
	if b[0] == '-' {
		neg = true
		i = 1
		limit = uint64(1) << 63
		if i == len(b) {
			return 0, false
		}
	}
	if b[i] < '1' || b[i] > '9' {
		return 0, false
	}

	var v uint64
	for ; i < len(b); i++ {
		if b[i] < '0' || b[i] > '9' {
			return 0, false
		}
		d := uint64(b[i] - '0')
		if v > (limit-d)/10 {
			return 0, false
		}
		v = v*10 + d
	}

	if neg {
		if v == uint64(1)<<63 {
			return math.MinInt64, true
		}
		return -int64(v), true
	}
	return int64(v), true
}

func printableByte(b byte) byte {
	if b >= 32 && b <= 126 {
		return b
	}
	return '?'
}

func (s *server) dispatch(w *bufio.Writer, argv [][]byte) error {
	cmd := commandName(argv[0])

	switch cmd {
	case "PING":
		return s.cmdPing(w, argv)
	case "ECHO":
		return s.cmdEcho(w, argv)
	case "SET":
		return s.cmdSet(w, argv)
	case "GET":
		return s.cmdGet(w, argv)
	case "DEL":
		return s.cmdDel(w, argv)
	default:
		return writeError(w, unknownCommandMessage(argv))
	}
}

func commandName(b []byte) string {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

func optionEquals(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c != s[i] {
			return false
		}
	}
	return true
}

func (s *server) cmdPing(w *bufio.Writer, argv [][]byte) error {
	switch len(argv) {
	case 1:
		return writeSimple(w, "PONG")
	case 2:
		return writeBulk(w, argv[1])
	default:
		return writeArityError(w, "ping")
	}
}

func (s *server) cmdEcho(w *bufio.Writer, argv [][]byte) error {
	if len(argv) != 2 {
		return writeArityError(w, "echo")
	}
	return writeBulk(w, argv[1])
}

func (s *server) cmdSet(w *bufio.Writer, argv [][]byte) error {
	if len(argv) < 3 {
		return writeArityError(w, "set")
	}

	expireAt := int64(0)
	if len(argv) != 3 {
		if len(argv) != 5 {
			return writeSyntaxError(w)
		}
		if !optionEquals(argv[3], "EX") && !optionEquals(argv[3], "PX") {
			return writeSyntaxError(w)
		}

		ttl, ok := parseRedisInt64(argv[4])
		if !ok {
			return writeError(w, "ERR value is not an integer or out of range")
		}

		var ttlMillis int64
		if optionEquals(argv[3], "EX") {
			if ttl <= 0 || ttl > math.MaxInt64/1000 {
				return writeInvalidExpireTime(w, "set")
			}
			ttlMillis = ttl * 1000
		} else {
			if ttl <= 0 {
				return writeInvalidExpireTime(w, "set")
			}
			ttlMillis = ttl
		}

		now := nowMillis()
		if ttlMillis > math.MaxInt64-now {
			return writeInvalidExpireTime(w, "set")
		}
		expireAt = now + ttlMillis
		if expireAt <= 0 {
			return writeInvalidExpireTime(w, "set")
		}
	}

	s.db.set(argv[1], argv[2], expireAt)
	return writeSimple(w, "OK")
}

func (s *server) cmdGet(w *bufio.Writer, argv [][]byte) error {
	if len(argv) != 2 {
		return writeArityError(w, "get")
	}
	if v, ok := s.db.get(argv[1]); ok {
		return writeBulk(w, v)
	}
	return writeNullBulk(w)
}

func (s *server) cmdDel(w *bufio.Writer, argv [][]byte) error {
	if len(argv) < 2 {
		return writeArityError(w, "del")
	}
	return writeInteger(w, s.db.del(argv[1:]))
}

func writeSimple(w *bufio.Writer, s string) error {
	_, err := w.WriteString("+" + s + "\r\n")
	return err
}

func writeError(w *bufio.Writer, msg string) error {
	msg = sanitizeError(msg)
	_, err := w.WriteString("-" + msg + "\r\n")
	return err
}

func writeArityError(w *bufio.Writer, cmd string) error {
	return writeError(w, fmt.Sprintf("ERR wrong number of arguments for '%s' command", cmd))
}

func writeInvalidExpireTime(w *bufio.Writer, cmd string) error {
	return writeError(w, fmt.Sprintf("ERR invalid expire time in '%s' command", cmd))
}

func writeSyntaxError(w *bufio.Writer) error {
	return writeError(w, "ERR syntax error")
}

func writeInteger(w *bufio.Writer, n int64) error {
	_, err := w.WriteString(":" + strconv.FormatInt(n, 10) + "\r\n")
	return err
}

func writeBulk(w *bufio.Writer, data []byte) error {
	if _, err := w.WriteString("$" + strconv.Itoa(len(data)) + "\r\n"); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	_, err := w.WriteString("\r\n")
	return err
}

func writeNullBulk(w *bufio.Writer) error {
	_, err := w.WriteString("$-1\r\n")
	return err
}

func sanitizeError(msg string) string {
	// Redis maps CR/LF inside error strings to spaces so it never emits invalid
	// RESP. It also avoids duplicated CRLF suffixes.
	msg = strings.TrimRight(msg, "\r\n")
	msg = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return ' '
		}
		return r
	}, msg)
	return msg
}

func unknownCommandMessage(argv [][]byte) string {
	cmd := string(argv[0])
	cmd = sanitizeError(cmd)

	// Match Redis' general shape while keeping the message bounded and safe for
	// binary arguments.
	var b strings.Builder
	fmt.Fprintf(&b, "ERR unknown command '%s', with args beginning with: ", cmd)
	limit := len(argv) - 1
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		arg := sanitizeError(string(argv[i+1]))
		if len(arg) > 128 {
			arg = arg[:128]
		}
		fmt.Fprintf(&b, "'%s' ", arg)
	}
	return b.String()
}
