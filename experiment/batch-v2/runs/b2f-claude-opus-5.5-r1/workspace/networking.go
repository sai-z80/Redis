package main

// Client handling, RESP2 request parsing and reply building.
// Ported from Redis networking.c (processInputBuffer,
// processInlineBuffer, processMultibulkBuffer, addReply*).

import (
	"bufio"
	"fmt"
	"math"
	"net"
	"strconv"
)

const (
	protoIOBufLen      = 1024 * 16
	protoInlineMaxSize = 1024 * 64
	protoMbulkBigArg   = 1024 * 32
	protoMaxBulkLen    = 512 * 1024 * 1024

	protoReqInline    = 1
	protoReqMultibulk = 2
)

type client struct {
	srv  *server
	conn net.Conn
	w    *bufio.Writer

	querybuf     []byte
	qbPos        int
	reqtype      int
	multibulklen int
	bulklen      int
	argv         [][]byte

	cmd     *redisCommand
	cmdTime int64

	closeAfterReply bool
}

func newClient(s *server, conn net.Conn) *client {
	return &client{
		srv:     s,
		conn:    conn,
		w:       bufio.NewWriterSize(conn, 16*1024),
		bulklen: -1,
	}
}

// ---------------------------------------------------------------- replies

func (c *client) addReplyRaw(b []byte) {
	if c.closeAfterReply {
		return
	}
	c.w.Write(b)
}

func (c *client) addReplyString(s string) {
	if c.closeAfterReply {
		return
	}
	c.w.WriteString(s)
}

func (c *client) addReplyStatus(s string) {
	c.addReplyString("+" + mapCRLF(s) + "\r\n")
}

// addReplyError emits "-ERR <msg>" unless msg already starts with '-'
// (in which case the error code is taken from it), mirroring
// addReplyErrorLength().
func (c *client) addReplyError(msg string) {
	msg = mapCRLF(msg)
	if len(msg) == 0 || msg[0] != '-' {
		c.addReplyString("-ERR " + msg + "\r\n")
	} else {
		c.addReplyString(msg + "\r\n")
	}
}

func (c *client) addReplyErrorFormat(format string, a ...interface{}) {
	c.addReplyError(fmt.Sprintf(format, a...))
}

func (c *client) addReplyErrorArity() {
	c.addReplyErrorFormat("wrong number of arguments for '%s' command", c.cmd.name)
}

func (c *client) addReplyErrorExpireTime() {
	c.addReplyErrorFormat("invalid expire time in '%s' command", c.cmd.name)
}

func (c *client) addReplyLongLong(v int64) {
	c.addReplyString(":" + strconv.FormatInt(v, 10) + "\r\n")
}

func (c *client) addReplyBulk(b []byte) {
	if c.closeAfterReply {
		return
	}
	c.w.WriteString("$" + strconv.Itoa(len(b)) + "\r\n")
	c.w.Write(b)
	c.w.WriteString("\r\n")
}

func (c *client) addReplyNull() {
	c.addReplyString("$-1\r\n")
}

// setProtocolError marks the client to be closed after the pending
// replies are flushed.
func (c *client) setProtocolError(msg string) {
	if c.srv.verbose {
		c.srv.logf("Protocol error (%s) from client: addr=%s", msg, c.conn.RemoteAddr())
	}
	c.closeAfterReply = true
}

// ---------------------------------------------------------------- parsing

func indexByteFrom(b []byte, from int, ch byte) int {
	for i := from; i < len(b); i++ {
		if b[i] == ch {
			return i
		}
	}
	return -1
}

func (c *client) resetClient() {
	c.argv = nil
	c.reqtype = 0
	c.multibulklen = 0
	c.bulklen = -1
	c.cmd = nil
}

// processInlineBuffer parses an inline (telnet style) request.
func (c *client) processInlineBuffer() bool {
	newline := indexByteFrom(c.querybuf, c.qbPos, '\n')
	if newline < 0 {
		if len(c.querybuf)-c.qbPos > protoInlineMaxSize {
			c.addReplyError("Protocol error: too big inline request")
			c.setProtocolError("too big inline request")
		}
		return false
	}
	linefeedChars := 1
	if newline != c.qbPos && c.querybuf[newline-1] == '\r' {
		newline--
		linefeedChars++
	}
	line := c.querybuf[c.qbPos:newline]
	argv, ok := splitArgs(line)
	if !ok {
		c.addReplyError("Protocol error: unbalanced quotes in request")
		c.setProtocolError("unbalanced quotes in inline request")
		return false
	}
	c.qbPos += len(line) + linefeedChars
	c.argv = argv
	return true
}

