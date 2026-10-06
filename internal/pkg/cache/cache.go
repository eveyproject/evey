package cache

import (
	"container/list"
	"log/slog"
	"sync"
	"time"
)

const defaultMaxSize = 10000

type entry[T any] struct {
	key       string
	val       T
	expiresAt time.Time
}

// TTLCache is a generic in-memory cache with per-entry TTL and max size.
// Uses an LRU doubly-linked list for O(1) eviction; Get promotes entries
// to the front, Set evicts from the back when over capacity.
type TTLCache[T any] struct {
	mu    sync.Mutex
	items map[string]*list.Element
	// lru holds the entries newest-first. INVARIANT: every element's Value is a
	// *entry[T] - the only insertion point is lru.PushFront(&entry[T]{...}) in
	// Set, so the bare el.Value.(*entry[T]) assertions in Get/Set/Delete/evict
	// can never fail. Documented rather than ok-guarded to avoid dead branches.
	lru     *list.List
	maxSize int
}

// New returns a TTLCache that evicts expired entries every cleanupInterval.
// Stop the returned stop func when done (e.g. on shutdown). May be nil for tests.
func New[T any](cleanupInterval time.Duration) (*TTLCache[T], func()) {
	return NewWithMaxSize[T](cleanupInterval, defaultMaxSize)
}

// NewWithMaxSize returns a TTLCache with an explicit max entry count.
func NewWithMaxSize[T any](cleanupInterval time.Duration, maxSize int) (*TTLCache[T], func()) {
	if maxSize <= 0 {
		maxSize = defaultMaxSize
	}
	c := &TTLCache[T]{
		items:   make(map[string]*list.Element),
		lru:     list.New(),
		maxSize: maxSize,
	}
	if cleanupInterval <= 0 {
		return c, func() {}
	}
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// No ctx: this is the detached cleanup ticker, started once at
				// construction. It belongs to no request, so there is no
				// request_id to correlate against.
				slog.Error("recovered panic in cache cleanup", "panic", r)
			}
		}()
		t := time.NewTicker(cleanupInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				c.evict()
			}
		}
	}()
	return c, func() { close(done) }
}

func (c *TTLCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero T
		return zero, false
	}
	e := el.Value.(*entry[T])
	if time.Now().After(e.expiresAt) {
		c.lru.Remove(el)
		delete(c.items, key)
		var zero T
		return zero, false
	}
	c.lru.MoveToFront(el)
	return e.val, true
}

func (c *TTLCache[T]) Set(key string, val T, ttl time.Duration) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry[T])
		e.val = val
		e.expiresAt = now.Add(ttl)
		c.lru.MoveToFront(el)
		return
	}
	e := &entry[T]{key: key, val: val, expiresAt: now.Add(ttl)}
	el := c.lru.PushFront(e)
	c.items[key] = el
	if c.lru.Len() > c.maxSize {
		back := c.lru.Back()
		if back != nil {
			oldest := back.Value.(*entry[T])
			c.lru.Remove(back)
			delete(c.items, oldest.key)
		}
	}
}

func (c *TTLCache[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.lru.Remove(el)
		delete(c.items, key)
	}
}

// Values returns the live entries' values, newest first: a snapshot for a
// caller that sums or inspects what the cache holds.
func (c *TTLCache[T]) Values() []T {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]T, 0, c.lru.Len())
	for el := c.lru.Front(); el != nil; el = el.Next() {
		if e := el.Value.(*entry[T]); !now.After(e.expiresAt) {
			out = append(out, e.val)
		}
	}
	return out
}

func (c *TTLCache[T]) evict() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for el := c.lru.Front(); el != nil; {
		next := el.Next()
		e := el.Value.(*entry[T])
		if now.After(e.expiresAt) {
			c.lru.Remove(el)
			delete(c.items, e.key)
		}
		el = next
	}
}
