package transcoding

import (
	"testing"
	"time"
)

func TestStreamHashRegistry_FirstSightSupersedesNothing(t *testing.T) {
	r := NewStreamHashRegistry(time.Minute, 16)
	if old, ok := r.Bind("tok-a", "media-1", "hashA"); ok || old != "" {
		t.Fatalf("first sight must supersede nothing, got (%q, %v)", old, ok)
	}
	if got := r.Holders("hashA"); got != 1 {
		t.Fatalf("Holders(hashA) = %d, want 1", got)
	}
}

func TestStreamHashRegistry_SameHashAgainIsNoop(t *testing.T) {
	r := NewStreamHashRegistry(time.Minute, 16)
	r.Bind("tok-a", "media-1", "hashA")
	if old, ok := r.Bind("tok-a", "media-1", "hashA"); ok || old != "" {
		t.Fatalf("rebinding the same hash must supersede nothing, got (%q, %v)", old, ok)
	}
	if got := r.Holders("hashA"); got != 1 {
		t.Fatalf("Holders(hashA) = %d, want 1 (a rebind must not double-count)", got)
	}
}

// The bug this whole type exists for: ONE viewer switching audio track
// re-fingerprints the same viewing session. The hash they just left is dead
// weight holding a video-encode slot, and nobody else is on it.
func TestStreamHashRegistry_TrackSwitchSupersedesTheAbandonedHash(t *testing.T) {
	r := NewStreamHashRegistry(time.Minute, 16)
	r.Bind("tok-a", "media-1", "hashA")

	old, ok := r.Bind("tok-a", "media-1", "hashB")
	if !ok || old != "hashA" {
		t.Fatalf("a hash change on the same stream must supersede the old one, got (%q, %v)", old, ok)
	}
	if got := r.Holders("hashA"); got != 0 {
		t.Fatalf("Holders(hashA) = %d, want 0", got)
	}
	if got := r.Holders("hashB"); got != 1 {
		t.Fatalf("Holders(hashB) = %d, want 1", got)
	}
}

// A second tab / second viewer on the SAME hash must protect it: reclaiming
// there would kill an encoder somebody is actively consuming.
func TestStreamHashRegistry_AnotherStreamStillOnTheOldHashProtectsIt(t *testing.T) {
	r := NewStreamHashRegistry(time.Minute, 16)
	r.Bind("tok-a", "media-1", "hashA")
	r.Bind("tok-b", "media-1", "hashA")

	if old, ok := r.Bind("tok-a", "media-1", "hashB"); ok || old != "" {
		t.Fatalf("a hash another stream still holds must NOT be superseded, got (%q, %v)", old, ok)
	}
	if got := r.Holders("hashA"); got != 1 {
		t.Fatalf("Holders(hashA) = %d, want 1 (tok-b still on it)", got)
	}

	// Once the last holder leaves, it becomes reclaimable.
	if old, ok := r.Bind("tok-b", "media-1", "hashC"); !ok || old != "hashA" {
		t.Fatalf("the LAST holder leaving must supersede, got (%q, %v)", old, ok)
	}
}

// A viewer moving to a different title abandons the previous title's encoder
// just as completely as a track switch does.
func TestStreamHashRegistry_MediaChangeAlsoSupersedes(t *testing.T) {
	r := NewStreamHashRegistry(time.Minute, 16)
	r.Bind("tok-a", "media-1", "hashA")
	if old, ok := r.Bind("tok-a", "media-2", "hashZ"); !ok || old != "hashA" {
		t.Fatalf("moving to another media must supersede the old hash, got (%q, %v)", old, ok)
	}
}

