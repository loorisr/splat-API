package main

import (
	"log"
	"sync"
	"time"
)

// taskTTL is how long task data remains in the in-memory store before cleanup.
const taskTTL = 300 * time.Second

// TaskStore is a thread-safe in-memory key-value store with TTL-based expiration.
//
// Every entry is automatically removed after taskTTL (5 minutes). A background
// goroutine sweeps expired entries every 60 seconds to keep memory usage bounded.
type TaskStore struct {
	mu    sync.RWMutex
	store map[string]*storeEntry
}

type storeEntry struct {
	value   interface{}
	expires time.Time
}

// NewTaskStore creates a TaskStore and starts the background cleanup goroutine.
func NewTaskStore() *TaskStore {
	ts := &TaskStore{
		store: make(map[string]*storeEntry),
	}
	go ts.cleanupLoop()
	return ts
}

// Set stores a value under key with a fresh TTL.
func (ts *TaskStore) Set(key string, value interface{}) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.store[key] = &storeEntry{
		value:   value,
		expires: time.Now().Add(taskTTL),
	}
}

// Get retrieves a value by key, returning nil if the key is absent or expired.
// Expired entries are lazily deleted on access.
func (ts *TaskStore) Get(key string) interface{} {
	ts.mu.RLock()
	entry, ok := ts.store[key]
	ts.mu.RUnlock()
	if !ok {
		return nil
	}
	if time.Now().After(entry.expires) {
		ts.Delete(key)
		return nil
	}
	return entry.value
}

// Delete removes a key from the store unconditionally.
func (ts *TaskStore) Delete(key string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.store, key)
}

// Pop atomically retrieves and deletes a key. Returns nil if absent.
// This implements the single-consumption pattern used by /result/{task_id}.
func (ts *TaskStore) Pop(key string) interface{} {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	entry, ok := ts.store[key]
	if !ok {
		return nil
	}
	delete(ts.store, key)
	return entry.value
}

// cleanupLoop runs every 60 seconds, purging entries whose TTL has expired.
func (ts *TaskStore) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		ts.mu.Lock()
		expired := 0
		for k, v := range ts.store {
			if now.After(v.expires) {
				delete(ts.store, k)
				expired++
			}
		}
		ts.mu.Unlock()
		if expired > 0 {
			log.Printf("Task store cleanup: removed %d expired entries", expired)
		}
	}
}
