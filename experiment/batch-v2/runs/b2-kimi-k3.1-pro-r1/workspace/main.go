// Command redis-mini implements the small, string-only Redis subset described
// in this repository's task. It intentionally has no persistence or active
// expiration goroutine: expirations are checked only when a key is accessed.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// entry.expiresAt is an absolute Unix time in milliseconds. A zero value means
// that the entry is persistent. Valid expiration times are always positive.
type entry struct {
	value     string
	expiresAt int64
}

type database struct {
	mu      sync.Mutex
	entries map[string]entry
}

type server struct {
	db database
}

func newServer() *server {
	return &server{db: database{entries: make(map[string]entry)}}
}

func main() {
	port := flag.Int("port", 6380, "TCP port to listen on")
	flag.Parse()

	if *port < 0 || *port > 65535 {
		fmt.Fprintln(os.Stderr, "invalid port: must be between 0 and 65535")
		os.Exit(2)
	}

	addr := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", *port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen on %s: %v", addr, err)
	}
	defer listener.Close()

	s := newServer()
	log.Printf("redis-mini listening on %s", listener.Addr())
	for {
		conn, err := listener.Accept()
		if err != nil {
			// There is no shutdown command in this subset. An accept failure is
			// therefore fatal rather than something the server can usefully
			// recover from without risking a busy loop.
			log.Fatalf("accept: %v", err)
		}
		go s.serveConn(conn)
	}
}

// execute runs one already-decoded command and writes its single RESP reply.
// All database operations for a command happen under one lock, so DEL with
// multiple keys and SET overwrite/TTL changes are atomic with respect to other
// clients.
func (s *server) execute(args [][]byte, w *respWriter) error {
	if len(args) == 0 {
		return nil
	}

	name := strings.ToUpper(string(args[0]))
	switch name {
	case "PING":
		if len(args) > 2 {
			w.writeError("wrong number of arguments for 'ping' command")
			return w.Flush()
		}
		if len(args) == 2 {
			w.writeBulk(args[1])
		} else {
			w.writeSimple("PONG")
		}

	case "ECHO":
		if len(args) != 2 {
			w.writeError("wrong number of arguments for 'echo' command")
			return w.Flush()
		}
		w.writeBulk(args[1])

	case "GET":
		if len(args) != 2 {
			w.writeError("wrong number of arguments for 'get' command")
			return w.Flush()
		}
		key := string(args[1])
		now := time.Now().UnixMilli()

		s.db.mu.Lock()
		e, ok := s.db.entries[key]
		if ok && expired(e, now) {
			delete(s.db.entries, key)
			ok = false
		}
		s.db.mu.Unlock()

		if !ok {
			w.writeNullBulk()
		} else {
			w.writeBulkString(e.value)
		}

	case "SET":
		if len(args) < 3 {
			w.writeError("wrong number of arguments for 'set' command")
			return w.Flush()
		}

		// Parse every option before touching the database. As in Redis's
		// extended SET parser, a repeated option of the same unit replaces the
		// earlier value, while mixing EX and PX is a syntax error.
		const (
			unitNone = iota
			unitEX
			unitPX
		)
		unit := unitNone
		var expireArg []byte
		for i := 3; i < len(args); i++ {
			option := strings.ToUpper(string(args[i]))
			switch option {
			case "EX":
				if unit == unitPX || i+1 >= len(args) {
					w.writeError("syntax error")
					return w.Flush()
				}
				unit = unitEX
				expireArg = args[i+1]
				i++
			case "PX":
				if unit == unitEX || i+1 >= len(args) {
					w.writeError("syntax error")
					return w.Flush()
				}
				unit = unitPX
				expireArg = args[i+1]
				i++
			default:
				// This also rejects the explicitly unsupported NX, XX, GET,
				// KEEPTTL, EXAT and PXAT options.
				w.writeError("syntax error")
				return w.Flush()
			}
		}

		var expiresAt int64
		if unit != unitNone {
			n, ok := parseRedisInt64(expireArg)
			if !ok {
				w.writeError("value is not an integer or out of range")
				return w.Flush()
			}

			// Redis rejects non-positive durations and every overflow while
			// converting EX to milliseconds or adding the current time.
			now := time.Now().UnixMilli()
			if n <= 0 || (unit == unitEX && n > maxInt64/1000) {
				w.writeError("invalid expire time in 'set' command")
				return w.Flush()
			}
			if unit == unitEX {
				n *= 1000
			}
			if n > maxInt64-now {
				w.writeError("invalid expire time in 'set' command")
				return w.Flush()
			}
			expiresAt = now + n
			if expiresAt <= 0 {
				w.writeError("invalid expire time in 'set' command")
				return w.Flush()
			}
		}

		key := string(args[1])
		value := string(args[2])
		s.db.mu.Lock()
		// Assignment both overwrites the value and clears an old TTL when no
		// new expiration was supplied. It also replaces an expired entry.
		s.db.entries[key] = entry{value: value, expiresAt: expiresAt}
		s.db.mu.Unlock()
		w.writeSimple("OK")

	case "DEL":
		if len(args) < 2 {
			w.writeError("wrong number of arguments for 'del' command")
			return w.Flush()
		}
		now := time.Now().UnixMilli()
		deleted := 0

		s.db.mu.Lock()
		for _, rawKey := range args[1:] {
			key := string(rawKey)
			e, ok := s.db.entries[key]
			if !ok {
				continue
			}
			delete(s.db.entries, key)
			if !expired(e, now) {
				deleted++
			}
		}
		s.db.mu.Unlock()
		w.writeInteger(int64(deleted))

	default:
		w.writeError(unknownCommandMessage(args))
	}
	return w.Flush()
}

