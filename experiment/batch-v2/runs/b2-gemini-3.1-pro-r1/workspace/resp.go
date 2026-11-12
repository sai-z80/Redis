package main

import (
	"bufio"
	"errors"
	"io"
	"strconv"
)

type RESPReader struct {
	r *bufio.Reader
}

func (rr *RESPReader) readLine() ([]byte, error) {
	line, err := rr.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) >= 2 && line[len(line)-2] == '\r' {
		return line[:len(line)-2], nil
	}
	return line[:len(line)-1], nil
}

func (rr *RESPReader) ReadCommand() ([][]byte, error) {
	line, err := rr.readLine()
	if err != nil {
		return nil, err
	}
	if len(line) == 0 || line[0] != '*' {
		return nil, errors.New("expected array")
	}
	numArgs, err := strconv.Atoi(string(line[1:]))
	if err != nil {
		return nil, err
	}
	if numArgs < 0 {
		return nil, errors.New("invalid array length")
	}

	args := make([][]byte, numArgs)
	for i := 0; i < numArgs; i++ {
		line, err = rr.readLine()
		if err != nil {
			return nil, err
		}
		if len(line) == 0 || line[0] != '$' {
			return nil, errors.New("expected bulk string")
		}
		length, err := strconv.Atoi(string(line[1:]))
		if err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, errors.New("invalid bulk string length")
		}

		data := make([]byte, length+2)
		_, err = io.ReadFull(rr.r, data)
		if err != nil {
			return nil, err
		}
		if data[length] != '\r' || data[length+1] != '\n' {
			return nil, errors.New("expected CRLF")
		}
		args[i] = data[:length]
	}
	return args, nil
}
