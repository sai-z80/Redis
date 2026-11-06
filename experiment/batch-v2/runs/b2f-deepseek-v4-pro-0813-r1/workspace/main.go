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
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPort        = 6380
	maxBulkLength      = 512 * 1024 * 1024 // Redis default proto-max-bulk-len
	maxMultibulkLength = 1024 * 1024
)

type entry struct {
	data     []byte
	expireAt int64 // Unix milliseconds; 0 means no expiry
}

type store struct {
	mu sync.RWMutex
	m  map[string]entry
}

func newStore() *store {
	return &store{m: make(map[string]entry)}
}

// expired reports whether the entry has an expiration time in the past. It
// does not mutate the store.
func expired(e entry, nowMs int64) bool {
	return e.expireAt != 0 && nowMs > e.expireAt
}

// lookup returns the entry for key, removing it if it is expired. The second
// result is false when the key is missing or expired.
func (s *store) lookup(key string, nowMs int64) (entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	if !ok {
		return entry{}, false
	}
	if expired(e, nowMs) {
		delete(s.m, key)
		return entry{}, false
	}
	return e, true
}

func (s *store) set(key string, data []byte, expireAt int64) {
	cp := make([]byte, len(data))
	copy(cp, data)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = entry{data: cp, expireAt: expireAt}
}

func (s *store) delete(key string, nowMs int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	if !ok {
		return 0
	}
	if expired(e, nowMs) {
		delete(s.m, key)
		return 0
	}
	delete(s.m, key)
	return 1
}

var (
	db     = newStore()
	execMu sync.Mutex // serializes command execution, like Redis's single thread
)

