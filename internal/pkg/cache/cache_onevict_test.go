package cache

import (
	"slices"
	"testing"
	"time"
)

// Every way out of the cache is reported once: the size cap, a delete, an
// expiry found by Get and one swept by the janitor.
func TestTTLCache_OnEvictSeesEveryWayOut(t *testing.T) {
	c, stop := NewWithMaxSize[int](0, 2)
	stop()
	var gone []string
	c.OnEvict(func(k string, _ int) { gone = append(gone, k) })

	c.Set("a", 1, time.Hour)
	c.Set("b", 2, time.Hour)
	c.Set("c", 3, time.Hour) // over the cap: "a" leaves
	c.Delete("b")
	c.Set("d", 4, -time.Second) // already expired
	if _, ok := c.Get("d"); ok {
		t.Fatal("an expired entry was served")
	}
	c.Set("e", 5, -time.Second)
	if n := len(c.Held()); n != 2 {
		t.Fatalf("Held = %d entries, want c and the expired e", n)
	}
	c.evict()
	if want := []string{"a", "b", "d", "e"}; !slices.Equal(gone, want) {
		t.Fatalf("evicted %v, want %v", gone, want)
	}
}
