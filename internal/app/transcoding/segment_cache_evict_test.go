package transcoding

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvictionPolicy_withDefaults(t *testing.T) {
	p := EvictionPolicy{MaxBytes: 100}.withDefaults()
	assert.EqualValues(t, 100, p.MaxBytes)
	assert.EqualValues(t, 10, p.MarginBytes)
	assert.Equal(t, 30*time.Second, p.SweepInterval)
	assert.EqualValues(t, DefaultMinFreeBytes, p.MinFreeBytes)
	assert.Equal(t, DefaultDownloadTTL, p.DownloadTTL)
	assert.Equal(t, DefaultProtectWindow, p.ProtectWindow)

	// A negative value disables a pass, zero takes the default.
	off := EvictionPolicy{MinFreeBytes: -1, DownloadTTL: -1}.withDefaults()
	assert.EqualValues(t, 0, off.MinFreeBytes)
	assert.EqualValues(t, 0, off.DownloadTTL)
}

// capOnly is a policy with the free-space and age passes off, so a test of
// the cap sees the cap alone.
func capOnly(maxBytes, margin int64) EvictionPolicy {
	return EvictionPolicy{MaxBytes: maxBytes, MarginBytes: margin, MinFreeBytes: -1, DownloadTTL: -1, ProtectWindow: time.Nanosecond}
}

func sweepNow(c *SegmentCache, p EvictionPolicy) {
	NewCacheBudget(c).sweepOnce(p.withDefaults(), quietLogger(), time.Now())
}

func TestSweepOnce_BelowMaxBytes_NoEviction(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegments(t, c, "a", 3, 50)
	writeSegments(t, c, "b", 3, 50)

	sweepNow(c, capOnly(10_000, 1_000))

	assert.DirExists(t, filepath.Join(c.baseDir, "a"))
	assert.DirExists(t, filepath.Join(c.baseDir, "b"))
}

func TestSweepOnce_AboveMax_EvictsOldest(t *testing.T) {
	// Two hashes; oldest by mtime gets evicted when over the cap.
	c, _ := newTestCache(t)
	writeSegments(t, c, "old", 4, 100)
	backdateHash(t, c, "old", -1*time.Hour)
	writeSegments(t, c, "new", 4, 100)
	backdateHash(t, c, "new", -20*time.Minute)

	sweepNow(c, capOnly(500, 50))

	assert.NoDirExists(t, filepath.Join(c.baseDir, "old"))
	assert.DirExists(t, filepath.Join(c.baseDir, "new"))
}

// A hash accessed within the protect window is a viewer's: it is spared by the
// cap pass whatever its rank. A hash idle for longer is not - the old "four
// most recent hashes" rule kept a finished film above the cap until four newer
// ones came along.
func TestSweepOnce_ProtectWindowSparesJustAccessedHashesOnly(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegments(t, c, "old", 5, 100)
	backdateHash(t, c, "old", -1*time.Hour)
	writeSegments(t, c, "newer", 5, 100) // written just now: inside the window

	p := capOnly(200, 0)
	p.ProtectWindow = 10 * time.Minute
	sweepNow(c, p)
	assert.NoDirExists(t, filepath.Join(c.baseDir, "old"))
	assert.DirExists(t, filepath.Join(c.baseDir, "newer"), "a just-accessed hash must be protected")

	// The same hash an hour later, nobody watching: it is the victim.
	backdateHash(t, c, "newer", -1*time.Hour)
	sweepNow(c, p)
	assert.NoDirExists(t, filepath.Join(c.baseDir, "newer"), "the window closes; recency alone protects nothing")
}

