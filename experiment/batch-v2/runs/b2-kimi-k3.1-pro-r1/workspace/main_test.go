package main

import (
	"math"
	"strconv"
	"testing"
)

func TestParseRedisInt64(t *testing.T) {
	valid := []string{
		"0", "1", "9", "10", "-1", "-10",
		strconv.FormatInt(math.MaxInt64, 10),
		strconv.FormatInt(math.MinInt64, 10),
	}
	for _, s := range valid {
		got, ok := parseRedisInt64([]byte(s))
		want, _ := strconv.ParseInt(s, 10, 64)
		if !ok || got != want {
			t.Errorf("parseRedisInt64(%q) = %d, %v; want %d, true", s, got, ok, want)
		}
	}

	invalid := []string{
		"", "+", "-", "+1", "-0", "00", "01", "-01", " 1", "1 ",
		"1.0", "1x", "0x10", "9223372036854775808", "-9223372036854775809",
		"999999999999999999999999999999999999", "--1", "\x00",
	}
	for _, s := range invalid {
		if got, ok := parseRedisInt64([]byte(s)); ok {
			t.Errorf("parseRedisInt64(%q) unexpectedly accepted as %d", s, got)
		}
	}
}

func TestExpired(t *testing.T) {
	if expired(entry{}, math.MaxInt64) {
		t.Fatal("persistent entry expired")
	}
	e := entry{expiresAt: 100}
	if expired(e, 100) {
		t.Fatal("entry expired before now was greater than expiresAt")
	}
	if !expired(e, 101) {
		t.Fatal("entry did not expire")
	}
}