func expired(e entry, now int64) bool {
	// Redis's lazy lookup considers a key expired once now is strictly greater
	// than its absolute expiration time.
	return e.expiresAt != 0 && now > e.expiresAt
}

const maxInt64 = int64(^uint64(0) >> 1)

// parseRedisInt64 matches Redis's string2ll syntax: a canonical decimal integer
// with no leading '+', no surrounding whitespace, and no leading zeroes (except
// the single digit "0"). Both malformed and overflowing values return false.
func parseRedisInt64(b []byte) (int64, bool) {
	if len(b) == 0 {
		return 0, false
	}
	if len(b) == 1 && b[0] == '0' {
		return 0, true
	}

	i := 0
	negative := false
	if b[0] == '-' {
		negative = true
		i++
		if i == len(b) {
			return 0, false
		}
	}
	if b[i] < '1' || b[i] > '9' {
		return 0, false
	}

	// Accumulating negatively lets MinInt64 be represented without a
	// intermediate positive overflow.
	var n int64
	for ; i < len(b); i++ {
		c := b[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		digit := int64(c - '0')
		if n < (minInt64+digit)/10 {
			return 0, false
		}
		n = n*10 - digit
	}
	if !negative {
		if n == minInt64 {
			return 0, false
		}
		n = -n
	}
	return n, true
}

const minInt64 = -maxInt64 - 1

func unknownCommandMessage(args [][]byte) string {
	// Keep dynamic input on one RESP line and bound the error size. The format
	// and quoting follow Redis's unknown-command diagnostic.
	const maxArgsText = 128
	var b strings.Builder
	b.WriteString("unknown command '")
	b.WriteString(safeErrorToken(args[0], 128))
	b.WriteString("', with args beginning with: ")
	used := 0
	for _, arg := range args[1:] {
		if used >= maxArgsText {
			break
		}
		token := safeErrorToken(arg, maxArgsText-used)
		b.WriteByte('\'')
		b.WriteString(token)
		b.WriteString("', ")
		used += len(token) + 4
	}
	return b.String()
}

func safeErrorToken(b []byte, limit int) string {
	if limit < 0 {
		limit = 0
	}
	if len(b) > limit {
		b = b[:limit]
	}
	var out strings.Builder
	out.Grow(len(b))
	for _, c := range b {
		switch c {
		case '\r', '\n':
			out.WriteByte(' ')
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}
