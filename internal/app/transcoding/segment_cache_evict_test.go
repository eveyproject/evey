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
	p := EvictionPolicy{MaxBytes: 100, MarginBytes: 0, SweepInterval: 0, ProtectRecent: 0}.withDefaults()
	assert.EqualValues(t, 100, p.MaxBytes)
	assert.EqualValues(t, 10, p.MarginBytes)
	assert.Equal(t, 30*time.Second, p.SweepInterval)
	assert.Equal(t, 4, p.ProtectRecent)
}

func TestSweepOnce_BelowMaxBytes_NoEviction(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegments(t, c, "a", 3, 50)
	writeSegments(t, c, "b", 3, 50)

	c.sweepOnce(EvictionPolicy{MaxBytes: 10_000, MarginBytes: 1_000, ProtectRecent: 0}, quietLogger())

	assert.DirExists(t, filepath.Join(c.baseDir, "a"))
	assert.DirExists(t, filepath.Join(c.baseDir, "b"))
}

func TestSweepOnce_AboveMax_EvictsOldest(t *testing.T) {
	// Two hashes; oldest by mtime gets evicted when over the cap.
	c, _ := newTestCache(t)
	writeSegments(t, c, "old", 4, 100)
	backdateHash(t, c, "old", -1*time.Hour)
	writeSegments(t, c, "new", 4, 100)

	c.sweepOnce(EvictionPolicy{MaxBytes: 500, MarginBytes: 50, ProtectRecent: 0}, quietLogger())

	assert.NoDirExists(t, filepath.Join(c.baseDir, "old"))
	assert.DirExists(t, filepath.Join(c.baseDir, "new"))
}

func TestSweepOnce_ProtectRecent(t *testing.T) {
	// Two hashes both over the cap. With ProtectRecent=1 the newest must
	// survive even if its eviction would have brought us under MaxBytes.
	c, _ := newTestCache(t)
	writeSegments(t, c, "old", 5, 100)
	backdateHash(t, c, "old", -1*time.Hour)
	writeSegments(t, c, "newer", 5, 100)

	c.sweepOnce(EvictionPolicy{MaxBytes: 200, MarginBytes: 0, ProtectRecent: 1}, quietLogger())

	assert.NoDirExists(t, filepath.Join(c.baseDir, "old"))
	assert.DirExists(t, filepath.Join(c.baseDir, "newer"), "newest hash must be protected")
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

	c.sweepOnce(EvictionPolicy{MaxBytes: 200, MarginBytes: 0, ProtectRecent: 0}, quietLogger())

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

	c.sweepOnce(EvictionPolicy{MaxBytes: 700, MarginBytes: 50, ProtectRecent: 0}, quietLogger())

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

func TestStartEviction_NoopWhenDisabled(t *testing.T) {
	c, _ := newTestCache(t)
	// MaxBytes 0 ⇒ disabled, returns a no-op stop. Most importantly, it
	// must NOT spawn a goroutine that survives across tests.
	stop := c.StartEviction(EvictionPolicy{}, quietLogger())
	stop()
}

func TestStartEviction_RunsAndStops(t *testing.T) {
	c, _ := newTestCache(t)
	writeSegments(t, c, "old", 5, 100)
	backdateHash(t, c, "old", -1*time.Hour)
	writeSegments(t, c, "new", 5, 100)

	stop := c.StartEviction(EvictionPolicy{
		MaxBytes:      300,
		MarginBytes:   50,
		SweepInterval: 30 * time.Millisecond,
		ProtectRecent: 1,
	}, quietLogger())
	defer stop()

	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(c.baseDir, "old"))
		return os.IsNotExist(err)
	}, 2*time.Second, 30*time.Millisecond)

	// new must survive
	assert.DirExists(t, filepath.Join(c.baseDir, "new"))
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
	backdateHash(t, c, "fresh", -10*time.Minute)

	c.Touch("watched") // a player just pulled a segment from this hash

	c.sweepOnce(EvictionPolicy{MaxBytes: 400, MarginBytes: 0, ProtectRecent: 0}, quietLogger())

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
	base := time.Unix(1000, 0)
	playing := func(n int) func() int { return func() int { return n } }

	cases := []struct {
		name      string
		active    func() int
		lastSweep time.Time
		now       time.Time
		want      bool
	}{
		{"idle keeps the tight cadence", playing(0), base, base.Add(30 * time.Second), true},
		{"unwired gate keeps the old behaviour", nil, base, base.Add(30 * time.Second), true},
		{"playing, inside the backoff window", playing(1), base, base.Add(30 * time.Second), false},
		{"playing, past the backoff window", playing(1), base, base.Add(6 * time.Minute), true},
		{"playing, first ever tick still sweeps", playing(1), time.Time{}, base, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := EvictionPolicy{
				MaxBytes:       1,
				ActivePlayback: tc.active,
			}.withDefaults()
			if got := p.shouldSweep(tc.now, tc.lastSweep); got != tc.want {
				t.Fatalf("shouldSweep = %v, want %v", got, tc.want)
			}
		})
	}
}

// The backoff is a playback concession, not a general slowdown of the disk-cap
// safety valve: a boot already over cap must be corrected even mid-viewing.
func TestEvictionPolicy_PlaybackBackoffDefaultsToFiveMinutes(t *testing.T) {
	p := EvictionPolicy{MaxBytes: 1}.withDefaults()
	if p.PlaybackSweepInterval != 5*time.Minute {
		t.Fatalf("default playback sweep interval = %v, want 5m", p.PlaybackSweepInterval)
	}
	if p.SweepInterval != 30*time.Second {
		t.Fatalf("idle cadence must stay 30s, got %v", p.SweepInterval)
	}
}
