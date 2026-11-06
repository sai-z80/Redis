package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Storage struct {
	mu    sync.RWMutex
	data  map[string]*Value
}

type Value struct {
	data []byte
	exp  *time.Time // nil means no expiry
}

func NewStorage() *Storage {
	return &Storage{
		data: make(map[string]*Value),
	}
}

func (s *Storage) Get(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	val, exists := s.data[key]
	if !exists {
		return nil, false
	}
	
	// Check expiry (lazy expiry)
	if val.exp != nil && time.Now().After(*val.exp) {
		delete(s.data, key)
		return nil, false
	}
	
	return val.data, true
}

func (s *Storage) Set(key string, value []byte, expiryMillis *int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	val := &Value{data: value}
	if expiryMillis != nil {
		exp := time.Now().Add(time.Duration(*expiryMillis) * time.Millisecond)
		val.exp = &exp
	}
	s.data[key] = val
}

func (s *Storage) Del(keys []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	deleted := 0
	for _, key := range keys {
		val, exists := s.data[key]
		if !exists {
			continue
		}
		
		// Check expiry (lazy expiry)
		if val.exp != nil && time.Now().After(*val.exp) {
			delete(s.data, key)
			continue
		}
		
		delete(s.data, key)
		deleted++
	}
	return deleted
}

type Client struct {
	conn net.Conn
	rd   *bufio.Reader
	wr   *bufio.Writer
	storage *Storage
}

func NewClient(conn net.Conn, storage *Storage) *Client {
	return &Client{
		conn: conn,
		rd:   bufio.NewReader(conn),
		wr:   bufio.NewWriter(conn),
		storage: storage,
	}
}

func (c *Client) Close() {
	c.conn.Close()
}

func (c *Client) readLine() ([]byte, error) {
	line, err := c.rd.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	// Remove \r\n
	if len(line) >= 2 {
		line = line[:len(line)-2]
	}
	return line, nil
}

func (c *Client) readBulkString(length int) ([]byte, error) {
	data := make([]byte, length)
	_, err := io.ReadFull(c.rd, data)
	if err != nil {
		return nil, err
	}
	// Read trailing \r\n
	_, err = c.rd.Discard(2)
	return data, err
}

func (c *Client) readRequest() ([][]byte, error) {
	line, err := c.readLine()
	if err != nil {
		return nil, err
	}
	
	if len(line) == 0 || line[0] != '*' {
		return nil, fmt.Errorf("invalid request")
	}
	
	numArgs, err := strconv.Atoi(string(line[1:]))
	if err != nil {
		return nil, err
	}
	
	if numArgs <= 0 {
		return [][]byte{}, nil
	}
	
	args := make([][]byte, numArgs)
	for i := 0; i < numArgs; i++ {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}
		
		if len(line) == 0 || line[0] != '$' {
			return nil, fmt.Errorf("invalid bulk string header")
		}
		
		length, err := strconv.Atoi(string(line[1:]))
		if err != nil {
			return nil, err
		}
		
		if length < 0 {
			return nil, fmt.Errorf("invalid bulk string length")
		}
		
		arg, err := c.readBulkString(length)
		if err != nil {
			return nil, err
		}
		args[i] = arg
	}
	
	return args, nil
}

func (c *Client) writeSimpleString(s string) error {
	_, err := c.wr.WriteString("+" + s + "\r\n")
	return err
}

func (c *Client) writeError(s string) error {
	_, err := c.wr.WriteString("-ERR " + s + "\r\n")
	return err
}

func (c *Client) writeInteger(n int) error {
	_, err := c.wr.WriteString(":" + strconv.Itoa(n) + "\r\n")
	return err
}

func (c *Client) writeBulkString(b []byte) error {
	if b == nil {
		_, err := c.wr.WriteString("$-1\r\n")
		return err
	}
	_, err := c.wr.WriteString("$" + strconv.Itoa(len(b)) + "\r\n")
	if err != nil {
		return err
	}
	_, err = c.wr.Write(b)
	if err != nil {
		return err
	}
	_, err = c.wr.WriteString("\r\n")
	return err
}

