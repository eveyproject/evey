package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

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
	require.NoError(t, err)
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
	require.Error(t, err)
	_, err = procThreads(deadPID)
	require.Error(t, err)
	_, err = procCPUTicks(deadPID)
	assert.Error(t, err)
}

// TestApplyNiceDisabled is a no-op when cpuNice <= 0 and must not touch the
// process or panic.
func TestApplyNiceDisabled(t *testing.T) {
	r := &Runner{cpuNice: 0}
	assert.NotPanics(t, func() { r.applyNice(os.Getpid()) })
}

// spawnSleep starts a real `sleep` child wired into a minimal processEntry the
// resource monitor can act on (Stop needs cancel + done + a throwaway sockPath).
// A goroutine reaps it and closes done; cleanup kills any survivor.
func spawnSleep(t *testing.T, r *Runner, id string) (*processEntry, int) {
	t.Helper()
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	cmd := exec.Command("sleep", "120")
	require.NoError(t, cmd.Start())

	_, cancel := context.WithCancel(context.Background())
	e := &processEntry{
		cmd:      cmd,
		cancel:   cancel,
		sockPath: filepath.Join(t.TempDir(), "p.sock"),
		pluginID: id,
		done:     make(chan struct{}),
	}
	go func() { _, _ = cmd.Process.Wait(); close(e.done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	r.mu.Lock()
	r.processes[id] = e
	r.mu.Unlock()
	return e, cmd.Process.Pid
}

func processAlive(pid int) bool {
	// Signal 0 probes existence without delivering anything.
	return syscall.Kill(pid, 0) == nil
}

// TestCheckResources_KillsMemoryOverLimit is the core proof of the userspace
// approach: with a memory cap set BELOW a live process's real RSS (read from
// /proc), one monitor pass stops that process. Deterministic — the cap is
// derived from the measured RSS, so the breach cannot be flaky.
func TestCheckResources_KillsMemoryOverLimit(t *testing.T) {
	r := NewRunner()
	e, pid := spawnSleep(t, r, "memhog")

	// A freshly Start()ed process has no RSS accounted in /proc yet; under load
	// the gap is wide enough that an immediate read returns 0 (flaky). Wait for
	// the first positive reading — the cap is still derived from the real RSS.
	var rss int64
	require.Eventually(t, func() bool {
		var err error
		rss, err = procRSSBytes(pid)
		return err == nil && rss > 0
	}, 5*time.Second, 20*time.Millisecond, "process RSS must appear in /proc")
	r.resLimits = resourceLimits{memBytes: rss / 2} // cap below actual RSS → must trip

	r.checkResources(e, pid, map[int]*cpuSample{}, false)

	assert.Eventually(t, func() bool { return !processAlive(pid) }, 5*time.Second, 50*time.Millisecond,
		"monitor must stop a process exceeding its memory cap")
	r.mu.RLock()
	_, stillTracked := r.processes["memhog"]
	r.mu.RUnlock()
	assert.False(t, stillTracked, "stopped plugin must be removed from the runner")
}

// TestCheckResources_LeavesUnderLimit is the negative control: a process well
// under its cap is never touched.
func TestCheckResources_LeavesUnderLimit(t *testing.T) {
	r := NewRunner()
	_, pid := spawnSleep(t, r, "calm")
	r.resLimits = resourceLimits{memBytes: 4 << 30} // 4 GiB — never exceeded by sleep

	r.checkResources(r.processes["calm"], pid, map[int]*cpuSample{}, false)

	assert.True(t, processAlive(pid), "a process under its cap must keep running")
	r.mu.RLock()
	_, stillTracked := r.processes["calm"]
	r.mu.RUnlock()
	assert.True(t, stillTracked, "an under-cap plugin must stay tracked")
}

// TestResourceMonitorLoop_StopsHog proves the real background sweeper (the
// goroutine, not a direct call) detects and stops an over-cap process end to end.
func TestResourceMonitorLoop_StopsHog(t *testing.T) {
	r := NewRunner()
	e, pid := spawnSleep(t, r, "loophog")
	rss, err := procRSSBytes(pid)
	require.NoError(t, err)
	r.resLimits = resourceLimits{memBytes: rss / 2}

	r.startResourceMonitor() // launches the real ticker loop (first sweep at resMonInterval)

	select {
	case <-e.done:
		// process exited — the sweeper stopped it
	case <-time.After(3*resMonInterval + 15*time.Second):
		// Generous deadline ON PURPOSE: under a full `make test` (testcontainers
		// + every suite in parallel) the monitor goroutine is CPU-starved and
		// the kill can slip past one sweep interval — measured flaking ~1/9
		// full-suite runs at interval+4s. The happy path returns on e.done
		// immediately, so the slack only stretches the FAILURE case.
		t.Fatal("resource monitor loop did not stop the over-cap process in time")
	}
	assert.False(t, processAlive(pid))
}
