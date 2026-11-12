package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

const (
	// These are Redis's normal protocol limits. Keeping the count-line limit
	// also prevents an unterminated header from growing without bound.
	maxProtocolLine = 64 * 1024
	maxBulkLength   = 512 * 1024 * 1024
	maxMultibulkLen = int64(1<<31 - 1)
)

type protocolError struct {
	message string
}

func (e *protocolError) Error() string { return e.message }

var errProtocolLineTooLong = errors.New("protocol line too long")

// serveConn processes requests sequentially on each connection. Redis clients
// may pipeline requests; bufio.Reader preserves any bytes belonging to the next
// request while the current command is executing.
func (s *server) serveConn(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReaderSize(conn, 64*1024)
	w := newRESPWriter(conn)

	for {
		args, err := readCommand(r)
		if err != nil {
			if pe, ok := err.(*protocolError); ok {
				w.writeError("Protocol error: " + pe.message)
				_ = w.Flush()
			}
			// EOF, partial requests, network errors, and protocol errors all
			// close just this client connection. Redis closes after sending a
			// protocol error because the remaining stream cannot be framed
			// reliably.
			return
		}
		if len(args) == 0 {
			// Empty and negative multibulk lengths produce no command and no
			// reply, matching Redis's request parser.
			continue
		}
		if err := s.execute(args, w); err != nil {
			return
		}
	}
}

// readCommand reads exactly one RESP multibulk request. Inline commands and
// non-bulk array elements are deliberately outside this server's protocol
// subset and result in a protocol error.
func readCommand(r *bufio.Reader) ([][]byte, error) {
	line, err := readProtocolLine(r)
	if err != nil {
		if errors.Is(err, errProtocolLineTooLong) {
			return nil, &protocolError{message: "too big mbulk count string"}
		}
		return nil, err
	}
	if len(line) == 0 || line[0] != '*' {
		got := byte('?')
		if len(line) != 0 {
			got = line[0]
		}
		return nil, &protocolError{message: fmt.Sprintf("expected '*', got '%c'", got)}
	}
	count, ok := parseRedisInt64(line[1:])
	if !ok || count > maxMultibulkLen {
		return nil, &protocolError{message: "invalid multibulk length"}
	}
	if count <= 0 {
		return nil, nil
	}

	// Do not make a massive up-front allocation for a client-supplied count.
	// Redis similarly starts with a bounded argv allocation and grows it.
	capacity := int(count)
	if capacity > 1024 {
		capacity = 1024
	}
	args := make([][]byte, 0, capacity)
	for i := int64(0); i < count; i++ {
		line, err = readProtocolLine(r)
		if err != nil {
			if errors.Is(err, errProtocolLineTooLong) {
				return nil, &protocolError{message: "too big bulk count string"}
			}
			return nil, err
		}
		if len(line) == 0 || line[0] != '$' {
			got := byte('?')
			if len(line) != 0 {
				got = line[0]
			}
			return nil, &protocolError{message: fmt.Sprintf("expected '$', got '%c'", got)}
		}
		n, ok := parseRedisInt64(line[1:])
		if !ok || n < 0 || n > maxBulkLength {
			return nil, &protocolError{message: "invalid bulk length"}
		}

		// Read the payload and its CRLF together so arbitrary binary bytes,
		// including embedded CRLF and NUL, are never interpreted as framing.
		payload := make([]byte, int(n)+2)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
		if payload[n] != '\r' || payload[n+1] != '\n' {
			return nil, &protocolError{message: "invalid bulk terminator"}
		}
		args = append(args, payload[:n:n])
	}
	return args, nil
}

// readProtocolLine returns a line without its CRLF. A count line cannot exceed
// Redis's inline/header limit, and both CR and LF are required for multibulk
// framing.
func readProtocolLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		if len(line)+len(part) > maxProtocolLine+2 {
			return nil, errProtocolLineTooLong
		}
		line = append(line, part...)
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return nil, err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, &protocolError{message: "invalid line terminator"}
	}
	return line[:len(line)-2], nil
}

type respWriter struct {
	w *bufio.Writer
}

func newRESPWriter(w io.Writer) *respWriter {
	return &respWriter{w: bufio.NewWriterSize(w, 64*1024)}
}

func (w *respWriter) Flush() error {
	return w.w.Flush()
}

func (w *respWriter) writeSimple(s string) {
	w.w.WriteByte('+')
	w.w.WriteString(s)
	w.w.WriteString("\r\n")
}

func (w *respWriter) writeError(message string) {
	w.w.WriteString("-ERR ")
	w.w.WriteString(message)
	w.w.WriteString("\r\n")
}

func (w *respWriter) writeInteger(n int64) {
	w.w.WriteByte(':')
	w.w.WriteString(strconv.FormatInt(n, 10))
	w.w.WriteString("\r\n")
}

func (w *respWriter) writeNullBulk() {
	w.w.WriteString("$-1\r\n")
}

func (w *respWriter) writeBulk(b []byte) {
	w.w.WriteByte('$')
	w.w.WriteString(strconv.Itoa(len(b)))
	w.w.WriteString("\r\n")
	w.w.Write(b)
	w.w.WriteString("\r\n")
}

func (w *respWriter) writeBulkString(s string) {
	w.w.WriteByte('$')
	w.w.WriteString(strconv.Itoa(len(s)))
	w.w.WriteString("\r\n")
	w.w.WriteString(s)
	w.w.WriteString("\r\n")
}
