package sysinfo

import (
	"testing"
	"time"
)

// Moving from sync.Once to a locked cache (2026-08-01) fixed a real stall: one
// hanging probe blocked every caller for the life of the process. But
// "remember successes only" swung too far the other way - on a host with no
// ffmpeg, every call re-forked a process while holding ffmpegMu for up to 3s,
// so a polling admin dashboard paid 3s per refresh, serialised.
//
// Contract: a failure is remembered for a few seconds, then retried. Both
// halves matter - without the memory we re-fork in a loop, without the retry a
// repaired host stays marked broken until restart.

func resetFFmpegProbeState(t *testing.T) {
	t.Helper()
	ffmpegMu.Lock()
	ffmpegCached, ffmpegProbed, ffmpegProbes = "", false, 0
	ffmpegRetryAt = time.Time{}
	ffmpegMu.Unlock()
	t.Cleanup(func() {
		ffmpegMu.Lock()
		ffmpegCached, ffmpegProbed, ffmpegProbes = "", false, 0
		ffmpegRetryAt = time.Time{}
		ffmpegNow = time.Now
		ffmpegMu.Unlock()
	})
}

func probeCount() int {
	ffmpegMu.Lock()
	defer ffmpegMu.Unlock()
	return ffmpegProbes
}

func TestFFmpegVersion_FailedProbeIsNotRefiredOnEveryCall(t *testing.T) {
	resetFFmpegProbeState(t)

	// A path that cannot exist: the probe fails for sure.
	const missing = "/nonexistent/evey-test-ffmpeg"
	for range 5 {
		if v := FFmpegVersion(missing); v != "" {
			t.Fatalf("a failed probe must return an empty version, got %q", v)
		}
	}

	if n := probeCount(); n != 1 {
		t.Fatalf("5 calls triggered %d ffmpeg executions instead of one: every call "+
			"forks a process while holding the global lock, so a refreshing admin "+
			"panel serialises", n)
	}
}

func TestFFmpegVersion_RetriesOnceTheWindowHasPassed(t *testing.T) {
	resetFFmpegProbeState(t)

	base := time.Now()
	ffmpegMu.Lock()
	ffmpegNow = func() time.Time { return base }
	ffmpegMu.Unlock()

	const missing = "/nonexistent/evey-test-ffmpeg"
	FFmpegVersion(missing)
	FFmpegVersion(missing)
	if n := probeCount(); n != 1 {
		t.Fatalf("inside the window we expect 1 probe, got %d", n)
	}

	// The host is "repaired" later: the window must reopen, otherwise a server
	// up for weeks would stay marked as having no ffmpeg.
	ffmpegMu.Lock()
	ffmpegNow = func() time.Time { return base.Add(ffmpegFailureRetryAfter + time.Second) }
	ffmpegMu.Unlock()

	FFmpegVersion(missing)
	if n := probeCount(); n != 2 {
		t.Fatalf("past the window the probe must be retried (want 2, got %d): otherwise "+
			"a transient failure condemns the answer until restart", n)
	}
}
