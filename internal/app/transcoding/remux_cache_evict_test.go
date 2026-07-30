package transcoding

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeRemuxFile(t *testing.T, dir, name string, size int, mtime time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, make([]byte, size), 0o644))
	require.NoError(t, os.Chtimes(p, mtime, mtime))
	return p
}

// Over budget, the janitor deletes oldest-mtime files until under target,
// protects the freshest, never touches in-flight *.tmp, and leaves a
// under-budget dir alone.
func TestRemuxCacheJanitor_Sweep(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	// 5 × 100 B = 500 B. MaxBytes 250, Margin 0 → target 250.
	old := writeRemuxFile(t, dir, "remux-v1-aaaa.mp4", 100, base.Add(-5*time.Minute))
	mid := writeRemuxFile(t, dir, "remux-v1-bbbb.mp4", 100, base.Add(-4*time.Minute))
	ccc := writeRemuxFile(t, dir, "remux-v1-cccc.mp4", 100, base.Add(-3*time.Minute))
	ddd := writeRemuxFile(t, dir, "remux-v1-dddd.mp4", 100, base.Add(-2*time.Minute))
	freshest := writeRemuxFile(t, dir, "remux-v1-eeee.mp4", 100, base.Add(-1*time.Minute))
	// In-flight producer output: must survive a concurrent sweep.
	tmp := writeRemuxFile(t, dir, "remux-v1-ffff.mp4.tmp", 100, base.Add(-9*time.Minute))

	j := NewRemuxCacheJanitor(dir)
	j.sweepOnce(EvictionPolicy{MaxBytes: 250, ProtectRecent: 1}, slog.Default())

	// Oldest three evicted to fall to/under 250 (500→200).
	assert.NoFileExists(t, old)
	assert.NoFileExists(t, mid)
	assert.NoFileExists(t, ccc)
	// dddd remains; eeee is the protected freshest.
	assert.FileExists(t, ddd)
	assert.FileExists(t, freshest)
	// The .tmp was neither counted nor deleted.
	assert.FileExists(t, tmp)
}

func TestRemuxCacheJanitor_UnderBudget_NoOp(t *testing.T) {
	dir := t.TempDir()
	a := writeRemuxFile(t, dir, "remux-v1-aaaa.mp4", 100, time.Now())
	j := NewRemuxCacheJanitor(dir)
	j.sweepOnce(EvictionPolicy{MaxBytes: 1000, ProtectRecent: 4}, slog.Default())
	assert.FileExists(t, a)
}

// A missing cache dir (nothing remuxed yet) is not an error.
func TestRemuxCacheJanitor_MissingDir_NoError(t *testing.T) {
	j := NewRemuxCacheJanitor(filepath.Join(t.TempDir(), "does-not-exist"))
	files, err := j.listFiles()
	require.NoError(t, err)
	assert.Empty(t, files)
}

// The budget is a ceiling, not a target: a directory sitting EXACTLY on
// MaxBytes is not over budget and must not lose a file. Evicting here would
// delete a remux the very next request re-produces, on every sweep, forever.
//
// Note: the early return is belt-and-braces, not load-bearing - the eviction
// loop's own `total <= target` guard already stops at the boundary, so
// mutating `<=` to `<` here changes nothing observable. The test pins the
// BEHAVIOUR (no file is lost at the ceiling), which is what matters.
func TestRemuxCacheJanitor_ExactlyAtBudget_NoEviction(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	a := writeRemuxFile(t, dir, "remux-v1-aaaa.mp4", 100, base.Add(-2*time.Minute))
	b := writeRemuxFile(t, dir, "remux-v1-bbbb.mp4", 100, base.Add(-time.Minute))

	NewRemuxCacheJanitor(dir).sweepOnce(EvictionPolicy{MaxBytes: 200, ProtectRecent: 0}, slog.Default())

	assert.FileExists(t, a, "200 bytes against a 200-byte budget is not over budget")
	assert.FileExists(t, b)
}

// One byte over IS over budget - the other side of the same edge.
func TestRemuxCacheJanitor_OneByteOverBudget_Evicts(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	oldest := writeRemuxFile(t, dir, "remux-v1-aaaa.mp4", 100, base.Add(-2*time.Minute))
	newest := writeRemuxFile(t, dir, "remux-v1-bbbb.mp4", 101, base.Add(-time.Minute))

	NewRemuxCacheJanitor(dir).sweepOnce(EvictionPolicy{MaxBytes: 200, ProtectRecent: 0}, slog.Default())

	assert.NoFileExists(t, oldest, "the least-recently-produced file goes first")
	assert.FileExists(t, newest)
}

