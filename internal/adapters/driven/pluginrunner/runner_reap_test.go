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

// spawnFakeStalePluginNoFork is spawnFakeStalePlugin for the processes a test
// needs to SURVIVE the reap.
//
// `sh <script>` forks to run sleep, and a forked-but-not-yet-exec'd child
// briefly carries its parent's cmdline - so the reap can match that transient
// twin, signal it, and the shell then exits because its child died. The
// registered "owned" process therefore died as a side effect, roughly one run
// in eight under load. That is an artifact of the fake, not a defect in the
// reap (which runs once at boot, before any plugin is started).
//
// `exec tail -f "$0"` REPLACES the shell instead: one process, no fork, no
// twin. The path stays a separate argv element because the reap matches a
// prefix on one element - buried inside the -c string it would not match.
func spawnFakeStalePluginNoFork(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	script := filepath.Join(dir, "plg-stale-test", "main.js")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatalf("mkdir fake plugin dir: %v", err)
	}
	if err := os.WriteFile(script, []byte("sleep 300\n"), 0o755); err != nil {
		t.Fatalf("write fake plugin script: %v", err)
	}
	cmd := exec.Command("/bin/sh", "-c", `exec tail -f "$0"`, script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn fake plugin: %v", err)
	}
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})
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

	// Both must SURVIVE, so both use the non-forking spawn - see its doc.
	// Unrelated process: cmdline points into a DIFFERENT directory.
	unrelated := spawnFakeStalePluginNoFork(t, t.TempDir())

	// Owned process: registered in r.processes (a plugin this instance runs).
	owned := spawnFakeStalePluginNoFork(t, dir)

	r := NewRunner(dir)
	r.mu.Lock()
	r.processes["plg-owned"] = &processEntry{cmd: owned, pluginID: "plg-owned"}
	r.mu.Unlock()

	r.ReapStaleProcesses()

	// Wait PAST the grace period before judging. The previous version slept a
	// fixed 300ms, which is shorter than killTimeout (3s), so it could never
	// observe the delayed SIGKILL it existed to rule out - it only measured a
	// clock, and drifted red under load.
	//
	// Asserting "the reap selected zero victims" instead looks tempting and is
	// also wrong: `sh <script>` FORKS to run sleep, and a forked-but-not-yet-
	// exec'd child briefly carries its parent's cmdline, so the reap can
	// legitimately match that transient twin. That is a test artifact, not a
	// product defect. What actually matters is narrower and checkable: THESE
	// TWO processes must still be alive once every escalation has had its
	// chance to fire.
	time.Sleep(killTimeout + 500*time.Millisecond)

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
