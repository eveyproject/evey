//go:build linux

package pluginrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	// Start() returns at fork, BEFORE the child has exec'd: until it does,
	// /proc/<pid>/cmdline still holds the test binary's argv, so the reap's
	// cmdline match finds nothing and the test sees zero victims. Cheap under
	// no load, which is why this raced only under -race and on CI.
	waitCmdline(t, cmd.Process.Pid, script)
	return cmd
}

// waitCmdline blocks until pid's /proc cmdline advertises want, i.e. the exec
// has landed and the process is matchable by the reap.
func waitCmdline(t *testing.T, pid int, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
		if err == nil && strings.Contains(string(data), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d never exec'd with %q in its cmdline", pid, want)
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
// previous instance left running - matched by cmdline under the runner's
// plugin dir - before starting fresh ones.
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

	// Selection, not liveness, is what this test is about. The previous version
	// slept a fixed 300ms and then checked both processes were alive - but the
	// escalation it was meant to rule out fires after killTimeout (3s), so the
	// check landed long before anything could have happened and proved nothing
	// about it. Under load it still went red now and then, on timing rather
	// than on behaviour. A victim count of zero is the actual invariant: a
	// process that was never selected can never be signalled, at any delay.
	if n := r.ReapStaleProcesses(); n != 0 {
		t.Errorf("reap selected %d victim(s); neither the unrelated nor the owned process may be one", n)
	}

	// Kept as the second half of the same claim: nothing was signalled
	// synchronously either.
	if syscall.Kill(unrelated.Process.Pid, 0) != nil {
		t.Errorf("unrelated process %d was killed by the reap", unrelated.Process.Pid)
	}
	if syscall.Kill(owned.Process.Pid, 0) != nil {
		t.Errorf("owned (registered) plugin process %d was killed by the reap", owned.Process.Pid)
	}
}

// The delayed SIGKILL used to check only that SOMETHING held the pid. Across
// the grace period the victim normally dies and Linux may hand its pid to an
// unrelated process - which then got SIGKILLed. This is the guard: identity,
// not liveness. (It also made TestReapStaleProcesses_Spares... flaky, since
// tests spawn processes fast enough to recycle pids.)
func TestStillReapable_RejectsARecycledPid(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(dir)

	// A live process that is NOT a plugin under our dir: exactly what a pid
	// recycled to a stranger looks like.
	stranger := spawnFakeStalePlugin(t, t.TempDir())
	if r.stillReapable(stranger.Process.Pid, syscall.Getuid()) {
		t.Error("a live pid that is not one of our plugins must never be escalated to SIGKILL")
	}

	// A real one still under our dir stays reapable.
	ours := spawnFakeStalePlugin(t, dir)
	if !r.stillReapable(ours.Process.Pid, syscall.Getuid()) {
		t.Error("a plugin still running under our dir must remain reapable")
	}

	// A pid nobody holds is not reapable either.
	if r.stillReapable(1<<30, syscall.Getuid()) {
		t.Error("a dead/absent pid must not be reapable")
	}
}
