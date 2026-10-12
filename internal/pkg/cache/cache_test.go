package cache_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/pkg/cache"
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

// The background sweep removes what expired and only that: a live entry
// survives it. The expired half alone proved nothing - Get hides an expired
// entry by itself - so a sweep evicting the wrong ones went unseen
// (mutation 2026-09-28).
func TestBackgroundCleanup_KeepsLiveEntries(t *testing.T) {
	c, stop := cache.New[int](10 * time.Millisecond)
	defer stop()
	c.Set("live", 1, time.Hour)
	c.Set("dead", 2, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	if _, ok := c.Get("live"); !ok {
		t.Fatal("the sweep evicted an entry that had not expired")
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
	// LRU is position-based via container/list - Set inserts to the front
	// and over-capacity removes from the back. No sleeps needed to
	// distinguish entries.
	c.Set("a", 1, time.Minute)
	c.Set("b", 2, time.Minute)
	c.Set("c", 3, time.Minute)
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

func TestMaxSize_RecentGetAvoidsEviction(t *testing.T) {
	c, stop := cache.NewWithMaxSize[int](0, 3)
	defer stop()
	c.Set("a", 1, time.Minute)
	c.Set("b", 2, time.Minute)
	c.Set("c", 3, time.Minute)
	// Access 'a' to refresh its LRU position.
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("expected 'a'=1, got ok=%v v=%d", ok, v)
	}
	// Insert 'd' → should evict 'b' (oldest after 'a' bumped), not 'a'.
	c.Set("d", 4, time.Minute)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected 'a' to survive eviction after recent Get")
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("expected 'b' (least recently used) to be evicted")
	}
}

func TestMaxSize_InsertBeyondCapEvictsFront(t *testing.T) {
	c, stop := cache.NewWithMaxSize[int](0, 2)
	defer stop()
	c.Set("a", 1, time.Minute)
	c.Set("b", 2, time.Minute)
	c.Set("c", 3, time.Minute)
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected first inserted 'a' to be evicted")
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Fatalf("expected 'c'=3, got ok=%v v=%d", ok, v)
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

// Values lists the live entries only: an expired one is not reported.
func TestTTLCache_ValuesSkipsExpired(t *testing.T) {
	c, stop := cache.New[int](0)
	defer stop()
	c.Set("live", 1, time.Hour)
	c.Set("gone", 2, -time.Second)
	got := c.Values()
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("Values() = %v, want [1]", got)
	}
}
