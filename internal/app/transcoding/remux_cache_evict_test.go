package transcoding

import (
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