func TestStreamHashRegistry_ExpiredStreamReleasesItsHold(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	r := NewStreamHashRegistry(30*time.Second, 16)
	r.now = func() time.Time { return now }

	r.Bind("tok-gone", "media-1", "hashA")
	r.Bind("tok-a", "media-1", "hashA")
	if got := r.Holders("hashA"); got != 2 {
		t.Fatalf("Holders(hashA) = %d, want 2", got)
	}

	// tok-gone never comes back; tok-a keeps streaming past the TTL and then
	// switches track. The dead stream's hold must not protect hashA forever.
	now = now.Add(20 * time.Second)
	r.Bind("tok-a", "media-1", "hashA")
	now = now.Add(20 * time.Second) // tok-gone is now 40 s stale, tok-a 20 s

	old, ok := r.Bind("tok-a", "media-1", "hashB")
	if !ok || old != "hashA" {
		t.Fatalf("an expired holder must not protect the hash, got (%q, %v)", old, ok)
	}
}

func TestStreamHashRegistry_ForgetReleasesTheHold(t *testing.T) {
	r := NewStreamHashRegistry(time.Minute, 16)
	r.Bind("tok-a", "media-1", "hashA")
	r.Bind("tok-b", "media-1", "hashA")
	r.Forget("tok-b")
	if got := r.Holders("hashA"); got != 1 {
		t.Fatalf("Holders(hashA) = %d, want 1 after Forget", got)
	}
	if old, ok := r.Bind("tok-a", "media-1", "hashB"); !ok || old != "hashA" {
		t.Fatalf("after Forget the remaining holder leaving must supersede, got (%q, %v)", old, ok)
	}
	r.Forget("nope") // unknown key is a no-op, never a corrupting decrement
}

func TestStreamHashRegistry_EmptyInputsAreIgnored(t *testing.T) {
	r := NewStreamHashRegistry(time.Minute, 16)
	// A request with no stream token (share links, hand-built tests) must never
	// collapse every anonymous viewer onto one registry entry.
	if old, ok := r.Bind("", "media-1", "hashA"); ok || old != "" {
		t.Fatalf("empty stream key must be ignored, got (%q, %v)", old, ok)
	}
	if old, ok := r.Bind("tok-a", "media-1", ""); ok || old != "" {
		t.Fatalf("empty hash must be ignored, got (%q, %v)", old, ok)
	}
	if got := r.Holders("hashA"); got != 0 {
		t.Fatalf("Holders(hashA) = %d, want 0", got)
	}
}

// The map is bounded: an abandoned-attempt flood must not grow it without end,
// and evicting a stream must give its hold back rather than leak it.
func TestStreamHashRegistry_BoundedAndEvictionReleasesHolds(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	r := NewStreamHashRegistry(time.Hour, 4)
	r.now = func() time.Time { now = now.Add(time.Second); return now }

	for _, k := range []string{"t1", "t2", "t3", "t4", "t5", "t6"} {
		r.Bind(k, "media-1", "hash-"+k)
	}
	if got := r.Len(); got > 4 {
		t.Fatalf("Len() = %d, want <= 4", got)
	}
	// t1 was the coldest: evicted, and its hold released with it.
	if got := r.Holders("hash-t1"); got != 0 {
		t.Fatalf("Holders(hash-t1) = %d, want 0 after eviction", got)
	}
}

func TestStreamHashRegistry_NilReceiverIsSafe(t *testing.T) {
	var r *StreamHashRegistry
	if old, ok := r.Bind("tok", "media", "hash"); ok || old != "" {
		t.Fatalf("nil registry must be inert, got (%q, %v)", old, ok)
	}
	r.Forget("tok")
	if got := r.Holders("hash"); got != 0 {
		t.Fatalf("nil registry Holders = %d, want 0", got)
	}
}

func TestStreamHashRegistry_ConcurrentBindIsRaceFree(t *testing.T) {
	r := NewStreamHashRegistry(time.Minute, 256)
	done := make(chan struct{})
	for i := range 8 {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			key := string(rune('a' + i))
			for j := range 200 {
				r.Bind(key, "media-1", string(rune('A'+j%5)))
			}
		}(i)
	}
	for range 8 {
		<-done
	}
}
