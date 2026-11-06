package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Database represents the key-value store
type Database struct {
	mu      sync.RWMutex
	data    map[string]*Value
	expires map[string]time.Time
}

// Value represents a stored value with optional expiry
type Value struct {
	data []byte
}

// NewDatabase creates a new database instance
func NewDatabase() *Database {
	return &Database{
		data:    make(map[string]*Value),
		expires: make(map[string]time.Time),
	}
}

// Get retrieves a value from the database, checking for expiry
func (db *Database) Get(key string) ([]byte, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Check if key exists
	val, exists := db.data[key]
	if !exists {
		return nil, false
	}

	// Check if key has expired
	if expiry, hasExpiry := db.expires[key]; hasExpiry {
		if time.Now().After(expiry) {
			// Key has expired, delete it (lazy expiry)
			db.mu.RUnlock()
			db.mu.Lock()
			delete(db.data, key)
			delete(db.expires, key)
			db.mu.Unlock()
			db.mu.RLock()
			return nil, false
		}
	}

	return val.data, true
}

// Set stores a value in the database with optional expiry
func (db *Database) Set(key string, value []byte, expiry time.Duration) {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.data[key] = &Value{data: value}
	if expiry > 0 {
		db.expires[key] = time.Now().Add(expiry)
	} else {
		delete(db.expires, key)
	}
}

// Delete removes a key from the database
func (db *Database) Delete(key string) bool {
	db.mu.Lock()
	defer db.mu.Unlock()

	_, exists := db.data[key]
	if exists {
		delete(db.data, key)
		delete(db.expires, key)
		return true
	}
	return false
}

// Client represents a connected client
type Client struct {
	conn   net.Conn
	db     *Database
	reader *bufio.Reader
	writer *RESP2Writer
}

// Handle processes client commands
func (c *Client) Handle() {
	defer c.conn.Close()

	for {
		parser := NewRESP2Parser(c.reader)
		args, err := parser.ReadArray()
		if err != nil {
			if err.Error() != "EOF" {
				c.writer.WriteError(fmt.Sprintf("ERR Protocol error: %v", err))
			}
			return
		}

		c.executeCommand(args)
	}
}

// executeCommand processes a command with its arguments
func (c *Client) executeCommand(args [][]byte) {
	if len(args) == 0 {
		c.writer.WriteError("ERR wrong number of arguments for command")
		return
	}

	cmd := strings.ToUpper(string(args[0]))

	switch cmd {
	case "PING":
		c.handlePing(args)
	case "ECHO":
		c.handleEcho(args)
	case "SET":
		c.handleSet(args)
	case "GET":
		c.handleGet(args)
	case "DEL":
		c.handleDel(args)
	default:
		c.writer.WriteError(fmt.Sprintf("ERR unknown command '%s'", cmd))
	}
}

func (c *Client) handlePing(args [][]byte) {
	if len(args) == 1 {
		c.writer.WriteSimpleString("PONG")
	} else if len(args) == 2 {
		c.writer.WriteBulkString(args[1])
	} else {
		c.writer.WriteError("ERR wrong number of arguments for 'ping' command")
	}
}

func (c *Client) handleEcho(args [][]byte) {
	if len(args) != 2 {
		c.writer.WriteError("ERR wrong number of arguments for 'echo' command")
		return
	}
	c.writer.WriteBulkString(args[1])
}

func (c *Client) handleSet(args [][]byte) {
	if len(args) < 3 {
		c.writer.WriteError("ERR wrong number of arguments for 'set' command")
		return
	}

	key := string(args[1])
	value := args[2]
	var expiry time.Duration

	// Parse optional EX/PX arguments
	i := 3
	for i < len(args) {
		arg := strings.ToUpper(string(args[i]))
		if arg == "EX" || arg == "PX" {
			if i+1 >= len(args) {
				c.writer.WriteError("ERR syntax error")
				return
			}

			expiryStr := string(args[i+1])
			expiryInt, err := strconv.ParseInt(expiryStr, 10, 64)
			if err != nil || expiryInt <= 0 {
				c.writer.WriteError("ERR value is not an integer or out of range")
				return
			}

			if arg == "EX" {
				expiry = time.Duration(expiryInt) * time.Second
			} else { // PX
				expiry = time.Duration(expiryInt) * time.Millisecond
			}
			i += 2
		} else {
			// Unknown option
			c.writer.WriteError("ERR syntax error")
			return
		}
	}

	c.db.Set(key, value, expiry)
	c.writer.WriteSimpleString("OK")
}

func (c *Client) handleGet(args [][]byte) {
	if len(args) != 2 {
		c.writer.WriteError("ERR wrong number of arguments for 'get' command")
		return
	}

	key := string(args[1])
	if value, ok := c.db.Get(key); ok {
		c.writer.WriteBulkString(value)
	} else {
		c.writer.WriteBulkString(nil)
	}
}

func (c *Client) handleDel(args [][]byte) {
	if len(args) < 2 {
		c.writer.WriteError("ERR wrong number of arguments for 'del' command")
		return
	}

	count := 0
	for i := 1; i < len(args); i++ {
		key := string(args[i])
		if c.db.Delete(key) {
			count++
		}
	}
	c.writer.WriteInteger(int64(count))
}

func main() {
	port := flag.Int("port", 6380, "Port to listen on")
	flag.Parse()

	db := NewDatabase()
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Printf("Failed to listen on port %d: %v\n", *port, err)
		return
	}
	defer listener.Close()

	fmt.Printf("Redis-mini server listening on 127.0.0.1:%d\n", *port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}

		reader := bufio.NewReader(conn)
		writer := NewRESP2Writer(conn)
		client := &Client{
			conn:   conn,
			db:     db,
			reader: reader,
			writer: writer,
		}
		go client.Handle()
	}
}