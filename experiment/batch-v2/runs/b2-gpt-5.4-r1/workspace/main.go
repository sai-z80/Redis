package main

import (
	"bufio"
	"errors"
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

type entry struct {
	value     []byte
	expiresAt time.Time
	hasExpiry bool
}

type store struct {
	mu   sync.RWMutex
	data map[string]entry
}

func newStore() *store {
	return &store{data: make(map[string]entry)}
}

func keyString(key []byte) string {
	return string(key)
}

func (s *store) get(key []byte) ([]byte, bool) {
	ks := keyString(key)
	now := time.Now()

	s.mu.RLock()
	e, ok := s.data[ks]
	if !ok {
		s.mu.RUnlock()
		return nil, false
	}
	if e.hasExpiry && !e.expiresAt.After(now) {
		s.mu.RUnlock()
		s.mu.Lock()
		e2, ok2 := s.data[ks]
		if ok2 && e2.hasExpiry && !e2.expiresAt.After(time.Now()) {
			delete(s.data, ks)
		}
		s.mu.Unlock()
		return nil, false
	}
	val := append([]byte(nil), e.value...)
	s.mu.RUnlock()
	return val, true
}

func (s *store) set(key, value []byte, ttl *time.Duration) {
	ks := keyString(key)
	v := append([]byte(nil), value...)
	e := entry{value: v}
	if ttl != nil {
		e.hasExpiry = true
		e.expiresAt = time.Now().Add(*ttl)
	}
	s.mu.Lock()
	s.data[ks] = e
	s.mu.Unlock()
}

func (s *store) del(keys [][]byte) int64 {
	now := time.Now()
	seen := make(map[string]struct{}, len(keys))
	var deleted int64
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		ks := keyString(key)
		if _, ok := seen[ks]; ok {
			continue
		}
		seen[ks] = struct{}{}
		e, ok := s.data[ks]
		if !ok {
			continue
		}
		if e.hasExpiry && !e.expiresAt.After(now) {
			delete(s.data, ks)
			continue
		}
		delete(s.data, ks)
		deleted++
	}
	return deleted
}

type respConn struct {
	r *bufio.Reader
	w *bufio.Writer
}

func newRespConn(c net.Conn) *respConn {
	return &respConn{
		r: bufio.NewReader(c),
		w: bufio.NewWriter(c),
	}
}

func (c *respConn) readLine() ([]byte, error) {
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, errors.New("protocol error")
	}
	return line[:len(line)-2], nil
}

func (c *respConn) readRequest() ([][]byte, error) {
	line, err := c.readLine()
	if err != nil {
		return nil, err
	}
	if len(line) == 0 || line[0] != '*' {
		return nil, errors.New("protocol error")
	}
	n, err := strconv.Atoi(string(line[1:]))
	if err != nil || n < 0 {
		return nil, errors.New("protocol error")
	}
	args := make([][]byte, n)
	for i := 0; i < n; i++ {
		header, err := c.readLine()
		if err != nil {
			return nil, err
		}
		if len(header) == 0 || header[0] != '$' {
			return nil, errors.New("protocol error")
		}
		l, err := strconv.Atoi(string(header[1:]))
		if err != nil || l < 0 {
			return nil, errors.New("protocol error")
		}
		buf := make([]byte, l+2)
		if _, err := io.ReadFull(c.r, buf); err != nil {
			return nil, err
		}
		if buf[l] != '\r' || buf[l+1] != '\n' {
			return nil, errors.New("protocol error")
		}
		args[i] = append([]byte(nil), buf[:l]...)
	}
	return args, nil
}

func (c *respConn) writeSimple(s string) error {
	_, err := c.w.WriteString("+" + s + "\r\n")
	return err
}

func (c *respConn) writeError(msg string) error {
	_, err := c.w.WriteString("-ERR " + msg + "\r\n")
	return err
}

func (c *respConn) writeInt(v int64) error {
	_, err := c.w.WriteString(":" + strconv.FormatInt(v, 10) + "\r\n")
	return err
}