func TestSweepOnce_SkipsInFlightHash(t *testing.T) {
	// Two old hashes, both over the cap. The one with a live session
	// (pendingRefs > 0) must NOT be evicted even though its atime makes it the
	// oldest candidate - evicting it would stall the player. The idle one goes.
	c, _ := newTestCache(t)
	writeSegments(t, c, "inflight", 5, 100)
	backdateHash(t, c, "inflight", -2*time.Hour)
	writeSegments(t, c, "idle", 5, 100)
	backdateHash(t, c, "idle", -1*time.Hour)

	release := c.RegisterSession("inflight")
	defer release()

	sweepNow(c, capOnly(200, 0))

	assert.DirExists(t, filepath.Join(c.baseDir, "inflight"), "an in-flight hash must not be evicted")
	assert.NoDirExists(t, filepath.Join(c.baseDir, "idle"), "the idle hash must be evicted to reclaim space")
}

func TestSweepOnce_StopsAtTarget(t *testing.T) {
	// Three hashes; cap leaves room for ~2. The oldest gets evicted, sweep
	// stops because we're back under MaxBytes - MarginBytes.
	c, _ := newTestCache(t)
	writeSegments(t, c, "h1", 3, 100)
	backdateHash(t, c, "h1", -2*time.Hour)
	writeSegments(t, c, "h2", 3, 100)
	backdateHash(t, c, "h2", -1*time.Hour)
	writeSegments(t, c, "h3", 3, 100)
	backdateHash(t, c, "h3", -30*time.Minute)

	sweepNow(c, capOnly(700, 50))

	assert.NoDirExists(t, filepath.Join(c.baseDir, "h1"), "oldest must go")
	assert.DirExists(t, filepath.Join(c.baseDir, "h2"))
	assert.DirExists(t, filepath.Join(c.baseDir, "h3"))
}

func TestTotalBytes_Empty(t *testing.T) {
	c, _ := newTestCache(t)
	n, err := c.TotalBytes()
	require.NoError(t, err)
	assert.EqualValues(t, 0, n)
}

func TestTotalBytes_Sum(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegments(t, c, "a", 3, 100) // 300
	writeSegments(t, c, "b", 5, 200) // 1000
	n, err := c.TotalBytes()
	require.NoError(t, err)
	assert.EqualValues(t, 1300, n)
}

func TestStart_RunsAndStops(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegments(t, c, "old", 5, 100)
	backdateHash(t, c, "old", -1*time.Hour)
	writeSegments(t, c, "new", 5, 100)

	p := capOnly(300, 50)
	p.SweepInterval = 30 * time.Millisecond
	p.ProtectWindow = 10 * time.Minute
	stop := NewCacheBudget(c).Start(p, quietLogger())
	defer stop()

	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(c.baseDir, "old"))
		return os.IsNotExist(err)
	}, eventuallyWait, 30*time.Millisecond)

	// new must survive: written just now, inside the protect window
	assert.DirExists(t, filepath.Join(c.baseDir, "new"))
}

// A zero cap disables the cap pass - it never means "a budget of zero". Read
// as a budget, the first tick would empty the cache.
func TestStart_ZeroCapDisablesTheCapPass(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegments(t, c, "a", 5, 100)
	backdateHash(t, c, "a", -1*time.Hour)
	stop := NewCacheBudget(c).Start(EvictionPolicy{MinFreeBytes: -1, DownloadTTL: -1, SweepInterval: 5 * time.Millisecond}, quietLogger())
	time.Sleep(60 * time.Millisecond)
	assert.DirExists(t, filepath.Join(c.baseDir, "a"))
	stop()
	require.NotPanics(t, stop, "stop must be safe to call again")
}

func TestSweepOnce_TouchKeepsActivelyReadHashWarm(t *testing.T) {
	// Two hashes over the cap. "watched" has the OLDER mtime - its ReadRate=0
	// copy mux finished an hour ago - but a player is still pulling segments, so
	// Touch records a fresh access. "fresh" was written more recently but nobody
	// is reading it. With mtime alone the sweeper would evict "watched" (older);
	// with the access-time merge it stays warm and "fresh" is the victim instead.
	// This is the copy-rung eviction-under-viewer fix.
	c, _ := newTestCache(t)
	writeSegments(t, c, "watched", 3, 100)
	backdateHash(t, c, "watched", -1*time.Hour)
	writeSegments(t, c, "fresh", 3, 100)
	backdateHash(t, c, "fresh", -20*time.Minute)

	c.Touch("watched") // a player just pulled a segment from this hash

	p := capOnly(400, 0)
	p.ProtectWindow = 10 * time.Minute
	sweepNow(c, p)

	assert.DirExists(t, filepath.Join(c.baseDir, "watched"), "actively-read hash must stay warm via Touch")
	assert.NoDirExists(t, filepath.Join(c.baseDir, "fresh"), "un-read hash is the LRU victim")
}

