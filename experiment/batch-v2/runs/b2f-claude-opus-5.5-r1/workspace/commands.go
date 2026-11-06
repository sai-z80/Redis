package main

// Command table and implementations: PING, ECHO, SET, GET, DEL.
// Ported from server.c (command dispatch), t_string.c and db.c.

import (
	"bytes"
	"fmt"
	"math"
	"strings"
)

type redisCommand struct {
	name  string // lowercase full name, used in error messages
	arity int    // >0 exact argc, <0 minimum argc (-N)
	proc  func(c *client)
}

var commandTable map[string]*redisCommand

func init() {
	cmds := []*redisCommand{
		{"ping", -1, pingCommand},
		{"echo", 2, echoCommand},
		{"set", -3, setCommand},
		{"get", 2, getCommand},
		{"del", -2, delCommand},
	}
	commandTable = make(map[string]*redisCommand, len(cmds))
	for _, c := range cmds {
		commandTable[c.name] = c
	}
}

func lookupCommand(name []byte) *redisCommand {
	return commandTable[strings.ToLower(string(name))]
}

// cstr truncates b at the first NUL byte, emulating how C "%s" formats
// treat the (NUL-terminated) sds strings.
func cstr(b []byte) []byte {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return b[:i]
	}
	return b
}

func truncate(b []byte, n int) []byte {
	if n < 0 {
		n = 0
	}
	if len(b) > n {
		return b[:n]
	}
	return b
}

// processCommand looks up and executes the command in c.argv, performing
// the unknown-command and arity checks of server.c processCommand().
func processCommand(c *client) {
	cmd := lookupCommand(c.argv[0])
	if cmd == nil {
		var args []byte
		for i := 1; i < len(c.argv) && len(args) < 128; i++ {
			args = append(args, '\'')
			args = append(args, truncate(cstr(c.argv[i]), 128-len(args)+1)...)
			args = append(args, '\'', ' ')
		}
		c.addReplyErrorFormat("unknown command '%s', with args beginning with: %s",
			truncate(cstr(c.argv[0]), 128), args)
		return
	}
	c.cmd = cmd
	argc := len(c.argv)
	if (cmd.arity > 0 && cmd.arity != argc) || argc < -cmd.arity {
		c.addReplyErrorArity()
		return
	}
	db := c.srv.db
	db.mu.Lock()
	c.cmdTime = mstime()
	cmd.proc(c)
	db.mu.Unlock()
}

// PING [message]
func pingCommand(c *client) {
	if len(c.argv) > 2 {
		c.addReplyErrorArity()
		return
	}
	if len(c.argv) == 1 {
		c.addReplyStatus("PONG")
	} else {
		c.addReplyBulk(c.argv[1])
	}
}

// ECHO message
func echoCommand(c *client) {
	c.addReplyBulk(c.argv[1])
}

const (
	objNoFlags = 0
	objEX      = 1 << 2
	objPX      = 1 << 3
)

const (
	unitSeconds      = 0
	unitMilliseconds = 1
)

func eqFold(a []byte, s string) bool {
	return len(a) == len(s) && strings.EqualFold(string(a), s)
}

// parseExtendedStringArgumentsOrReply: the subset supporting EX and PX.
// Any other option (including NX/XX/GET/KEEPTTL/EXAT/PXAT, which are out
// of scope) is a syntax error.
func parseSetArgs(c *client) (flags int, unit int, expire []byte, ok bool) {
	unit = unitSeconds
	argv := c.argv
	for j := 3; j < len(argv); j++ {
		opt := cstr(argv[j])
		var next []byte
		hasNext := j != len(argv)-1
		if hasNext {
			next = argv[j+1]
		}
		if eqFold(opt, "ex") && flags&objPX == 0 && hasNext {
			flags |= objEX
			expire = next
			j++
		} else if eqFold(opt, "px") && flags&objEX == 0 && hasNext {
			flags |= objPX
			unit = unitMilliseconds
			expire = next
			j++
		} else {
			c.addReplyError("syntax error")
			return 0, 0, nil, false
		}
	}
	return flags, unit, expire, true
}

// getExpireMillisecondsOrReply converts the expire argument into an
// absolute unix time in milliseconds.
func getExpireMillisecondsOrReply(c *client, expire []byte, flags int, unit int) (int64, bool) {
	ms, ok := string2ll(expire)
	if !ok {
		c.addReplyError("value is not an integer or out of range")
		return 0, false
	}
	if ms <= 0 || (unit == unitSeconds && ms > math.MaxInt64/1000) {
		c.addReplyErrorExpireTime()
		return 0, false
	}
	if unit == unitSeconds {
		ms *= 1000
	}
	if flags&(objPX|objEX) != 0 {
		// Overflow wraps negative, as in C (detected below).
		ms = int64(uint64(ms) + uint64(c.cmdTime))
	}
	if ms <= 0 {
		c.addReplyErrorExpireTime()
		return 0, false
	}
	return ms, true
}

// SET key value [EX seconds | PX milliseconds]
func setCommand(c *client) {
	flags, unit, expire, ok := parseSetArgs(c)
	if !ok {
		return
	}
	var when int64
	if expire != nil {
		if when, ok = getExpireMillisecondsOrReply(c, expire, flags, unit); !ok {
			return
		}
	}
	db := c.srv.db
	key := string(c.argv[1])
	db.lookupKey(key, c.cmdTime) // lookupKeyWrite: lazily expire
	val := make([]byte, len(c.argv[2]))
	copy(val, c.argv[2])
	db.setKey(key, val, expire != nil)
	if expire != nil {
		db.setExpire(key, when)
	}
	c.addReplyStatus("OK")
}

// GET key
func getCommand(c *client) {
	e := c.srv.db.lookupKey(string(c.argv[1]), c.cmdTime)
	if e == nil {
		c.addReplyNull()
		return
	}
	c.addReplyBulk(e.val)
}

// DEL key [key ...]
func delCommand(c *client) {
	db := c.srv.db
	numdel := int64(0)
	for _, k := range c.argv[1:] {
		key := string(k)
		db.expireIfNeeded(key, c.cmdTime)
		if db.syncDelete(key) {
			numdel++
		}
	}
	c.addReplyLongLong(numdel)
}

var _ = fmt.Sprintf
