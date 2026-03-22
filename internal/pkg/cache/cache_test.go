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
