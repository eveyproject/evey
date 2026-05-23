package pluginrunner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResourceLimitsDefaults verifies zero (unset) fields fall back to the
// package defaults while explicit values are preserved.
func TestResourceLimitsDefaults(t *testing.T) {
	got := resourceLimits{}.defaults()
	assert.Equal(t, defaultMemBytes, got.memBytes)
	assert.Equal(t, defaultPIDsMax, got.pidsMax)

	custom := resourceLimits{memBytes: 1 << 20, pidsMax: 7}.defaults()
	assert.Equal(t, int64(1<<20), custom.memBytes)
	assert.Equal(t, 7, custom.pidsMax)
}

// TestDirSize sums regular files recursively and ignores directories.
func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 100), 0o644))
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "b.bin"), make([]byte, 250), 0o644))

	total, err := dirSize(dir)
	require.NoError(t, err)
	assert.Equal(t, int64(350), total)
}

// TestDirSizeMissing is error-tolerant by design: a missing directory yields
// 0 bytes without aborting, so a transient FS error never spuriously trips the
// disk cap (the sweep just sees no growth that pass).
func TestDirSizeMissing(t *testing.T) {
	total, err := dirSize(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.NoError(t, err)
	assert.Zero(t, total)
}

// TestProcReadersSelf exercises the /proc samplers against the test process
// itself: they must return plausible, non-error values.
func TestProcReadersSelf(t *testing.T) {
	pid := os.Getpid()

	rss, err := procRSSBytes(pid)
	require.NoError(t, err)
	assert.Positive(t, rss, "RSS of the running test must be > 0")

	threads, err := procThreads(pid)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, threads, 1, "a running process has at least one thread")

	// CPU ticks are monotonic and never error for a live PID; the value itself
	// may be 0 on a freshly-started process.
	_, err = procCPUTicks(pid)
	require.NoError(t, err)
}

// TestProcReadersDeadPID returns an error (not a panic) for a PID that cannot
// exist, so the sweeper safely skips exited plugins.
func TestProcReadersDeadPID(t *testing.T) {
	const deadPID = 1 << 30 // far above any real PID
	_, err := procRSSBytes(deadPID)
	assert.Error(t, err)
	_, err = procThreads(deadPID)
	assert.Error(t, err)
	_, err = procCPUTicks(deadPID)
	assert.Error(t, err)
}

// TestApplyNiceDisabled is a no-op when cpuNice <= 0 and must not touch the
// process or panic.
func TestApplyNiceDisabled(t *testing.T) {
	r := &Runner{cpuNice: 0}
	assert.NotPanics(t, func() { r.applyNice(os.Getpid()) })
}