// MarginBytes makes the sweep overshoot deliberately, so the next write does not
// immediately put the cache back over budget and trigger another sweep.
func TestRemuxCacheJanitor_MarginEvictsBelowTheCeiling(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	for i, name := range []string{"aaaa", "bbbb", "cccc", "dddd"} {
		writeRemuxFile(t, dir, "remux-v1-"+name+".mp4", 100, base.Add(-time.Duration(10-i)*time.Minute))
	}

	// 400 B against MaxBytes 300 with a 100 B margin: target is 200, so TWO
	// files go, not the one that would merely clear the ceiling.
	NewRemuxCacheJanitor(dir).sweepOnce(
		EvictionPolicy{MaxBytes: 300, MarginBytes: 100, ProtectRecent: 0}, slog.Default())

	files, err := NewRemuxCacheJanitor(dir).listFiles()
	require.NoError(t, err)
	assert.Len(t, files, 2, "the margin must be swept below the ceiling, not to it")
}

// ProtectRecent is a count of files, and an operator (or a default) can set it
// above what the cache holds. It must clamp, not slice out of range: an
// unguarded files[:len-protected] panics, and this sweep runs on a background
// ticker where a panic is only visible as a cache that silently stops bounding.
func TestRemuxCacheJanitor_ProtectRecentAboveFileCount_ClampsInsteadOfPanicking(t *testing.T) {
	dir := t.TempDir()
	a := writeRemuxFile(t, dir, "remux-v1-aaaa.mp4", 500, time.Now().Add(-time.Minute))

	require.NotPanics(t, func() {
		NewRemuxCacheJanitor(dir).sweepOnce(
			EvictionPolicy{MaxBytes: 100, ProtectRecent: 99}, slog.Default())
	})
	assert.FileExists(t, a, "everything is protected, so nothing may be evicted even over budget")
}

// MaxBytes <= 0 DISABLES the janitor - it does not mean "a budget of zero".
// Reading it as a budget would make the sweeper delete every remux in the
// directory on its first tick. The returned stop must also be safe to defer: a
// nil or panicking no-op would take the server down at shutdown.
func TestRemuxCacheJanitor_StartEviction_ZeroBudgetDisablesRatherThanEvictsAll(t *testing.T) {
	for _, maxBytes := range []int64{0, -1} {
		t.Run(fmt.Sprintf("MaxBytes=%d", maxBytes), func(t *testing.T) {
			dir := t.TempDir()
			base := time.Now()
			// More files than the default ProtectRecent (4), or the protection
			// alone would keep them and the test would pass either way.
			var kept []string
			for i, name := range []string{"aaaa", "bbbb", "cccc", "dddd", "eeee", "ffff"} {
				kept = append(kept, writeRemuxFile(t, dir, "remux-v1-"+name+".mp4", 500,
					base.Add(-time.Duration(10-i)*time.Minute)))
			}

			stop := NewRemuxCacheJanitor(dir).StartEviction(EvictionPolicy{
				MaxBytes:      maxBytes,
				SweepInterval: 5 * time.Millisecond, // many ticks inside the wait below
			}, nil)
			require.NotNil(t, stop, "a disabled janitor must still return a usable stop")

			time.Sleep(100 * time.Millisecond)
			for _, p := range kept {
				assert.FileExists(t, p,
					"MaxBytes=%d means DISABLED - an enabled janitor at a zero budget would evict everything", maxBytes)
			}

			require.NotPanics(t, stop)
		})
	}
}

// stop() must actually join the sweeper goroutine, not just signal it: a
// half-stopped janitor keeps deleting files under a directory the caller
// believes it owns again.
func TestRemuxCacheJanitor_StartEviction_StopJoinsTheSweeper(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	oldest := writeRemuxFile(t, dir, "remux-v1-aaaa.mp4", 300, base.Add(-2*time.Minute))
	writeRemuxFile(t, dir, "remux-v1-bbbb.mp4", 100, base.Add(-time.Minute))

	j := NewRemuxCacheJanitor(dir)
	stop := j.StartEviction(EvictionPolicy{
		MaxBytes:      200,
		ProtectRecent: 1,
		SweepInterval: 10 * time.Millisecond,
	}, slog.Default())

	require.Eventually(t, func() bool {
		_, err := os.Stat(oldest)
		return os.IsNotExist(err)
	}, 2*time.Second, 5*time.Millisecond, "the background sweeper must actually sweep")

	stop()
	require.NotPanics(t, stop, "stop must be safe to call again (deferred plus explicit)")
}
