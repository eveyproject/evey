package transcoding

import (
	"testing"
	"time"
)

func TestHWRouteBlocklist_BlockExpiresAndCaps(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	b := NewHWRouteBlocklist(time.Hour, 2)
	b.now = func() time.Time { return now }

	if b.Blocked("m1") {
		t.Fatal("fresh list blocks nothing")
	}
	b.Block("m1", "route_incompatible")
	if !b.Blocked("m1") {
		t.Fatal("m1 must be blocked")
	}
	now = now.Add(time.Hour)
	if b.Blocked("m1") {
		t.Fatal("block must expire after the TTL")
	}
	if len(b.Entries()) != 0 {
		t.Fatal("expired entries must be pruned")
	}

	b.Block("a", "x")
	now = now.Add(time.Minute)
	b.Block("b", "x")
	now = now.Add(time.Minute)
	b.Block("c", "x") // over the cap: the entry closest to expiry (a) goes
	if b.Blocked("a") || !b.Blocked("b") || !b.Blocked("c") {
		t.Fatalf("cap eviction wrong: a=%v b=%v c=%v", b.Blocked("a"), b.Blocked("b"), b.Blocked("c"))
	}
	b.Unblock("b")
	if b.Blocked("b") {
		t.Fatal("unblock must clear")
	}
	b.Clear()
	if len(b.Entries()) != 0 {
		t.Fatal("clear must empty the list")
	}
}

func TestHWRouteBlocklist_NilSafe(t *testing.T) {
	var b *HWRouteBlocklist
	b.Block("m", "x")
	if b.Blocked("m") {
		t.Fatal("nil list blocks nothing")
	}
	b.Unblock("m")
	b.Clear()
	if b.Entries() != nil {
		t.Fatal("nil list has no entries")
	}
}