func TestTouch_ClearedOnCleanup(t *testing.T) {
	// The access-time registry must track the LIVE cache, not history - an entry
	// has to be dropped when its hash is cleaned up, or the map leaks one entry
	// per ever-served hash for the process lifetime.
	c, _ := newTestCache(t)
	writeSegments(t, c, "h", 2, 50)
	c.Touch("h")
	if _, ok := c.LastAccess("h"); !ok {
		t.Fatal("expected an access time right after Touch")
	}
	require.NoError(t, c.CleanupHash("h"))
	if _, ok := c.LastAccess("h"); ok {
		t.Error("access time must be cleared on CleanupHash (no map leak)")
	}
}

func TestTouch_EmptyHashNoop(t *testing.T) {
	c, _ := newTestCache(t)
	c.Touch("") // must not panic or record a phantom entry
	if _, ok := c.LastAccess(""); ok {
		t.Error("empty hash must not be recorded")
	}
}

// ============================================================================
// helpers
// ============================================================================

// writeSegments lays down `count` segments under hash with `size` bytes each.
// Returns the directory it wrote into.
func writeSegments(t *testing.T, c *SegmentCache, hash string, count, size int) string {
	t.Helper()
	dir, err := c.DirFor(hash)
	require.NoError(t, err)
	payload := make([]byte, size)
	for i := range count {
		name := SegmentKey{DecisionHash: hash, VariantIdx: 0, SegmentIdx: i}.fileName(false)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), payload, 0o644))
	}
	return dir
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// backdateHash backs the mtime of EVERY file under hash to (now+offset). Used
// to make a freshly-written hash look older than another for LRU-sort tests.
// Touching just the first file is not enough - dirSizeAndMtime picks the
// NEWEST mtime across all files in the dir.
func backdateHash(t *testing.T, c *SegmentCache, hash string, offset time.Duration) {
	t.Helper()
	dir := filepath.Join(c.baseDir, hash)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	target := time.Now().Add(offset)
	for _, e := range entries {
		require.NoError(t, os.Chtimes(filepath.Join(dir, e.Name()), target, target))
	}
}

// The sweep walks the ENTIRE cache root - one lstat per file - on every tick,
// whatever the fill level, because the total is only known once the walk is
// done. Doing that every 30 s while someone is watching puts a background
// worker in contention with the viewer for the same spindle, which is the
// starvation class this server keeps hitting on weak NAS hardware.
func TestEvictionPolicy_ShouldSweep(t *testing.T) {
	active := 0
	p := EvictionPolicy{ActivePlayback: func() int { return active }, PlaybackSweepInterval: 5 * time.Minute}.withDefaults()
	now := time.Now()
	assert.True(t, p.shouldSweep(now, time.Time{}), "the first tick always sweeps")
	assert.True(t, p.shouldSweep(now, now.Add(-30*time.Second)), "idle host: every tick")
	active = 1
	assert.False(t, p.shouldSweep(now, now.Add(-30*time.Second)), "playback: back off")
	assert.True(t, p.shouldSweep(now, now.Add(-6*time.Minute)), "playback: still sweeps at the slow cadence")
	assert.True(t, EvictionPolicy{}.withDefaults().shouldSweep(now, now.Add(-time.Second)), "no counter wired: fixed cadence")
}

func TestEvictionPolicy_PlaybackBackoffDefaultsToFiveMinutes(t *testing.T) {
	assert.Equal(t, 5*time.Minute, EvictionPolicy{}.withDefaults().PlaybackSweepInterval)
}
