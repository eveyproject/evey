//go:build linux

package pluginrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// spawnFakeStalePlugin starts a long-lived process whose cmdline points into
// dir, the way a real plugin process does (argv = interpreter + <dir>/<id>/
// main.js). The script is a real file exec'd via /bin/sh: rewriting argv[0]
// of /bin/sleep instead breaks on busybox userlands (the test image), where
// argv[0] selects the applet and the process exits instantly. The process is
// NOT registered in any runner, mimicking an orphan left behind by a dead
// server instance.
func spawnFakeStalePlugin(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	script := filepath.Join(dir, "plg-stale-test", "main.js")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatalf("mkdir fake plugin dir: %v", err)
	}
	if err := os.WriteFile(script, []byte("sleep 300\n"), 0o755); err != nil {
		t.Fatalf("write fake plugin script: %v", err)
	}
	cmd := exec.Command("/bin/sh", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn fake plugin: %v", err)
	}
	// Collect the child as soon as it dies: without a waiter it lingers as a
	// zombie and kill(pid, 0) keeps succeeding, masking a successful reap.
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})
	return cmd
}

func waitDead(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// TestReapStaleProcesses_KillsOrphansFromPreviousInstance locks the boot-time
// defense against plugin process leaks: pdeathsig delivery is not reliable on
// every kernel (and is cleared outright by the prod launcher's file caps on
// unfixed builds), so a restarting server must terminate plugin processes a
// previous instance left running — matched by cmdline under the runner's
// plugin dir — before starting fresh ones.
func TestReapStaleProcesses_KillsOrphansFromPreviousInstance(t *testing.T) {
	dir := t.TempDir()
	stale := spawnFakeStalePlugin(t, dir)

	r := NewRunner(dir)
	n := r.ReapStaleProcesses()

	if n < 1 {
		t.Errorf("expected at least 1 reaped process, got %d", n)
	}
	if !waitDead(stale.Process.Pid, 5*time.Second) {
		t.Errorf("stale plugin process %d still alive after reap", stale.Process.Pid)
	}
}

func TestReapStaleProcesses_SparesUnrelatedAndOwnedProcesses(t *testing.T) {
	dir := t.TempDir()

	// Unrelated process: cmdline points into a DIFFERENT directory.
	unrelated := spawnFakeStalePlugin(t, t.TempDir())

	// Owned process: registered in r.processes (a plugin this instance runs).
	owned := spawnFakeStalePlugin(t, dir)

	r := NewRunner(dir)
	r.mu.Lock()
	r.processes["plg-owned"] = &processEntry{cmd: owned, pluginID: "plg-owned"}
	r.mu.Unlock()

	r.ReapStaleProcesses()
	time.Sleep(300 * time.Millisecond)

	if syscall.Kill(unrelated.Process.Pid, 0) != nil {
		t.Errorf("unrelated process %d was killed by the reap", unrelated.Process.Pid)
	}
	if syscall.Kill(owned.Process.Pid, 0) != nil {
		t.Errorf("owned (registered) plugin process %d was killed by the reap", owned.Process.Pid)
	}
}