func (c *Client) writeNull() error {
	_, err := c.wr.WriteString("$-1\r\n")
	return err
}

func (c *Client) flush() error {
	return c.wr.Flush()
}

func (c *Client) handleCommand(args [][]byte) error {
	if len(args) == 0 {
		return c.writeError("empty command")
	}
	
	cmd := strings.ToUpper(string(args[0]))
	
	switch cmd {
	case "PING":
		return c.handlePing(args[1:])
	case "ECHO":
		return c.handleEcho(args[1:])
	case "SET":
		return c.handleSet(args[1:])
	case "GET":
		return c.handleGet(args[1:])
	case "DEL":
		return c.handleDel(args[1:])
	default:
		return c.writeError("unknown command '" + cmd + "'")
	}
}

func (c *Client) handlePing(args [][]byte) error {
	if len(args) > 1 {
		return c.writeError("wrong number of arguments for 'ping' command")
	}
	
	if len(args) == 0 {
		return c.writeSimpleString("PONG")
	}
	return c.writeBulkString(args[0])
}

func (c *Client) handleEcho(args [][]byte) error {
	if len(args) != 1 {
		return c.writeError("wrong number of arguments for 'echo' command")
	}
	return c.writeBulkString(args[0])
}

func (c *Client) handleSet(args [][]byte) error {
	if len(args) < 2 {
		return c.writeError("wrong number of arguments for 'set' command")
	}
	
	key := string(args[0])
	value := args[1]
	var expiryMillis *int64
	
	// Parse optional EX/PX arguments
	i := 2
	for i < len(args) {
		opt := strings.ToUpper(string(args[i]))
		if i+1 >= len(args) {
			return c.writeError("syntax error")
		}
		
		expStr := string(args[i+1])
		exp, err := strconv.ParseInt(expStr, 10, 64)
		if err != nil || exp <= 0 {
			return c.writeError("invalid expire time")
		}
		
		switch opt {
		case "EX":
			if expiryMillis != nil {
				return c.writeError("syntax error")
			}
			ms := exp * 1000
			expiryMillis = &ms
		case "PX":
			if expiryMillis != nil {
				return c.writeError("syntax error")
			}
			expiryMillis = &exp
		default:
			return c.writeError("syntax error")
		}
		i += 2 // Skip the option and its value
	}
	
	c.storage.Set(key, value, expiryMillis)
	return c.writeSimpleString("OK")
}

func (c *Client) handleGet(args [][]byte) error {
	if len(args) != 1 {
		return c.writeError("wrong number of arguments for 'get' command")
	}
	
	key := string(args[0])
	if data, exists := c.storage.Get(key); exists {
		return c.writeBulkString(data)
	}
	return c.writeNull()
}

func (c *Client) handleDel(args [][]byte) error {
	if len(args) == 0 {
		return c.writeError("wrong number of arguments for 'del' command")
	}
	
	keys := make([]string, len(args))
	for i, arg := range args {
		keys[i] = string(arg)
	}
	
	deleted := c.storage.Del(keys)
	return c.writeInteger(deleted)
}

func (c *Client) process() {
	defer c.Close()
	
	for {
		args, err := c.readRequest()
		if err != nil {
			if err != io.EOF {
				// Try to send error response if possible
				c.writeError("protocol error")
				c.flush()
			}
			break
		}
		
		if err := c.handleCommand(args); err != nil {
			break
		}
		
		if err := c.flush(); err != nil {
			break
		}
	}
}

func main() {
	port := flag.Int("port", 6380, "port to listen on")
	flag.Parse()
	
	storage := NewStorage()
	
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on port %d: %v\n", *port, err)
		os.Exit(1)
	}
	defer ln.Close()
	
	fmt.Printf("Redis-mini server listening on 127.0.0.1:%d\n", *port)
	
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Accept error: %v\n", err)
			continue
		}
		
		client := NewClient(conn, storage)
		go client.process()
	}
}