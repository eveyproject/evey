package cache_test

import (
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/pkg/cache"
)

func TestGetMiss(t *testing.T) {
	c, stop := cache.New[string](0)
	defer stop()
	_, ok := c.Get("missing")
	if ok {
		t.Fatal("expected miss")
	}
}

func TestSetGet(t *testing.T) {
	c, stop := cache.New[string](0)
	defer stop()
	c.Set("k", "v", time.Minute)
	v, ok := c.Get("k")
	if !ok || v != "v" {
		t.Fatalf("expected hit got ok=%v v=%q", ok, v)
	}
}

func TestExpiry(t *testing.T) {
	c, stop := cache.New[string](0)
	defer stop()
	c.Set("k", "v", time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected expired entry to miss")
	}
}

func TestDelete(t *testing.T) {
	c, stop := cache.New[string](0)
	defer stop()
	c.Set("k", "v", time.Minute)
	c.Delete("k")
	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected miss after delete")
	}
}

func TestBackgroundCleanup(t *testing.T) {
	c, stop := cache.New[int](10 * time.Millisecond)
	defer stop()
	c.Set("x", 1, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	_, ok := c.Get("x")
	if ok {
		t.Fatal("expected cleanup to evict expired entry")
	}
}

func TestOverwrite(t *testing.T) {
	c, stop := cache.New[int](0)
	defer stop()
	c.Set("k", 1, time.Minute)
	c.Set("k", 2, time.Minute)
	v, ok := c.Get("k")
	if !ok || v != 2 {
		t.Fatalf("expected 2 got ok=%v v=%d", ok, v)
	}
}

func TestMaxSize_EvictsOldest(t *testing.T) {
	c, stop := cache.NewWithMaxSize[int](0, 3)
	defer stop()
	c.Set("a", 1, time.Minute)
	time.Sleep(time.Millisecond)
	c.Set("b", 2, time.Minute)
	time.Sleep(time.Millisecond)
	c.Set("c", 3, time.Minute)
	time.Sleep(time.Millisecond)
	c.Set("d", 4, time.Minute)

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected oldest entry 'a' to be evicted")
	}
	if v, ok := c.Get("d"); !ok || v != 4 {
		t.Fatalf("expected 'd'=4, got ok=%v v=%d", ok, v)
	}
}

func TestMaxSize_OverwriteDoesNotInflateCount(t *testing.T) {
	c, stop := cache.NewWithMaxSize[int](0, 2)
	defer stop()
	c.Set("a", 1, time.Minute)
	c.Set("b", 2, time.Minute)
	c.Set("a", 10, time.Minute)
	// overwrite should not trigger eviction
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Fatalf("expected 'b'=2, got ok=%v v=%d", ok, v)
	}
	if v, ok := c.Get("a"); !ok || v != 10 {
		t.Fatalf("expected 'a'=10, got ok=%v v=%d", ok, v)
	}
}

func TestNewWithMaxSize_DefaultOnZero(t *testing.T) {
	c, stop := cache.NewWithMaxSize[int](0, 0)
	defer stop()
	// should not panic, uses default maxSize
	c.Set("k", 1, time.Minute)
	if v, ok := c.Get("k"); !ok || v != 1 {
		t.Fatal("expected hit")
	}
}
