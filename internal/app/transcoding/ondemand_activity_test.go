package transcoding

import (
	"sync"
	"testing"
	"time"
)

// A media served by the on-demand packager must read as active playback for as
// long as its requests keep coming - that is the whole point: the ffmpeg
// session counters never see this path, so without it every background worker
// treats an mkvgo viewer as an idle server.
func TestOnDemandActivity_TouchCountsAsActive(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewOnDemandActivity(time.Minute)
	a.now = func() time.Time { return now }

	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("fresh tracker must report 0, got %d", got)
	}
	a.Touch("mi-1")
	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("a touched media must count as one active playback, got %d", got)
	}
}

// One viewer scrubbing through a file fires hundreds of segment requests; they
// are ONE playback, not hundreds. Counting requests instead of media would
// suspend every background worker far beyond the real load.
func TestOnDemandActivity_RepeatedTouchesOnOneMediaCountOnce(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewOnDemandActivity(time.Minute)
	a.now = func() time.Time { return now }

	for range 50 {
		a.Touch("mi-1")
		now = now.Add(time.Second)
	}
	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("repeated touches on one media must count once, got %d", got)
	}
}

func TestOnDemandActivity_DistinctMediaCountSeparately(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewOnDemandActivity(time.Minute)
	a.now = func() time.Time { return now }

	a.Touch("mi-1")
	a.Touch("mi-2")
	if got := a.ActiveSessionCount(); got != 2 {
		t.Fatalf("two media must count as two active playbacks, got %d", got)
	}
}

// The counter must fall back to zero on its own: nothing signals "playback
// stopped" on a stateless path (the client just stops asking), so a viewer who
// closes the tab would otherwise pin the drain suspended forever.
func TestOnDemandActivity_EntryExpiresAfterTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewOnDemandActivity(time.Minute)
	a.now = func() time.Time { return now }

	a.Touch("mi-1")
	now = now.Add(59 * time.Second)
	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("inside the TTL the media must still count, got %d", got)
	}
	now = now.Add(2 * time.Second)
	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("past the TTL the media must stop counting, got %d", got)
	}
}

// A player buffers ahead in bursts then goes quiet; the TTL window must be
// measured from the LAST request, not the first, or a steady viewer would flap
// in and out of "active" mid-playback.
func TestOnDemandActivity_TouchRefreshesTheWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewOnDemandActivity(time.Minute)
	a.now = func() time.Time { return now }

	a.Touch("mi-1")
	now = now.Add(50 * time.Second)
	a.Touch("mi-1")
	now = now.Add(50 * time.Second)
	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("a re-touched media must stay active, got %d", got)
	}
}

// Expired entries must not accumulate: this map is fed by every segment request
// of every viewer, so a tracker that only reads through its TTL would leak one
// entry per media ever played until restart.
func TestOnDemandActivity_ExpiredEntriesAreReclaimed(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewOnDemandActivity(time.Minute)
	a.now = func() time.Time { return now }

	for i := range 100 {
		a.Touch(string(rune('a' + i%26)))
		now = now.Add(time.Minute)
	}
	a.ActiveSessionCount()
	if got := a.size(); got > 2 {
		t.Fatalf("expired entries must be reclaimed, %d still held", got)
	}
}

// The tracker sits on the HTTP segment path: every concurrent viewer touches it
// on every request while background workers read it. It must be race-free.
func TestOnDemandActivity_ConcurrentTouchAndRead(t *testing.T) {
	a := NewOnDemandActivity(time.Minute)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for range 100 {
				a.Touch(string(rune('a' + i)))
				a.ActiveSessionCount()
			}
		})
	}
	wg.Wait()

	if got := a.ActiveSessionCount(); got != 8 {
		t.Fatalf("8 concurrently touched media must all count, got %d", got)
	}
}

// A nil tracker is the "feature not wired" case (degenerate test setups, and
// any future caller that has no on-demand path); it must read as zero, never
// panic.
func TestOnDemandActivity_NilIsZero(t *testing.T) {
	var a *OnDemandActivity
	a.Touch("mi-1")
	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("nil tracker must report 0, got %d", got)
	}
}