// processMultibulkBuffer parses a RESP multibulk request. Returns true
// when a complete command is available in c.argv.
func (c *client) processMultibulkBuffer() bool {
	if c.multibulklen == 0 {
		newline := indexByteFrom(c.querybuf, c.qbPos, '\r')
		if newline < 0 {
			if len(c.querybuf)-c.qbPos > protoInlineMaxSize {
				c.addReplyError("Protocol error: too big mbulk count string")
				c.setProtocolError("too big mbulk count string")
			}
			return false
		}
		// Buffer should also contain \n
		if newline-c.qbPos > len(c.querybuf)-c.qbPos-2 {
			return false
		}
		ll, ok := string2ll(c.querybuf[c.qbPos+1 : newline])
		if !ok || ll > math.MaxInt32 {
			c.addReplyError("Protocol error: invalid multibulk length")
			c.setProtocolError("invalid mbulk count")
			return false
		}
		c.qbPos = newline + 2
		if ll <= 0 {
			return true
		}
		c.multibulklen = int(ll)
		n := c.multibulklen
		if n > 1024 {
			n = 1024
		}
		c.argv = make([][]byte, 0, n)
	}

	for c.multibulklen > 0 {
		if c.bulklen == -1 {
			newline := indexByteFrom(c.querybuf, c.qbPos, '\r')
			if newline < 0 {
				if len(c.querybuf)-c.qbPos > protoInlineMaxSize {
					c.addReplyError("Protocol error: too big bulk count string")
					c.setProtocolError("too big bulk count string")
					return false
				}
				break
			}
			if newline-c.qbPos > len(c.querybuf)-c.qbPos-2 {
				break
			}
			if c.querybuf[c.qbPos] != '$' {
				c.addReplyErrorFormat("Protocol error: expected '$', got '%c'", c.querybuf[c.qbPos])
				c.setProtocolError("expected $ but got something else")
				return false
			}
			ll, ok := string2ll(c.querybuf[c.qbPos+1 : newline])
			if !ok || ll < 0 || ll > protoMaxBulkLen {
				c.addReplyError("Protocol error: invalid bulk length")
				c.setProtocolError("invalid bulk length")
				return false
			}
			c.qbPos = newline + 2
			if ll >= protoMbulkBigArg {
				// Make room for the big argument up front.
				remaining := len(c.querybuf) - c.qbPos
				if remaining <= int(ll)+2 {
					nb := make([]byte, remaining, int(ll)+2)
					copy(nb, c.querybuf[c.qbPos:])
					c.querybuf = nb
					c.qbPos = 0
				}
			}
			c.bulklen = int(ll)
		}
		if len(c.querybuf)-c.qbPos < c.bulklen+2 {
			break // not enough data
		}
		arg := make([]byte, c.bulklen)
		copy(arg, c.querybuf[c.qbPos:c.qbPos+c.bulklen])
		c.argv = append(c.argv, arg)
		c.qbPos += c.bulklen + 2
		c.bulklen = -1
		c.multibulklen--
	}
	return c.multibulklen == 0
}

// processInputBuffer executes every complete command in the query buffer.
func (c *client) processInputBuffer() {
	for c.qbPos < len(c.querybuf) {
		if c.closeAfterReply {
			break
		}
		if c.reqtype == 0 {
			if c.querybuf[c.qbPos] == '*' {
				c.reqtype = protoReqMultibulk
			} else {
				c.reqtype = protoReqInline
			}
		}
		var ready bool
		if c.reqtype == protoReqInline {
			ready = c.processInlineBuffer()
		} else {
			ready = c.processMultibulkBuffer()
		}
		if !ready {
			break
		}
		if len(c.argv) == 0 {
			c.resetClient()
		} else {
			processCommand(c)
			c.resetClient()
		}
	}
	// Trim the consumed part of the query buffer.
	if c.qbPos > 0 {
		n := copy(c.querybuf, c.querybuf[c.qbPos:])
		c.querybuf = c.querybuf[:n]
		c.qbPos = 0
	}
}

// serve is the per-connection read/process/write loop.
func (c *client) serve() {
	defer c.conn.Close()
	readbuf := make([]byte, protoIOBufLen)
	for {
		readlen := protoIOBufLen
		buf := readbuf
		// For a big pending bulk argument, read directly into spare capacity.
		if c.reqtype == protoReqMultibulk && c.multibulklen > 0 && c.bulklen >= protoMbulkBigArg {
			remaining := c.bulklen + 2 - (len(c.querybuf) - c.qbPos)
			if remaining > 0 && remaining < readlen {
				readlen = remaining
			}
		}
		n, err := c.conn.Read(buf[:readlen])
		if n > 0 {
			c.querybuf = append(c.querybuf, buf[:n]...)
			c.processInputBuffer()
			if ferr := c.w.Flush(); ferr != nil {
				return
			}
			if c.closeAfterReply {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
