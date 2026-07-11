package httpapi

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
)

// countingAccessChecker counts calls to HasAccess. Goroutine-safe.
type countingAccessChecker struct {
	allowed bool
	err     error
	calls   int64
}

func (c *countingAccessChecker) HasAccess(_ context.Context, _ shared.UserID, _ shared.LibraryID) (bool, error) {
	atomic.AddInt64(&c.calls, 1)
	return c.allowed, c.err
}

func TestHLSCacheAccessChecker_CachesResult(t *testing.T) {
	inner := &countingAccessChecker{allowed: true}
	ch := NewCachedLibraryAccessChecker(inner)
	defer ch.Close()

	for i := 0; i < 3; i++ {
		ok, err := ch.HasAccess(context.Background(), shared.UserID("u1"), shared.LibraryID("lib1"))
		if err != nil || !ok {
			t.Fatalf("want allowed, got ok=%v err=%v", ok, err)
		}
	}
	if got := atomic.LoadInt64(&inner.calls); got != 1 {
		t.Fatalf("expected 1 inner call, got %d", got)
	}
}

func TestHLSCacheAccessChecker_KeyedByUserAndLibrary(t *testing.T) {
	inner := &countingAccessChecker{allowed: true}
	ch := NewCachedLibraryAccessChecker(inner)
	defer ch.Close()

	_, _ = ch.HasAccess(context.Background(), "u1", "lib1")
	_, _ = ch.HasAccess(context.Background(), "u2", "lib1")
	_, _ = ch.HasAccess(context.Background(), "u1", "lib2")
	_, _ = ch.HasAccess(context.Background(), "u1", "lib1")
	if got := atomic.LoadInt64(&inner.calls); got != 3 {
		t.Fatalf("expected 3 distinct inner calls, got %d", got)
	}
}

func TestHLSCacheAccessChecker_ErrorNotCached(t *testing.T) {
	inner := &countingAccessChecker{err: errors.New("db down")}
	ch := NewCachedLibraryAccessChecker(inner)
	defer ch.Close()

	if _, err := ch.HasAccess(context.Background(), "u1", "lib1"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ch.HasAccess(context.Background(), "u1", "lib1"); err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt64(&inner.calls); got != 2 {
		t.Fatalf("errors should not be cached, got %d calls", got)
	}
}

// Concurrent callers must see a consistent cache with no race condition.
// Run with -race to catch data races. The inner should be called at most
// N times where N ≤ goroutines (first racers may all miss the cache).
func TestHLSCacheAccessChecker_ConcurrentSameKey(t *testing.T) {
	inner := &countingAccessChecker{allowed: true}
	ch := NewCachedLibraryAccessChecker(inner)
	defer ch.Close()

	const goroutines = 50
	const iterations = 20
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ok, err := ch.HasAccess(context.Background(), "u1", "lib1")
				if err != nil || !ok {
					t.Errorf("unexpected: ok=%v err=%v", ok, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt64(&inner.calls); got > goroutines {
		t.Fatalf("inner called too many times: %d (max %d)", got, goroutines)
	}
}
