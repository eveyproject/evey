package cache

import (
	"log/slog"
	"sync"
	"time"
)

const defaultMaxSize = 10000

type entry[T any] struct {
	val       T
	expiresAt time.Time
	createdAt time.Time
}

// TTLCache is a generic in-memory cache with per-entry TTL and max size.
// When maxSize is exceeded, the oldest entries are evicted first.
type TTLCache[T any] struct {
	m       sync.Map
	maxSize int
	mu      sync.Mutex
	count   int
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
	c := &TTLCache[T]{maxSize: maxSize}
	if cleanupInterval <= 0 {
		return c, func() {}
	}
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
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
	v, ok := c.m.Load(key)
	if !ok {
		var zero T
		return zero, false
	}
	e := v.(entry[T])
	if time.Now().After(e.expiresAt) {
		c.mu.Lock()
		if _, still := c.m.LoadAndDelete(key); still {
			c.count--
		}
		c.mu.Unlock()
		var zero T
		return zero, false
	}
	return e.val, true
}

func (c *TTLCache[T]) Set(key string, val T, ttl time.Duration) {
	now := time.Now()

	c.mu.Lock()
	if _, loaded := c.m.Load(key); !loaded {
		c.count++
	}
	c.m.Store(key, entry[T]{val: val, expiresAt: now.Add(ttl), createdAt: now})
	if c.count > c.maxSize {
		c.evictOldest()
	}
	c.mu.Unlock()
}

func (c *TTLCache[T]) evictOldest() {
	var oldestKey any
	var oldestTime time.Time
	first := true
	c.m.Range(func(k, v any) bool {
		e := v.(entry[T])
		if first || e.createdAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.createdAt
			first = false
		}
		return true
	})
	if oldestKey != nil {
		c.m.Delete(oldestKey)
		c.count--
	}
}

func (c *TTLCache[T]) Delete(key string) {
	c.mu.Lock()
	if _, loaded := c.m.LoadAndDelete(key); loaded {
		c.count--
	}
	c.mu.Unlock()
}

func (c *TTLCache[T]) evict() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m.Range(func(k, v any) bool {
		if e, ok := v.(entry[T]); ok && now.After(e.expiresAt) {
			c.m.Delete(k)
			c.count--
		}
		return true
	})
}
