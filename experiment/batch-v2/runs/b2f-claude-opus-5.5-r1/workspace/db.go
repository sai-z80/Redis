package main

// A minimal port of the keyspace parts of Redis db.c / expire.c:
// a single database holding string values with optional absolute
// millisecond expire times, with lazy expiry only.

import (
	"sync"
	"time"
)

type dbEntry struct {
	val    []byte
	expire int64 // absolute unix time in ms; -1 means no expire
}

type redisDb struct {
	mu   sync.Mutex
	dict map[string]*dbEntry
}

func newDb() *redisDb {
	return &redisDb{dict: make(map[string]*dbEntry)}
}

// mstime returns the current unix time in milliseconds.
func mstime() int64 {
	return time.Now().UnixMilli()
}

// keyIsExpired mirrors keyIsExpired(): a key is expired when the command
// time snapshot is strictly greater than its expire time.
func (db *redisDb) keyIsExpired(e *dbEntry, now int64) bool {
	if e.expire < 0 {
		return false
	}
	return now > e.expire
}

// expireIfNeeded deletes the key if it is logically expired. Returns true
// if the key was expired (and removed).
func (db *redisDb) expireIfNeeded(key string, now int64) bool {
	e, ok := db.dict[key]
	if !ok {
		return false
	}
	if !db.keyIsExpired(e, now) {
		return false
	}
	delete(db.dict, key)
	return true
}

// lookupKey returns the entry for key, or nil if missing or expired.
func (db *redisDb) lookupKey(key string, now int64) *dbEntry {
	db.expireIfNeeded(key, now)
	return db.dict[key]
}

// setKey stores val at key. If keepTTL is false any existing expire is
// removed (like setKey() without SETKEY_KEEPTTL).
func (db *redisDb) setKey(key string, val []byte, keepTTL bool) {
	if e, ok := db.dict[key]; ok {
		e.val = val
		if !keepTTL {
			e.expire = -1
		}
		return
	}
	db.dict[key] = &dbEntry{val: val, expire: -1}
}

// setExpire sets an absolute expire time (ms) on an existing key.
func (db *redisDb) setExpire(key string, when int64) {
	if e, ok := db.dict[key]; ok {
		e.expire = when
	}
}

// syncDelete removes key; returns true if it existed.
func (db *redisDb) syncDelete(key string) bool {
	if _, ok := db.dict[key]; ok {
		delete(db.dict, key)
		return true
	}
	return false
}
