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
	for i := 0; i < count; i++ {
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
// Touching just the first file is not enough — dirSizeAndMtime picks the
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
