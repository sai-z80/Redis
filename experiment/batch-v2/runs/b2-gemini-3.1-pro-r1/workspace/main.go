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
)

func writeSimpleString(w io.Writer, s string) {
	fmt.Fprintf(w, "+%s\r\n", s)
}

func writeError(w io.Writer, s string) {
	fmt.Fprintf(w, "-%s\r\n", s)
}

func writeInteger(w io.Writer, i int) {
	fmt.Fprintf(w, ":%d\r\n", i)
}

func writeBulkString(w io.Writer, b []byte) {
	fmt.Fprintf(w, "$%d\r\n", len(b))
	w.Write(b)
	w.Write([]byte("\r\n"))
}

func writeNull(w io.Writer) {
	fmt.Fprintf(w, "$-1\r\n")
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	rr := &RESPReader{r: bufio.NewReader(conn)}

	for {
		cmdBytes, err := rr.ReadCommand()
		if err != nil {
			return
		}

		if len(cmdBytes) == 0 {
			continue
		}

		cmd := strings.ToUpper(string(cmdBytes[0]))

		switch cmd {
		case "PING":
			if len(cmdBytes) > 2 {
				writeError(conn, "ERR wrong number of arguments for 'ping' command")
			} else if len(cmdBytes) == 2 {
				writeBulkString(conn, cmdBytes[1])
			} else {
				writeSimpleString(conn, "PONG")
			}
		case "ECHO":
			if len(cmdBytes) != 2 {
				writeError(conn, "ERR wrong number of arguments for 'echo' command")
			} else {
				writeBulkString(conn, cmdBytes[1])
			}
		case "SET":
			if len(cmdBytes) < 3 {
				writeError(conn, "ERR wrong number of arguments for 'set' command")
				continue
			}
			key := string(cmdBytes[1])
			val := cmdBytes[2]

			var expiresAt int64 = 0
			var hasExpire bool
			var syntaxError bool
			var invalidExpire bool
			var notInteger bool

			for i := 3; i < len(cmdBytes); i++ {
				opt := strings.ToUpper(string(cmdBytes[i]))
				if opt == "EX" || opt == "PX" {
					if hasExpire {
						syntaxError = true
						break
					}
					hasExpire = true
					if i+1 >= len(cmdBytes) {
						syntaxError = true
						break
					}
					timeValStr := string(cmdBytes[i+1])
					timeVal, err := strconv.ParseInt(timeValStr, 10, 64)
					if err != nil {
						notInteger = true
						break
					}
					if timeVal <= 0 {
						invalidExpire = true
						break
					}

					if opt == "EX" {
						expiresAt = nowMs() + timeVal*1000
					} else {
						expiresAt = nowMs() + timeVal
					}
					i++ // skip the value
				} else {
					syntaxError = true
					break
				}
			}

			if syntaxError {
				writeError(conn, "ERR syntax error")
			} else if notInteger {
				writeError(conn, "ERR value is not an integer or out of range")
			} else if invalidExpire {
				writeError(conn, "ERR invalid expire time in 'set' command")
			} else {
				dbSet(key, val, expiresAt)
				writeSimpleString(conn, "OK")
			}

		case "GET":
			if len(cmdBytes) != 2 {
				writeError(conn, "ERR wrong number of arguments for 'get' command")
				continue
			}
			key := string(cmdBytes[1])
			item, ok := dbGet(key)
			if !ok {
				writeNull(conn)
			} else {
				writeBulkString(conn, item.Value)
			}
		case "DEL":
			if len(cmdBytes) < 2 {
				writeError(conn, "ERR wrong number of arguments for 'del' command")
				continue
			}
			var keys []string
			for i := 1; i < len(cmdBytes); i++ {
				keys = append(keys, string(cmdBytes[i]))
			}
			count := dbDel(keys)
			writeInteger(conn, count)
		default:
			writeError(conn, fmt.Sprintf("ERR unknown command '%s'", strings.ToLower(string(cmdBytes[0]))))
		}
	}
}

func main() {
	port := flag.Int("port", 6380, "port to listen on")
	flag.Parse()

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen error: %v\n", err)
		os.Exit(1)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn)
	}
}
