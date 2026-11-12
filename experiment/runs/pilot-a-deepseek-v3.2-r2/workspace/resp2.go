package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// RESP2Parser handles parsing of RESP2 protocol
type RESP2Parser struct {
	reader *bufio.Reader
}

// NewRESP2Parser creates a new RESP2 parser
func NewRESP2Parser(reader *bufio.Reader) *RESP2Parser {
	return &RESP2Parser{reader: reader}
}

// ReadArray reads a RESP2 array
func (p *RESP2Parser) ReadArray() ([][]byte, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}

	if line[0] != '*' {
		return nil, fmt.Errorf("expected '*', got %q", line[0])
	}

	numArgs, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, fmt.Errorf("invalid number of arguments: %v", err)
	}

	if numArgs < 0 {
		return nil, fmt.Errorf("negative number of arguments")
	}

	args := make([][]byte, 0, numArgs)
	for i := 0; i < numArgs; i++ {
		arg, err := p.ReadBulkString()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}

	return args, nil
}

// ReadBulkString reads a RESP2 bulk string
func (p *RESP2Parser) ReadBulkString() ([]byte, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}

	if line[0] != '$' {
		return nil, fmt.Errorf("expected '$', got %q", line[0])
	}

	length, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, fmt.Errorf("invalid bulk string length: %v", err)
	}

	if length < 0 {
		// Null bulk string
		return nil, nil
	}

	data := make([]byte, length)
	_, err = io.ReadFull(p.reader, data)
	if err != nil {
		return nil, fmt.Errorf("failed to read bulk string data: %v", err)
	}

	// Read trailing CRLF
	_, err = p.readLine()
	if err != nil {
		return nil, fmt.Errorf("failed to read bulk string terminator: %v", err)
	}

	return data, nil
}

// readLine reads a line ending with CRLF
func (p *RESP2Parser) readLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// Remove trailing CRLF
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", fmt.Errorf("line doesn't end with CRLF")
	}

	return line[:len(line)-2], nil
}

// RESP2Writer handles writing RESP2 protocol responses
type RESP2Writer struct {
	writer io.Writer
}

// NewRESP2Writer creates a new RESP2 writer
func NewRESP2Writer(writer io.Writer) *RESP2Writer {
	return &RESP2Writer{writer: writer}
}

// WriteSimpleString writes a simple string response
func (w *RESP2Writer) WriteSimpleString(s string) error {
	_, err := fmt.Fprintf(w.writer, "+%s\r\n", s)
	return err
}

// WriteError writes an error response
func (w *RESP2Writer) WriteError(s string) error {
	_, err := fmt.Fprintf(w.writer, "-%s\r\n", s)
	return err
}

// WriteInteger writes an integer response
func (w *RESP2Writer) WriteInteger(n int64) error {
	_, err := fmt.Fprintf(w.writer, ":%d\r\n", n)
	return err
}

// WriteBulkString writes a bulk string response
func (w *RESP2Writer) WriteBulkString(b []byte) error {
	if b == nil {
		_, err := fmt.Fprintf(w.writer, "$-1\r\n")
		return err
	}
	
	_, err := fmt.Fprintf(w.writer, "$%d\r\n", len(b))
	if err != nil {
		return err
	}
	
	_, err = w.writer.Write(b)
	if err != nil {
		return err
	}
	
	_, err = fmt.Fprintf(w.writer, "\r\n")
	return err
}