package cache

import (
	"log/slog"
	"sync"
	"time"
)

type entry[T any] struct {
	val     T
	expiresAt time.Time
}

// TTLCache is a generic in-memory cache with per-entry TTL.
// Safe for concurrent use. No LRU, no size limit.
type TTLCache[T any] struct {
	m sync.Map
}

// New returns a TTLCache that evicts expired entries every cleanupInterval.
// Stop the returned stop func when done (e.g. on shutdown). May be nil for tests.
func New[T any](cleanupInterval time.Duration) (*TTLCache[T], func()) {
	c := &TTLCache[T]{}
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
		c.m.Delete(key)
		var zero T
		return zero, false
	}
	return e.val, true
}

func (c *TTLCache[T]) Set(key string, val T, ttl time.Duration) {
	c.m.Store(key, entry[T]{val: val, expiresAt: time.Now().Add(ttl)})
}

func (c *TTLCache[T]) Delete(key string) {
	c.m.Delete(key)
}

func (c *TTLCache[T]) evict() {
	now := time.Now()
	c.m.Range(func(k, v any) bool {
		if e, ok := v.(entry[T]); ok && now.After(e.expiresAt) {
			c.m.Delete(k)
		}
		return true
	})
}
