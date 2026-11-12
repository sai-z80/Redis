package main

import (
	"sync"
	"time"
)

type Item struct {
	Value     []byte
	ExpiresAt int64
}

var (
	mu    sync.Mutex
	store = make(map[string]Item)
)

func nowMs() int64 {
	return time.Now().UnixNano() / 1e6
}

func dbGet(key string) (Item, bool) {
	mu.Lock()
	defer mu.Unlock()

	item, ok := store[key]
	if !ok {
		return Item{}, false
	}
	if item.ExpiresAt > 0 && item.ExpiresAt <= nowMs() {
		delete(store, key)
		return Item{}, false
	}
	return item, true
}

func dbSet(key string, value []byte, expiresAt int64) {
	mu.Lock()
	defer mu.Unlock()
	vCopy := make([]byte, len(value))
	copy(vCopy, value)

	store[key] = Item{
		Value:     vCopy,
		ExpiresAt: expiresAt,
	}
}

func dbDel(keys []string) int {
	mu.Lock()
	defer mu.Unlock()

	count := 0
	for _, key := range keys {
		item, ok := store[key]
		if ok {
			delete(store, key)
			if item.ExpiresAt == 0 || item.ExpiresAt > nowMs() {
				count++
			}
		}
	}
	return count
}
