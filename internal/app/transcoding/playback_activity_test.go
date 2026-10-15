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
func TestPlaybackActivity_TouchCountsAsActive(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
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
func TestPlaybackActivity_RepeatedTouchesOnOneMediaCountOnce(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
	a.now = func() time.Time { return now }

	for range 50 {
		a.Touch("mi-1")
		now = now.Add(time.Second)
	}
	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("repeated touches on one media must count once, got %d", got)
	}
}

func TestPlaybackActivity_DistinctMediaCountSeparately(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
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
func TestPlaybackActivity_EntryExpiresAfterTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
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
func TestPlaybackActivity_TouchRefreshesTheWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
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
func TestPlaybackActivity_ExpiredEntriesAreReclaimed(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
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
func TestPlaybackActivity_ConcurrentTouchAndRead(t *testing.T) {
	a := NewPlaybackActivity(time.Minute)

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

// Direct-play is a DIFFERENT traffic shape: one long-lived ServeFile that can
// block for the whole film. The TTL alone would expire mid-playback, so an
// in-flight request must count as active for as long as it runs, however long
// that is.
func TestPlaybackActivity_InFlightRequestOutlivesTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
	a.now = func() time.Time { return now }

	done := a.Begin("mi-1")
	now = now.Add(2 * time.Hour) // a long film, served by one request

	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("an in-flight request must count regardless of the TTL, got %d", got)
	}
	done()
}

// Once the request ends, the media must fall back to the TTL window (a player
// making many short range requests has gaps) and then expire.
func TestPlaybackActivity_EndFallsBackToTTLThenExpires(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
	a.now = func() time.Time { return now }

	a.Begin("mi-1")()

	now = now.Add(30 * time.Second)
	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("just after a request the TTL window must still cover it, got %d", got)
	}
	now = now.Add(time.Minute)
	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("past the TTL with no request in flight it must expire, got %d", got)
	}
}

// Two players on the same media is still ONE media being read; the counter must
// not double it, and must not drop to zero while the second is still streaming.
func TestPlaybackActivity_OverlappingRequestsOnOneMediaCountOnce(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
	a.now = func() time.Time { return now }

	first := a.Begin("mi-1")
	second := a.Begin("mi-1")
	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("two requests on one media count once, got %d", got)
	}

	first()
	now = now.Add(2 * time.Hour)
	if got := a.ActiveSessionCount(); got != 1 {
		t.Fatalf("the still-running request must keep the media active, got %d", got)
	}

	second()
	now = now.Add(2 * time.Hour)
	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("once every request ended and the TTL lapsed it must expire, got %d", got)
	}
}

// An in-flight entry must never be reclaimed by the expiry sweep - that would
// lose the release's bookkeeping and let the counter go negative or stick.
func TestPlaybackActivity_InFlightEntrySurvivesSweep(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
	a.now = func() time.Time { return now }

	done := a.Begin("mi-1")
	now = now.Add(time.Hour)
	a.ActiveSessionCount() // runs the sweep

	done()
	now = now.Add(time.Hour)
	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("release after a sweep must still settle to 0, got %d", got)
	}
	if got := a.size(); got != 0 {
		t.Fatalf("the entry must be reclaimed once released and expired, %d held", got)
	}
}

// Releasing twice (a defer plus a manual call, or a retry) must not corrupt the
// count into a stuck-active or negative state.
func TestPlaybackActivity_DoubleReleaseIsSafe(t *testing.T) {
	now := time.Unix(1000, 0)
	a := NewPlaybackActivity(time.Minute)
	a.now = func() time.Time { return now }

	done := a.Begin("mi-1")
	done()
	done()

	now = now.Add(2 * time.Minute)
	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("a double release must settle to 0, got %d", got)
	}
}

func TestPlaybackActivity_BeginOnNilIsSafe(t *testing.T) {
	var a *PlaybackActivity
	done := a.Begin("mi-1")
	done()
	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("nil tracker must report 0, got %d", got)
	}
}

// A nil tracker is the "feature not wired" case (degenerate test setups, and
// any future caller that has no on-demand path); it must read as zero, never
// panic.
func TestPlaybackActivity_NilIsZero(t *testing.T) {
	var a *PlaybackActivity
	a.Touch("mi-1")
	if got := a.ActiveSessionCount(); got != 0 {
		t.Fatalf("nil tracker must report 0, got %d", got)
	}
}

// Forget drops settled media at once, and keeps a media whose request is still
// in flight: someone is reading it right now.
func TestPlaybackActivity_ForgetDropsSettledMediaOnly(t *testing.T) {
	a := NewPlaybackActivity(time.Minute)
	a.Touch("ondemand")
	release := a.Begin("direct")
	defer release()
	a.Touch("other")
	if n := a.ActiveSessionCount(); n != 3 {
		t.Fatalf("active = %d, want 3", n)
	}
	a.Forget("ondemand", "direct", "never-seen")
	if n := a.ActiveSessionCount(); n != 2 {
		t.Fatalf("active after Forget = %d, want 2 (the in-flight read and the untouched media stay)", n)
	}
	var nilA *PlaybackActivity
	nilA.Forget("x")
}