func sanitizeErr(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func errReply(msg string) []byte {
	return []byte("-ERR " + sanitizeErr(msg) + "\r\n")
}

func simpleReply(s string) []byte {
	return []byte("+" + sanitizeErr(s) + "\r\n")
}

func intReply(n int64) []byte {
	return []byte(":" + strconv.FormatInt(n, 10) + "\r\n")
}

func bulkReply(b []byte) []byte {
	if b == nil {
		return []byte("$-1\r\n")
	}
	var out []byte
	out = append(out, '$')
	out = strconv.AppendInt(out, int64(len(b)), 10)
	out = append(out, '\r', '\n')
	out = append(out, b...)
	out = append(out, '\r', '\n')
	return out
}

func nullBulkReply() []byte {
	return []byte("$-1\r\n")
}

func wrongArityReply(cmd string) []byte {
	return errReply(fmt.Sprintf("wrong number of arguments for '%s' command", sanitizeErr(cmd)))
}

func syntaxReply() []byte {
	return errReply("syntax error")
}

func invalidExpireReply(cmd string) []byte {
	return errReply(fmt.Sprintf("invalid expire time in '%s' command", sanitizeErr(cmd)))
}

func integerReply() []byte {
	return errReply("value is not an integer or out of range")
}

func unknownCommandReply(name string, args [][]byte) []byte {
	msg := "unknown command '" + sanitizeErr(name) + "'"
	if len(args) > 1 {
		msg += ", with args beginning with: "
		for i, a := range args[1:] {
			if i > 0 {
				msg += ", "
			}
			msg += "'" + sanitizeErr(string(a)) + "'"
		}
	}
	return errReply(msg)
}

func execute(args [][]byte) []byte {
	if len(args) == 0 {
		return errReply("empty command")
	}
	cmd := strings.ToLower(string(args[0]))

	switch cmd {
	case "ping":
		switch len(args) {
		case 1:
			return simpleReply("PONG")
		case 2:
			return bulkReply(args[1])
		default:
			return wrongArityReply(cmd)
		}
	case "echo":
		if len(args) != 2 {
			return wrongArityReply(cmd)
		}
		return bulkReply(args[1])
	case "get":
		if len(args) != 2 {
			return wrongArityReply(cmd)
		}
		key := string(args[1])
		nowMs := time.Now().UnixMilli()
		e, ok := db.lookup(key, nowMs)
		if !ok {
			return nullBulkReply()
		}
		return bulkReply(e.data)
	case "set":
		if len(args) < 3 {
			return wrongArityReply(cmd)
		}
		return setCommand(args)
	case "del":
		if len(args) < 2 {
			return wrongArityReply(cmd)
		}
		nowMs := time.Now().UnixMilli()
		seen := make(map[string]struct{}, len(args)-1)
		var n int64
		for _, a := range args[1:] {
			key := string(a)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			n += int64(db.delete(key, nowMs))
		}
		return intReply(n)
	default:
		return unknownCommandReply(string(args[0]), args)
	}
}

func setCommand(args [][]byte) []byte {
	key := string(args[1])
	value := args[2]

	// Mirror Redis parseExtendedStringArgumentsOrReply for the subset of SET
	// options that is in scope: EX and PX. As in Redis, the last EX (or PX)
	// argument wins when the same option is given more than once.
	expireUnit := 0 // 0 = no expire, 1 = EX, 2 = PX
	var expireArg []byte

	for i := 3; i < len(args); i++ {
		opt := strings.ToUpper(string(args[i]))
		switch opt {
		case "EX":
			// EX cannot follow PX in Redis's SET option parser.
			if expireUnit == 2 {
				return syntaxReply()
			}
			if i+1 >= len(args) {
				return syntaxReply()
			}
			expireArg = args[i+1]
			expireUnit = 1
			i++
		case "PX":
			// PX cannot follow EX in Redis's SET option parser.
			if expireUnit == 1 {
				return syntaxReply()
			}
			if i+1 >= len(args) {
				return syntaxReply()
			}
			expireArg = args[i+1]
			expireUnit = 2
			i++
		default:
			// Only EX and PX are in scope for this subset. Every other SET
			// option (NX, XX, GET, KEEPTTL, EXAT, PXAT, and unknown tokens)
			// is a syntax error here.
			return syntaxReply()
		}
	}

	var ttlMs int64
	if expireUnit != 0 {
		n, ok := parseTTL(expireArg)
		if !ok {
			return integerReply()
		}
		if n <= 0 {
			return invalidExpireReply("set")
		}
		if expireUnit == 1 {
			// Redis rejects EX values that cannot be converted to
			// milliseconds without overflowing a signed 64-bit value.
			if n > math.MaxInt64/1000 {
				return invalidExpireReply("set")
			}
			ttlMs = n * 1000
		} else {
			ttlMs = n
		}
	}

	nowMs := time.Now().UnixMilli()
	// Expire the old key before overwriting, as Redis does for write lookups.
	db.lookup(key, nowMs)

	expireAt := int64(0)
	if expireUnit != 0 {
		if ttlMs > math.MaxInt64-nowMs {
			return invalidExpireReply("set")
		}
		expireAt = nowMs + ttlMs
		if expireAt <= 0 {
			return invalidExpireReply("set")
		}
	}

	db.set(key, value, expireAt)
	return simpleReply("OK")
}

func parseTTL(arg []byte) (int64, bool) {
	if len(arg) == 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(string(arg), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, errors.New("protocol error: expected CRLF")
	}
	return []byte(line[:len(line)-2]), nil
}

func readCommand(r *bufio.Reader) ([][]byte, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if len(line) == 0 || line[0] != '*' {
		return nil, errors.New("protocol error: expected multibulk request")
	}
	count, err := strconv.Atoi(string(line[1:]))
	if err != nil || count <= 0 || count > maxMultibulkLength {
		return nil, errors.New("protocol error: invalid multibulk length")
	}

	args := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		line, err = readLine(r)
		if err != nil {
			return nil, err
		}
		if len(line) == 0 || line[0] != '$' {
			return nil, errors.New("protocol error: expected bulk string")
		}
		n, err := strconv.Atoi(string(line[1:]))
		if err != nil || n < 0 || n > maxBulkLength {
			return nil, errors.New("protocol error: invalid bulk length")
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		if buf[n] != '\r' || buf[n+1] != '\n' {
			return nil, errors.New("protocol error: bulk string not terminated by CRLF")
		}
		args = append(args, buf[:n])
	}
	return args, nil
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	for {
		args, err := readCommand(r)
		if err != nil {
			if err != io.EOF {
				// Redis emits a protocol error and closes the connection on
				// malformed RESP input.
				w.Write(errReply("Protocol error: " + err.Error()))
				w.Flush()
			}
			return
		}
		execMu.Lock()
		reply := execute(args)
		execMu.Unlock()
		if _, err := w.Write(reply); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func main() {
	port := flag.Int("port", defaultPort, "TCP port to listen on")
	flag.Parse()

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen on %s: %v", addr, err)
	}
	log.Printf("redis-mini listening on %s", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConn(conn)
	}
}
