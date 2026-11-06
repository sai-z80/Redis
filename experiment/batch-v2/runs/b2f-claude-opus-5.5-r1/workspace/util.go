package main

// Ported from Redis util.c / sds.c helpers.

import "math"

// longStrSize mirrors LONG_STR_SIZE (21) from util.h.
const longStrSize = 21

// string2ll converts a byte string into a long long (int64) with the exact
// strictness of Redis' string2ll(): no leading/trailing spaces, no '+',
// no leading zeros, and overflow detection.
func string2ll(s []byte) (int64, bool) {
	slen := len(s)
	if slen == 0 || slen >= longStrSize {
		return 0, false
	}
	if slen == 1 && s[0] == '0' {
		return 0, true
	}
	p := 0
	negative := false
	if s[0] == '-' {
		negative = true
		p++
		if p == slen {
			return 0, false
		}
	}
	var v uint64
	if s[p] >= '1' && s[p] <= '9' {
		v = uint64(s[p] - '0')
		p++
	} else {
		return 0, false
	}
	for p < slen && s[p] >= '0' && s[p] <= '9' {
		if v > math.MaxUint64/10 {
			return 0, false
		}
		v *= 10
		d := uint64(s[p] - '0')
		if v > math.MaxUint64-d {
			return 0, false
		}
		v += d
		p++
	}
	if p < slen {
		return 0, false
	}
	if negative {
		if v > uint64(-(math.MinInt64+1))+1 {
			return 0, false
		}
		return int64(-v), true
	}
	if v > math.MaxInt64 {
		return 0, false
	}
	return int64(v), true
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexDigitToInt(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// splitArgs is a port of sdssplitargs(): it splits an inline command line
// into arguments, supporting "double quoted" strings with escapes and
// 'single quoted' strings. Returns ok=false on unbalanced quotes.
// As in C, a NUL byte terminates the line.
func splitArgs(line []byte) ([][]byte, bool) {
	at := func(i int) byte {
		if i < len(line) {
			return line[i]
		}
		return 0
	}
	p := 0
	var vector [][]byte
	for {
		for at(p) != 0 && isSpace(at(p)) {
			p++
		}
		if at(p) == 0 {
			if vector == nil {
				vector = [][]byte{}
			}
			return vector, true
		}
		inq, insq, done := false, false, false
		current := []byte{}
		for !done {
			c := at(p)
			if inq {
				if c == '\\' && at(p+1) == 'x' && isHexDigit(at(p+2)) && isHexDigit(at(p+3)) {
					current = append(current, hexDigitToInt(at(p+2))*16+hexDigitToInt(at(p+3)))
					p += 3
				} else if c == '\\' && at(p+1) != 0 {
					p++
					var b byte
					switch at(p) {
					case 'n':
						b = '\n'
					case 'r':
						b = '\r'
					case 't':
						b = '\t'
					case 'b':
						b = '\b'
					case 'a':
						b = '\a'
					default:
						b = at(p)
					}
					current = append(current, b)
				} else if c == '"' {
					// closing quote must be followed by a space or nothing at all
					if at(p+1) != 0 && !isSpace(at(p+1)) {
						return nil, false
					}
					done = true
				} else if c == 0 {
					return nil, false // unterminated quotes
				} else {
					current = append(current, c)
				}
			} else if insq {
				if c == '\\' && at(p+1) == '\'' {
					p++
					current = append(current, '\'')
				} else if c == '\'' {
					if at(p+1) != 0 && !isSpace(at(p+1)) {
						return nil, false
					}
					done = true
				} else if c == 0 {
					return nil, false
				} else {
					current = append(current, c)
				}
			} else {
				switch c {
				case ' ', '\n', '\r', '\t', 0:
					done = true
				case '"':
					inq = true
				case '\'':
					insq = true
				default:
					current = append(current, c)
				}
			}
			if at(p) != 0 {
				p++
			}
		}
		vector = append(vector, current)
	}
}

// mapCRLF replaces '\r' and '\n' with spaces, like
// sdsmapchars(s, "\r\n", "  ", 2) used for error/status replies.
func mapCRLF(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c == '\r' || c == '\n' {
			b[i] = ' '
		}
	}
	return string(b)
}
