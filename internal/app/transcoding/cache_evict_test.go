package transcoding_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/app/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSubdir creates root/<name>/blob with size bytes and forces both file
// and directory modtimes to mt so the LRU ordering is deterministic.
func writeSubdir(t *testing.T, root, name string, size int, mt time.Time) string {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	blob := filepath.Join(dir, "blob")
	require.NoError(t, os.WriteFile(blob, make([]byte, size), 0o644))
	require.NoError(t, os.Chtimes(blob, mt, mt))
	require.NoError(t, os.Chtimes(dir, mt, mt))
	return dir
}

// dirExists is a thin assertion helper for clarity at call sites.
func dirExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// gbFromBytes converts byte counts to the float64 GB CacheEvictor expects.
func gbFromBytes(b int64) float64 { return float64(b) / (1024 * 1024 * 1024) }

func TestCacheEvictor_TotalUnderCap_NoEviction(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	now := time.Now()

	for i := range 5 {
		writeSubdir(t, root, "s"+string(rune('0'+i)), 1024, now.Add(time.Duration(-i)*time.Hour))
	}

	// 1 MB cap vs ~5 KB total — no eviction.
	e := transcoding.NewCacheEvictor(root, gbFromBytes(1024*1024), time.Minute)
	freed, err := e.EvictOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), freed)

	for i := range 5 {
		assert.True(t, dirExists(filepath.Join(root, "s"+string(rune('0'+i)))), "subdir s%d should still exist", i)
	}
}

func TestCacheEvictor_OverHighWatermark_EvictsLRUUntilLowMark(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	now := time.Now()

	// 5 dirs of 100 bytes each = 500 bytes total. Cap = 500 → high mark
	// at 475 (95%), low mark at 400 (80%). We must evict at least 100 bytes
	// to fall under 400, which means at least 1 LRU dir gets removed.
	mtimes := []time.Time{
		now.Add(-5 * time.Hour), // s0 oldest -> evicted first
		now.Add(-4 * time.Hour),
		now.Add(-3 * time.Hour),
		now.Add(-2 * time.Hour),
		now.Add(-1 * time.Hour),
	}
	for i, mt := range mtimes {
		writeSubdir(t, root, "s"+string(rune('0'+i)), 100, mt)
	}

	e := transcoding.NewCacheEvictor(root, gbFromBytes(500), time.Minute)
	freed, err := e.EvictOnce(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, freed, int64(100), "must free at least one LRU subdir")

	// Oldest must be gone.
	assert.False(t, dirExists(filepath.Join(root, "s0")), "oldest LRU subdir must be evicted")
	// Newest must remain.
	assert.True(t, dirExists(filepath.Join(root, "s4")), "newest subdir must survive")
}

func TestCacheEvictor_DisabledWhenMaxGBZero(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSubdir(t, root, "s0", 1024, time.Now())

	e := transcoding.NewCacheEvictor(root, 0, time.Minute)
	freed, err := e.EvictOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), freed)
	assert.True(t, dirExists(filepath.Join(root, "s0")), "disabled evictor must not touch the cache")
}

func TestCacheEvictor_RunStopsOnContextCancel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := transcoding.NewCacheEvictor(root, 1, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(done)
	}()

	// Give the loop one or two ticks then cancel.
	time.Sleep(120 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestCacheEvictor_RunDisabledReturnsImmediately(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	e := transcoding.NewCacheEvictor(root, 0, time.Minute)

	done := make(chan struct{})
	go func() {
		e.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("disabled Run should return immediately")
	}
}

func TestCacheEvictor_LockedSubdir_LoggedAndOtherEvicted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses unix permissions; cannot simulate locked subdir")
	}
	t.Parallel()
	root := t.TempDir()
	now := time.Now()

	// "locked" is the LRU candidate but unreadable; "fresh" is large and
	// recent. The cap forces eviction; the locked dir must not crash the
	// pass — eviction should fall through to the next-oldest readable dir.
	locked := writeSubdir(t, root, "locked", 100, now.Add(-10*time.Hour))
	writeSubdir(t, root, "old", 400, now.Add(-5*time.Hour))
	writeSubdir(t, root, "fresh", 100, now.Add(-1*time.Hour))

	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	// Cap = 500 → high mark 475, low mark 400. Total readable size ~500 bytes
	// (locked is unreadable, so its size is unknown to the evictor; "old" +
	// "fresh" alone = 500 → over the high mark).
	e := transcoding.NewCacheEvictor(root, gbFromBytes(500), time.Minute)
	freed, err := e.EvictOnce(context.Background())
	require.NoError(t, err)
	assert.Greater(t, freed, int64(0), "evictor must keep working despite locked subdir")

	// "old" was the next-oldest readable subdir → it must be the one removed.
	assert.False(t, dirExists(filepath.Join(root, "old")), "next-oldest readable subdir must be evicted")
	// "fresh" must survive.
	assert.True(t, dirExists(filepath.Join(root, "fresh")), "newest subdir must survive")
}
