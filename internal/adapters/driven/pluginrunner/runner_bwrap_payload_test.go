package pluginrunner

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startBwrapSleeper starts a copy of sleep as a plugin inside the real
// bubblewrap sandbox, waits until it runs, and returns the runner with its
// tracked entry.
func startBwrapSleeper(t *testing.T, id string) (*Runner, *processEntry) {
	t.Helper()
	r, e := startBwrapSleeperNoWait(t, id)
	// bwrap reports the child right after the clone, before it execs the
	// plugin: wait for the plugin to be the one running under that PID.
	require.Eventually(t, func() bool {
		cmdline, err := os.ReadFile("/proc/" + strconv.Itoa(e.payloadPID) + "/cmdline")
		return err == nil && strings.HasSuffix(strings.Split(string(cmdline), "\x00")[0], "/sleep")
	}, 5*time.Second, 10*time.Millisecond, "bwrap's child never exec'd the plugin")
	return r, e
}

// startBwrapSleeperNoWait returns as soon as Start does: bwrap may still be
// setting the sandbox up.
func startBwrapSleeperNoWait(t *testing.T, id string) (*Runner, *processEntry) {
	t.Helper()
	if err := ProbeSandbox(sandboxModeBubblewrap); err != nil {
		t.Skipf("bubblewrap unusable here: %v", err)
	}
	// Not under /tmp: the sandbox mounts a fresh tmpfs there, which would hide
	// the install dir from the plugin (real plugins live under /data/plugins).
	wd, err := os.Getwd()
	require.NoError(t, err)
	pluginsDir, err := os.MkdirTemp(wd, "evey-bwrap-payload-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(pluginsDir) })
	require.NoError(t, os.Chmod(pluginsDir, 0o755)) //nolint:gosec // the plugin may run under evey-plugin and must traverse it
	installDir := filepath.Join(pluginsDir, id)
	require.NoError(t, os.MkdirAll(installDir, 0o755))
	// The copy keeps its name: a multi-call coreutils binary (Ubuntu's Rust
	// coreutils) dispatches on argv[0] and refuses any other name.
	copyFile(t, "/bin/sleep", filepath.Join(installDir, "sleep"))

	r := NewRunner(pluginsDir).WithAllowMissingSandbox(true).WithSandboxMode(sandboxModeBubblewrap)
	p := &plugin.Plugin{ID: id, Name: "sleeper", Entrypoint: "sleep 60", InstallDir: installDir}
	require.NoError(t, r.Start(context.Background(), p))
	t.Cleanup(func() { _ = r.Kill(id) })

	r.mu.RLock()
	e := r.processes[id]
	r.mu.RUnlock()
	require.NotNil(t, e)
	return r, e
}

// TestBwrapStart_PayloadIsTheSandboxedChild pins what the runner tracks under
// bubblewrap: the process running the plugin code (the PID the host socket
// authenticates and the resource monitor samples) is bwrap's direct child,
// with no reaper in between, still in bwrap's process group so Stop/Kill
// reach it, and without bwrap's --info-fd leaked into it.
func TestBwrapStart_PayloadIsTheSandboxedChild(t *testing.T) {
	_, e := startBwrapSleeper(t, "bwrap-payload")
	bwrapPID := e.cmd.Process.Pid

	require.NotEqual(t, bwrapPID, e.payloadPID, "under --unshare-pid the plugin is a child of bwrap, not bwrap")
	assert.Equal(t, e.payloadPID, e.pid(), "the resource monitor must sample the plugin, not bwrap")
	assert.Equal(t, bwrapPID, procStatusInt(t, e.payloadPID, "PPid:"), "--as-pid-1: no reaper between bwrap and the plugin")

	pgid, err := syscall.Getpgid(e.payloadPID)
	require.NoError(t, err)
	assert.Equal(t, bwrapPID, pgid, "the plugin must stay in the group Stop/Kill signal")

	_, err = os.Readlink("/proc/" + strconv.Itoa(e.payloadPID) + "/fd/3")
	assert.ErrorIs(t, err, os.ErrNotExist, "bwrap's --info-fd must not reach the plugin (fd 3: %v)", err)
}

func TestBwrapKill_ReachesThePayload(t *testing.T) {
	r, e := startBwrapSleeper(t, "bwrap-kill")
	require.NoError(t, r.Kill("bwrap-kill"))
	assertGone(t, e.payloadPID)
}

// TestBwrapStop_ReachesThePayload: sleep has no SIGTERM handler, and as PID 1
// of its namespace it ignores a SIGTERM from outside. Stop must still end it
// without waiting for the SIGKILL escalation: the SIGTERM kills bwrap, and
// --die-with-parent then SIGKILLs the plugin.
func TestBwrapStop_ReachesThePayload(t *testing.T) {
	r, e := startBwrapSleeper(t, "bwrap-stop")
	start := time.Now()
	require.NoError(t, r.Stop(context.Background(), "bwrap-stop"))
	assert.Less(t, time.Since(start), shutdownTimeout, "Stop must not need the SIGKILL escalation")
	assertGone(t, e.payloadPID)
}

// TestBwrapStop_DuringSetup: stopped right after Start, the child is still
// bwrap's own setup code, PID 1 of its namespace with no SIGTERM handler and
// no parent-death signal armed yet - it outlived bwrap until the 5 s SIGKILL
// escalation. The short bubblewrap grace ends it.
func TestBwrapStop_DuringSetup(t *testing.T) {
	r, e := startBwrapSleeperNoWait(t, "bwrap-stop-early")
	start := time.Now()
	require.NoError(t, r.Stop(context.Background(), "bwrap-stop-early"))
	assert.Less(t, time.Since(start), bwrapTermGrace+killTimeout, "Stop must not wait the full shutdownTimeout")
	assertGone(t, e.payloadPID)
}

// TestStart_NonBwrapPayloadIsTheProcess: every other mode execs the plugin in
// place (Landlock through the launcher's syscall.Exec), so the tracked PID
// stays cmd.Process.Pid.
func TestStart_NonBwrapPayloadIsTheProcess(t *testing.T) {
	bin := buildTestBinary(t)
	r := NewRunner(filepath.Dir(bin)).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{ID: "plg-payload-basic", Name: bin}
	require.NoError(t, r.Start(context.Background(), p))
	t.Cleanup(func() { _ = r.Kill(p.ID) })

	r.mu.RLock()
	e := r.processes[p.ID]
	r.mu.RUnlock()
	require.NotNil(t, e)
	assert.Equal(t, e.cmd.Process.Pid, e.payloadPID)
}

func assertGone(t *testing.T, pid int) {
	t.Helper()
	assert.Eventually(t, func() bool {
		data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
		if err != nil {
			return true
		}
		// A zombie waiting for its reaper is dead already.
		fields := strings.Fields(string(data[strings.LastIndexByte(string(data), ')')+1:]))
		return len(fields) > 0 && fields[0] == "Z"
	}, 5*time.Second, 20*time.Millisecond, "plugin process %d still running", pid)
}

func procStatusInt(t *testing.T, pid int, key string) int {
	t.Helper()
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	require.NoError(t, err)
	for line := range strings.SplitSeq(string(data), "\n") {
		if after, ok := strings.CutPrefix(line, key); ok {
			n, convErr := strconv.Atoi(strings.TrimSpace(after))
			require.NoError(t, convErr)
			return n
		}
	}
	t.Fatalf("%s not in /proc/%d/status", key, pid)
	return 0
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755) //nolint:gosec // test-owned path
	require.NoError(t, err)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, out.Close())
}

// The short grace is for bubblewrap alone: a plugin exec'd in place
// (Landlock, basic) gets the SIGTERM itself and keeps the full timeout to
// act on it.
func TestTermGrace_OnlyShortUnderBwrap(t *testing.T) {
	bwrapped := &processEntry{cmd: &exec.Cmd{Process: &os.Process{Pid: 100}}, payloadPID: 101}
	inPlace := &processEntry{cmd: &exec.Cmd{Process: &os.Process{Pid: 100}}, payloadPID: 100}
	untracked := &processEntry{cmd: &exec.Cmd{Process: &os.Process{Pid: 100}}}
	assert.Equal(t, bwrapTermGrace, bwrapped.termGrace())
	assert.Equal(t, shutdownTimeout, inPlace.termGrace())
	assert.Equal(t, shutdownTimeout, untracked.termGrace())
}
