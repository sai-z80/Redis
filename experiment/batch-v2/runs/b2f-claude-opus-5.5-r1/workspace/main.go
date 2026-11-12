// Command redis-mini is a small Go port of a subset of Redis 7.2:
// RESP2 over TCP with PING, ECHO, SET (EX/PX), GET and DEL, and lazy
// expiry. Derived from the Redis source (BSD-3-Clause, see
// upstream/COPYING).
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
)

type server struct {
	db      *redisDb
	verbose bool
	logger  *log.Logger
}

func (s *server) logf(format string, a ...interface{}) {
	s.logger.Printf(format, a...)
}

func main() {
	port := flag.Int("port", 6380, "TCP port to listen on")
	bind := flag.String("bind", "", "address to bind (default: all interfaces)")
	verbose := flag.Bool("verbose", false, "verbose logging")
	flag.Parse()

	s := &server{
		db:      newDb(),
		verbose: *verbose,
		logger:  log.New(os.Stderr, "", log.LstdFlags),
	}

	addr := net.JoinHostPort(*bind, strconv.Itoa(*port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create server TCP listening socket %s: %v\n", addr, err)
		os.Exit(1)
	}
	s.logf("Ready to accept connections tcp on %s", ln.Addr())
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			s.logf("Accepting client connection: %v", err)
			continue
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			tc.SetNoDelay(true)
		}
		c := newClient(s, conn)
		go c.serve()
	}
}