func (c *respConn) writeBulk(b []byte) error {
	if _, err := c.w.WriteString("$" + strconv.Itoa(len(b)) + "\r\n"); err != nil {
		return err
	}
	if _, err := c.w.Write(b); err != nil {
		return err
	}
	_, err := c.w.WriteString("\r\n")
	return err
}

func (c *respConn) writeNil() error {
	_, err := c.w.WriteString("$-1\r\n")
	return err
}

func (c *respConn) flush() error {
	return c.w.Flush()
}

func wrongArity(cmd string) string {
	return fmt.Sprintf("wrong number of arguments for '%s' command", strings.ToLower(cmd))
}

func parsePositiveTTL(arg []byte, unit time.Duration) (*time.Duration, string) {
	n, err := strconv.ParseInt(string(arg), 10, 64)
	if err != nil {
		return nil, "value is not an integer or out of range"
	}
	if n <= 0 {
		return nil, "invalid expire time in 'set' command"
	}
	ttl := time.Duration(n) * unit
	return &ttl, ""
}

func handleCommand(s *store, c *respConn, args [][]byte) error {
	if len(args) == 0 {
		if err := c.writeError("unknown command ''"); err != nil {
			return err
		}
		return c.flush()
	}
	cmd := strings.ToUpper(string(args[0]))
	switch cmd {
	case "PING":
		switch len(args) {
		case 1:
			if err := c.writeSimple("PONG"); err != nil {
				return err
			}
		case 2:
			if err := c.writeBulk(args[1]); err != nil {
				return err
			}
		default:
			if err := c.writeError(wrongArity(cmd)); err != nil {
				return err
			}
		}
	case "ECHO":
		if len(args) != 2 {
			if err := c.writeError(wrongArity(cmd)); err != nil {
				return err
			}
		} else if err := c.writeBulk(args[1]); err != nil {
			return err
		}
	case "SET":
		if len(args) != 3 && len(args) != 5 {
			if err := c.writeError(wrongArity(cmd)); err != nil {
				return err
			}
			break
		}
		var ttl *time.Duration
		if len(args) == 5 {
			opt := strings.ToUpper(string(args[3]))
			switch opt {
			case "EX":
				var msg string
				ttl, msg = parsePositiveTTL(args[4], time.Second)
				if msg != "" {
					if err := c.writeError(msg); err != nil {
						return err
					}
					break
				}
			case "PX":
				var msg string
				ttl, msg = parsePositiveTTL(args[4], time.Millisecond)
				if msg != "" {
					if err := c.writeError(msg); err != nil {
						return err
					}
					break
				}
			default:
				if err := c.writeError("syntax error"); err != nil {
					return err
				}
				break
			}
			if ttl == nil && opt != "EX" && opt != "PX" {
				break
			}
			if ttl == nil {
				break
			}
		}
		s.set(args[1], args[2], ttl)
		if err := c.writeSimple("OK"); err != nil {
			return err
		}
	case "GET":
		if len(args) != 2 {
			if err := c.writeError(wrongArity(cmd)); err != nil {
				return err
			}
			break
		}
		if v, ok := s.get(args[1]); ok {
			if err := c.writeBulk(v); err != nil {
				return err
			}
		} else if err := c.writeNil(); err != nil {
			return err
		}
	case "DEL":
		if len(args) < 2 {
			if err := c.writeError(wrongArity(cmd)); err != nil {
				return err
			}
			break
		}
		if err := c.writeInt(s.del(args[1:])); err != nil {
			return err
		}
	default:
		if err := c.writeError(fmt.Sprintf("unknown command '%s'", string(args[0]))); err != nil {
			return err
		}
	}
	return c.flush()
}

func handleClient(conn net.Conn, s *store) {
	defer conn.Close()
	rc := newRespConn(conn)
	for {
		args, err := rc.readRequest()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "closed") {
				return
			}
			return
		}
		if err := handleCommand(s, rc, args); err != nil {
			return
		}
	}
}

func main() {
	port := flag.Int("port", 6380, "TCP port to listen on")
	flag.Parse()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	s := newStore()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				continue
			}
			log.Fatal(err)
		}
		go handleClient(conn, s)
	}
}
