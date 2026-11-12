package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── Data Store ──────────────────────────────────────────────────────────────

type entry struct {
	value    string
	expireAt int64 // unix milliseconds; 0 means no expiry
}

type store struct {
	mu   sync.Mutex
	data map[string]entry
}

func newStore() *store {
	return &store{data: make(map[string]entry)}
}

// get returns the value and true if the key exists and has not expired.
// Expired keys are lazily deleted.
func (s *store) get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if !ok {
		return "", false
	}
	if e.expireAt != 0 && time.Now().UnixMilli() >= e.expireAt {
		delete(s.data, key)
		return "", false
	}
	return e.value, true
}

// set stores a key-value pair with an optional expiry in milliseconds from now.
// expireMs <= 0 means no expiry.
func (s *store) set(key, value string, expireAt int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = entry{value: value, expireAt: expireAt}
}

// del deletes keys and returns the count of keys that actually existed (and were not expired).
func (s *store) del(keys []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	now := time.Now().UnixMilli()
	for _, key := range keys {
		e, ok := s.data[key]
		if !ok {
			continue
		}
		if e.expireAt != 0 && now >= e.expireAt {
			// expired – remove but don't count
			delete(s.data, key)
			continue
		}
		delete(s.data, key)
		count++
	}
	return count
}

// ─── RESP2 Protocol ──────────────────────────────────────────────────────────

// readLine reads until \r\n from a bufio.Reader and returns the line without CRLF.
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", fmt.Errorf("protocol error: expected CRLF")
	}
	return line[:len(line)-2], nil
}

// readCommand reads one RESP2 multibulk command (array of bulk strings).
func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}
	if len(line) == 0 || line[0] != '*' {
		return nil, fmt.Errorf("protocol error: expected '*'")
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil || n < 0 {
		return nil, fmt.Errorf("protocol error: invalid multibulk length")
	}
	args := make([]string, n)
	for i := 0; i < n; i++ {
		line, err = readLine(r)
		if err != nil {
			return nil, err
		}
		if len(line) == 0 || line[0] != '$' {
			return nil, fmt.Errorf("protocol error: expected '$'")
		}
		blen, err := strconv.Atoi(line[1:])
		if err != nil || blen < 0 {
			return nil, fmt.Errorf("protocol error: invalid bulk length")
		}
		buf := make([]byte, blen+2) // +2 for \r\n
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return nil, err
		}
		if buf[blen] != '\r' || buf[blen+1] != '\n' {
			return nil, fmt.Errorf("protocol error: expected CRLF after bulk data")
		}
		args[i] = string(buf[:blen])
	}
	return args, nil
}

// RESP reply helpers

func writeSimple(w *bufio.Writer, s string) {
	fmt.Fprintf(w, "+%s\r\n", s)
}

func writeError(w *bufio.Writer, s string) {
	fmt.Fprintf(w, "-%s\r\n", s)
}

func writeInteger(w *bufio.Writer, n int) {
	fmt.Fprintf(w, ":%d\r\n", n)
}

func writeBulk(w *bufio.Writer, s string) {
	fmt.Fprintf(w, "$%d\r\n", len(s))
	w.WriteString(s)
	w.WriteString("\r\n")
}

func writeNullBulk(w *bufio.Writer) {
	w.WriteString("$-1\r\n")
}

// ─── Command Handlers ────────────────────────────────────────────────────────

func handleConnection(conn net.Conn, db *store) {
	defer conn.Close()
	r := bufio.NewReaderSize(conn, 64*1024)
	w := bufio.NewWriterSize(conn, 64*1024)

	for {
		args, err := readCommand(r)
		if err != nil {
			return // client disconnected or protocol error
		}
		if len(args) == 0 {
			continue
		}

		cmd := strings.ToUpper(args[0])
		switch cmd {
		case "PING":
			cmdPing(w, args)
		case "ECHO":
			cmdEcho(w, args)
		case "SET":
			cmdSet(w, args, db)
		case "GET":
			cmdGet(w, args, db)
		case "DEL":
			cmdDel(w, args, db)
		default:
			writeError(w, fmt.Sprintf("ERR unknown command '%s', with args beginning with: %s", args[0], formatArgs(args[1:])))
		}
		w.Flush()
	}
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	var parts []string
	for _, a := range args {
		parts = append(parts, fmt.Sprintf("'%s'", a))
	}
	return strings.Join(parts, " ")
}

func cmdPing(w *bufio.Writer, args []string) {
	switch len(args) {
	case 1:
		writeSimple(w, "PONG")
	case 2:
		writeBulk(w, args[1])
	default:
		writeError(w, fmt.Sprintf("ERR wrong number of arguments for '%s' command", args[0]))
	}
}

func cmdEcho(w *bufio.Writer, args []string) {
	if len(args) != 2 {
		writeError(w, fmt.Sprintf("ERR wrong number of arguments for '%s' command", args[0]))
		return
	}
	writeBulk(w, args[1])
}

func cmdSet(w *bufio.Writer, args []string, db *store) {
	if len(args) < 3 {
		writeError(w, fmt.Sprintf("ERR wrong number of arguments for '%s' command", args[0]))
		return
	}

	key := args[1]
	value := args[2]
	var expireAt int64 // 0 = no expiry

	// Parse optional arguments starting at index 3
	i := 3
	hasEX := false
	hasPX := false
	for i < len(args) {
		opt := strings.ToUpper(args[i])
		switch opt {
		case "EX":
			if hasPX || hasEX {
				writeError(w, "ERR syntax error")
				return
			}
			i++
			if i >= len(args) {
				writeError(w, "ERR syntax error")
				return
			}
			secs, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				writeError(w, "ERR value is not an integer or out of range")
				return
			}
			if secs <= 0 {
				writeError(w, fmt.Sprintf("ERR invalid expire time in '%s' command", args[0]))
				return
			}
			hasEX = true
			expireAt = time.Now().UnixMilli() + secs*1000
			i++
		case "PX":
			if hasEX || hasPX {
				writeError(w, "ERR syntax error")
				return
			}
			i++
			if i >= len(args) {
				writeError(w, "ERR syntax error")
				return
			}
			ms, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				writeError(w, "ERR value is not an integer or out of range")
				return
			}
			if ms <= 0 {
				writeError(w, fmt.Sprintf("ERR invalid expire time in '%s' command", args[0]))
				return
			}
			hasPX = true
			expireAt = time.Now().UnixMilli() + ms
			i++
		default:
			writeError(w, "ERR syntax error")
			return
		}
	}

	db.set(key, value, expireAt)
	writeSimple(w, "OK")
}

func cmdGet(w *bufio.Writer, args []string, db *store) {
	if len(args) != 2 {
		writeError(w, fmt.Sprintf("ERR wrong number of arguments for '%s' command", args[0]))
		return
	}
	val, ok := db.get(args[1])
	if !ok {
		writeNullBulk(w)
		return
	}
	writeBulk(w, val)
}

func cmdDel(w *bufio.Writer, args []string, db *store) {
	if len(args) < 2 {
		writeError(w, fmt.Sprintf("ERR wrong number of arguments for '%s' command", args[0]))
		return
	}
	count := db.del(args[1:])
	writeInteger(w, count)
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	port := flag.Int("port", 6380, "TCP port to listen on")
	flag.Parse()

	db := newStore()

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}
	log.Printf("redis-mini listening on %s", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		go handleConnection(conn, db)
	}
}
