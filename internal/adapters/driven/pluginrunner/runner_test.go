package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTestBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	bin := filepath.Join(dir, "test-plugin")

	require.NoError(t, os.Chmod(dir, 0o755))
	require.NoError(t, os.Chmod(filepath.Dir(dir), 0o755))

	err := os.WriteFile(src, []byte(`package main
import (
	"os"
	"os/signal"
	"syscall"
)
func main() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	<-ch
}
`), 0o644)
	require.NoError(t, err)

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))
	return bin
}

func TestRunnerStartStop(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()

	r := NewRunner(dir).WithAllowMissingSandbox(true)

	p := &plugin.Plugin{
		ID:   "plg-test-1",
		Name: bin,
	}

	err := r.Start(ctx, p)
	require.NoError(t, err)

	healthy, err := r.Health(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, healthy)

	err = r.Stop(ctx, p.ID)
	require.NoError(t, err)

	healthy, err = r.Health(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, healthy)
}

type fakeStubCopier struct{ calls [][2]string }

func (f *fakeStubCopier) CopySDKStubs(lang, dir string) error {
	f.calls = append(f.calls, [2]string{lang, dir})
	return nil
}

// TestRunnerStartRefreshesSDKStubs: the server-shipped SDK stubs must be
// re-copied into the plugin's install dir at every Start, not only at install.
// Installed plugins otherwise keep the stubs frozen at install time forever;
// after a server upgrade that adds a host RPC, every such plugin runs against
// a stale contract (préprod 2026-07-11: "host.Fetch is not a function" on a
// plugin whose bundled proto was correct but got clobbered at install by the
// then-stale sdk/ copy).
func TestRunnerStartRefreshesSDKStubs(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	fake := &fakeStubCopier{}
	r := NewRunner(dir).WithAllowMissingSandbox(true).WithSDKStubs(fake)

	p := &plugin.Plugin{ID: "plg-test-stubs", Name: bin, Language: "nodejs", InstallDir: dir}
	require.NoError(t, r.Start(context.Background(), p))
	defer func() { _ = r.Stop(context.Background(), p.ID) }()

	require.Len(t, fake.calls, 1, "Start must refresh the SDK stubs")
	assert.Equal(t, [2]string{"nodejs", dir}, fake.calls[0])
}

func TestRunnerStopNotRunning(t *testing.T) {
	r := NewRunner()
	err := r.Stop(context.Background(), "nonexistent")
	require.ErrorContains(t, err, "not running")
}

// fakeHostRegistry records UnregisterPlugin calls to assert host-socket cleanup.
type fakeHostRegistry struct {
	unregistered []string
}

func (f *fakeHostRegistry) RegisterPlugin(id string) (string, error) {
	return "/tmp/fake-host-" + id + ".sock", nil
}
func (f *fakeHostRegistry) SetExpectedPID(_ string, _ int32) {}
func (f *fakeHostRegistry) UnregisterPlugin(id string) {
	f.unregistered = append(f.unregistered, id)
}

// Regression: Stop must unregister the host socket even when the plugin is no
// longer tracked in r.processes (desynced orphan). Otherwise a later re-enable
// fails with "plugin already registered" until a full server restart.
func TestRunnerStopUnregistersHostSockWhenUntracked(t *testing.T) {
	reg := &fakeHostRegistry{}
	r := NewRunner().WithHostRegistry(reg)
	err := r.Stop(context.Background(), "plg-orphan")
	assert.ErrorContains(t, err, "not running")
	assert.Contains(t, reg.unregistered, "plg-orphan", "host socket must be unregistered on the not-running path")
}

func TestRunnerKillUnregistersHostSockWhenUntracked(t *testing.T) {
	reg := &fakeHostRegistry{}
	r := NewRunner().WithHostRegistry(reg)
	require.NoError(t, r.Kill("plg-orphan"))
	assert.Contains(t, reg.unregistered, "plg-orphan", "host socket must be unregistered on Kill's not-running path")
}

func TestRunnerSendEventNotRunning(t *testing.T) {
	r := NewRunner()
	err := r.SendEvent(context.Background(), "nonexistent", nil)
	assert.ErrorContains(t, err, "not running")
}

func TestRunnerHealthNotRunning(t *testing.T) {
	r := NewRunner()
	healthy, err := r.Health(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestRunnerStartCreatesSocketDir(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()
	r := NewRunner(dir).WithAllowMissingSandbox(true)

	p := &plugin.Plugin{
		ID:   "plg-sockdir-test",
		Name: bin,
	}

	err := r.Start(ctx, p)
	require.NoError(t, err)
	defer r.Stop(ctx, p.ID)

	time.Sleep(50 * time.Millisecond)

	_, err = os.Stat(sockDir)
	assert.NoError(t, err)
}

func TestRunnerRejectsEntrypointOutsidePluginDir(t *testing.T) {
	outsideDir := t.TempDir()
	require.NoError(t, os.Chmod(outsideDir, 0o755))
	require.NoError(t, os.Chmod(filepath.Dir(outsideDir), 0o755))
	outsideBin := filepath.Join(outsideDir, "fake-bin")
	require.NoError(t, os.WriteFile(outsideBin, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	r := NewRunner("/data/plugins")
	ctx := context.Background()

	p := &plugin.Plugin{
		ID:         "plg-bad",
		Entrypoint: outsideBin,
	}

	err := r.Start(ctx, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside allowed plugin directories")
}

func TestRunnerWithLogger(t *testing.T) {
	dir := t.TempDir()
	logger := NewPluginLogger(dir, 10)
	r := NewRunner().WithLogger(logger)
	assert.NotNil(t, r.logger)
}

func TestRunnerWithIPCCallback(t *testing.T) {
	called := false
	cb := func(pluginID string, err error) { called = true }
	r := NewRunner().WithIPCCallback(cb)
	assert.NotNil(t, r.onIPCResult)
	r.onIPCResult("test", nil)
	assert.True(t, called)
}

func TestIsInterpreter(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"python3", true},
		{"python", true},
		{"node", true},
		{"php", true},
		{"ruby", true},
		{"bash", true},
		{"sh", true},
		{"perl", true},
		{"lua", true},
		{"gcc", false},
		{"/usr/bin/node", true},
		{"custom-binary", false},
	}
	for _, tt := range tests {
		if got := isInterpreter(tt.cmd); got != tt.want {
			t.Errorf("isInterpreter(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestValidateEntrypoint_DirectoryIsError(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(dir)
	_, err := r.validateEntrypoint(".", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory")
}

func TestValidateEntrypoint_NotExecutableIsError(t *testing.T) {
	dir := t.TempDir()
	noExec := filepath.Join(dir, "script.py")
	os.WriteFile(noExec, []byte("print('hi')"), 0o644)
	r := NewRunner(dir)
	_, err := r.validateEntrypoint("script.py", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not executable")
}

func TestRunnerStartEmptyEntrypoint(t *testing.T) {
	r := NewRunner(t.TempDir())
	p := &plugin.Plugin{ID: "plg-empty", Entrypoint: ""}
	// Entrypoint falls back to Name, which is also empty
	err := r.Start(context.Background(), p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty entrypoint")
}

func TestRunnerStartInterpreterNotFound(t *testing.T) {
	r := NewRunner(t.TempDir())
	p := &plugin.Plugin{
		ID:         "plg-no-interp",
		Entrypoint: "nonexistent_interpreter main.py",
	}
	err := r.Start(context.Background(), p)
	assert.Error(t, err)
}

func TestRunnerSendEvent_NoIPC(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()
	r := NewRunner(dir).WithAllowMissingSandbox(true)

	p := &plugin.Plugin{ID: "plg-no-ipc", Name: bin}
	err := r.Start(ctx, p)
	require.NoError(t, err)
	defer r.Stop(ctx, p.ID)

	time.Sleep(50 * time.Millisecond)

	err = r.SendEvent(ctx, "plg-no-ipc", map[string]any{"key": "val"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IPC not connected")
}

func TestRunnerRejectsPathTraversal(t *testing.T) {
	r := NewRunner("/data/plugins")
	ctx := context.Background()

	p := &plugin.Plugin{
		ID:         "plg-traversal",
		Entrypoint: "../../../etc/passwd",
		InstallDir: "/data/plugins/evil",
	}

	err := r.Start(ctx, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not contain '..'")
}

// TestRunnerRefusesUnsandboxedStart pins the fail-closed behaviour: when
// the plugin has no 'subprocess' permission and the sandbox lib can't be
// located/compiled, Start must refuse (ErrSandboxUnavailable) rather than
// silently run the plugin unsandboxed.
func TestRunnerRefusesUnsandboxedStart(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()

	// Default Runner = strict. Force "sandbox lib absent" deterministically so
	// ensureSandboxLib returns "" → Start must fail-closed. We must not rely on
	// the ambient absence of /app/libevey-sandbox.so: the prod-like container
	// builds it, which would otherwise flip this test red.
	r := NewRunner(dir).WithSandboxLibPath(t.TempDir() + "/absent-libevey-sandbox.so")
	p := &plugin.Plugin{ID: "plg-nosandbox", Name: bin /* no subprocess perm */}

	err := r.Start(ctx, p)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSandboxUnavailable)
}

// TestRunnerDevOverrideAllowsMissingSandbox pins the dev-mode contract:
// WithAllowMissingSandbox(true) must let a plugin without 'subprocess'
// permission start even when the sandbox lib is absent.  This is intentional
// for developer laptops / CI environments where libevey-sandbox.so is not
// built.  Production wiring MUST NOT pass true here (see wire.go).
func TestRunnerDevOverrideAllowsMissingSandbox(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()

	// Simulate dev mode: sandbox lib absent, bypass enabled.
	r := NewRunner(dir).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{ID: "plg-devoverride", Name: bin /* no subprocess perm */}

	err := r.Start(ctx, p)
	require.NoError(t, err, "dev-mode bypass must allow start without sandbox lib")
	_ = r.Stop(ctx, p.ID)
}

// TestWithSandboxMode_StoresBubblewrap verifies that WithSandboxMode stores
// the "bubblewrap" value on the runner.
func TestWithSandboxMode_StoresBubblewrap(t *testing.T) {
	r := NewRunner().WithSandboxMode("bubblewrap")
	assert.Equal(t, sandboxModeBubblewrap, r.sandboxMode)
}

// TestWithSandboxMode_StoresStrict verifies that WithSandboxMode stores "strict".
func TestWithSandboxMode_StoresStrict(t *testing.T) {
	r := NewRunner().WithSandboxMode("strict")
	assert.Equal(t, sandboxModeStrict, r.sandboxMode)
}

// TestWithSandboxMode_StoresBasic verifies that "basic" is stored as-is.
func TestWithSandboxMode_StoresBasic(t *testing.T) {
	r := NewRunner().WithSandboxMode("basic")
	assert.Equal(t, sandboxModeBasic, r.sandboxMode)
}

// TestPluginProcContext_LongRunning pins the plugin process lifetime contract.
// Plugins are long-running services (gRPC lifecycle stub, periodic jobs); a
// deadline on their exec context SIGKILLs them mid-flight and surfaces as a
// bogus "crashed / OOM" report. Prod proof (préprod 2026-07-05→11): every
// weather-v2 "crash" landed at start+30:00.000 exactly - the old 30-minute
// pluginExecTimeout, not memory.
func TestPluginProcContext_LongRunning(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, cancel := pluginProcContext(parent)
	defer cancel()

	_, hasDeadline := ctx.Deadline()
	assert.False(t, hasDeadline, "plugin process context must not carry a deadline")

	cancelParent()
	require.NoError(t, ctx.Err(), "plugin must outlive the request context that started it")

	cancel()
	assert.Error(t, ctx.Err(), "the returned cancel must still stop the process (Stop/rollback paths)")
}

// TestClassifyExitError_OOMHints pins the human-readable classification of
// SIGKILL crashes. Server-initiated kills (Stop/Kill/resmon) never reach this
// path - they detrack the entry first - so the hint must blame the outside
// world (host OOM-killer / manual kill) and redirect the EVEY_PLUGINS_MEMORY_MB
// suspicion to the resmon's own 'memory quota exceeded' log line. Anything
// else falls through to the raw error string.
func TestClassifyExitError_OOMHints(t *testing.T) {
	t.Run("nil error → empty", func(t *testing.T) {
		assert.Empty(t, classifyExitError(nil))
	})

	t.Run("non-exit error falls through", func(t *testing.T) {
		got := classifyExitError(os.ErrPermission)
		assert.Contains(t, got, "process crashed:")
		assert.Contains(t, got, "permission denied")
	})

	t.Run("SIGKILL → external kill / OOM hint", func(t *testing.T) {
		err := killSelfAndWait(t)
		got := classifyExitError(err)
		assert.Contains(t, got, "outside the server")
		assert.Contains(t, got, "OOM-killer")
		assert.Contains(t, got, "EVEY_PLUGINS_MEMORY_MB",
			"must tell the admin where cap breaches are actually logged")
	})
}

// killSelfAndWait runs `sleep 60`, sends SIGKILL, and returns the Wait error.
// The resulting *exec.ExitError carries WaitStatus.Signal()=SIGKILL - exactly
// the shape produced when the host OOM-killer or cgroup memory.max fires.
func killSelfAndWait(t *testing.T) error {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	require.NoError(t, cmd.Process.Signal(os.Kill))
	return cmd.Wait()
}

// TestWithSandboxMode_InvalidMappedToDefault documents that any unrecognised
// value (including empty string) is silently mapped to the default sandbox mode
// (bubblewrap). This prevents accidental escalation from a config typo -
// the runner stays in the known-safe default rather than refusing to start.
func TestWithSandboxMode_InvalidMappedToDefault(t *testing.T) {
	for _, bad := range []string{"", "none", "BUBBLEWRAP", "unknown"} {
		r := NewRunner().WithSandboxMode(bad)
		assert.Equal(t, defaultSandboxMode, r.sandboxMode,
			"mode %q should be mapped to the default sandbox mode", bad)
	}
}

// TestWithSandboxMode_BwrapAbsent verifies that Start returns an error with
// a helpful message when sandbox_mode=bubblewrap and bwrap is not in PATH.
// This test overrides PATH to guarantee bwrap is absent.
func TestWithSandboxMode_BwrapAbsent(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()

	t.Setenv("PATH", "/nonexistent-empty-path")

	r := NewRunner(dir).
		WithSandboxMode("bubblewrap").
		WithAllowMissingSandbox(true)

	p := &plugin.Plugin{
		ID:   "plg-bwrap-absent",
		Name: bin,
	}

	err := r.Start(ctx, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bwrap")
	assert.Contains(t, err.Error(), "$PATH")
}

// TestRunnerAllowsSubprocessPermWithDevOverride documents that a plugin granted
// the subprocess permission may start WITHOUT launcher confinement only under
// the dev override (WithAllowMissingSandbox). In production (no override) a
// missing launcher fail-closes instead - see
// TestApplyBasicSandbox_SubprocessRequiresLauncherWhenAbsent.
func TestRunnerAllowsSubprocessPermWithDevOverride(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()

	// Force the launcher absent so this exercises the LD_PRELOAD-less path.
	t.Setenv("EVEY_PLUGIN_LAUNCHER", filepath.Join(t.TempDir(), "missing-launcher"))

	r := NewRunner(dir).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{
		ID:          "plg-subproc",
		Name:        bin,
		Permissions: []string{"subprocess"},
	}

	err := r.Start(ctx, p)
	require.NoError(t, err)
	_ = r.Stop(ctx, p.ID)
}

// TestApplyBasicSandbox_SubprocessRequiresLauncherWhenAbsent pins the corrected
// fail-closed contract for the subprocess permission in basic mode. The
// LD_PRELOAD shim does not survive an exec of a child process, so a subprocess
// plugin is confined only by the launcher's kernel seccomp/Landlock. Therefore
// the subprocess permission must NOT unconditionally bypass the sandbox:
//   - launcher present       → allowed (the launcher confines it);
//   - launcher absent + prod  → refused (no confinement layer left);
//   - launcher absent + dev   → allowed (explicit override).
//
// Regression for the audit finding "basic mode starts a subprocess plugin
// unconfined and silently when the launcher binary is missing".
func TestApplyBasicSandbox_SubprocessRequiresLauncherWhenAbsent(t *testing.T) {
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	ctx := context.Background()

	subprocPlugin := func() *plugin.Plugin {
		return &plugin.Plugin{
			ID:          "plg-subproc",
			Name:        bin,
			InstallDir:  dir,
			Permissions: []string{"subprocess"},
		}
	}

	t.Run("launcher present → wrapped and allowed", func(t *testing.T) {
		launcher := filepath.Join(t.TempDir(), "evey-plugin-launcher")
		require.NoError(t, os.WriteFile(launcher, []byte("#!/bin/sh\n"), 0o755))
		t.Setenv("EVEY_PLUGIN_LAUNCHER", launcher)

		r := NewRunner(dir) // production: allowMissingSandbox=false
		cmd := exec.Command(bin)
		err := r.applyBasicSandbox(ctx, subprocPlugin(), cmd)
		require.NoError(t, err)
		assert.Equal(t, launcher, cmd.Path, "subprocess plugin must be wrapped by the launcher")
	})

	t.Run("launcher absent + prod → fail-closed", func(t *testing.T) {
		t.Setenv("EVEY_PLUGIN_LAUNCHER", filepath.Join(t.TempDir(), "missing-launcher"))

		r := NewRunner(dir) // production: no dev override
		cmd := exec.Command(bin)
		err := r.applyBasicSandbox(ctx, subprocPlugin(), cmd)
		require.ErrorIs(t, err, ErrSandboxUnavailable)
	})

	t.Run("launcher absent + dev override → allowed", func(t *testing.T) {
		t.Setenv("EVEY_PLUGIN_LAUNCHER", filepath.Join(t.TempDir(), "missing-launcher"))

		r := NewRunner(dir).WithAllowMissingSandbox(true)
		cmd := exec.Command(bin)
		err := r.applyBasicSandbox(ctx, subprocPlugin(), cmd)
		require.NoError(t, err)
	})
}

// TestSetupSocketDir_UIDSeparationParentTraversable is the regression for the
// prod crash where a uid-dropped plugin got
// "listen EACCES /tmp/evey/plugins/<id>/plugin.sock": the per-plugin dir was
// made group-writable but the 0700 PARENT wasn't traversable by the plugin
// group, so the dropped process couldn't even reach its own socket. Under uid
// separation the parent must grant group SEARCH (no listing); without it the
// parent stays locked down.
func TestSetupSocketDir_UIDSeparationParentTraversable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("targets the non-root uid-separation path; CI runs non-root")
	}
	reset := func() { _ = os.Chmod(sockDir, 0o700) }
	t.Cleanup(reset)

	t.Run("uid-sep on: parent group-traversable + subdir setgid", func(t *testing.T) {
		reset()
		r := NewRunner(t.TempDir()).WithUIDSeparation("on")
		dir, sock, err := r.setupSocketDir("plg-uidsep-on")
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		require.NoError(t, err)
		assert.Contains(t, sock, "plg-uidsep-on")

		parent, err := os.Stat(sockDir)
		require.NoError(t, err)
		assert.NotZero(t, parent.Mode().Perm()&0o010,
			"parent must be group-traversable (group execute) so the dropped uid can reach its socket")
		assert.Zero(t, parent.Mode().Perm()&0o044,
			"parent must NOT be readable by group/other - traverse only, no enumeration")

		sub, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o770), sub.Mode().Perm(), "per-plugin dir must be group rwx")
		assert.NotZero(t, sub.Mode()&os.ModeSetgid, "per-plugin dir must be setgid")
	})

	t.Run("uid-sep off: parent not loosened", func(t *testing.T) {
		reset()
		r := NewRunner(t.TempDir()).WithUIDSeparation("off")
		dir, _, err := r.setupSocketDir("plg-uidsep-off")
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		require.NoError(t, err)

		parent, err := os.Stat(sockDir)
		require.NoError(t, err)
		assert.Zero(t, parent.Mode().Perm()&0o077,
			"parent must grant nothing to group/other when uid separation is off")
	})
}

// In basic mode the uid drop is the secret wall, and only the launcher performs
// it: an ordinary plugin must not fall back to LD_PRELOAD at the server's uid
// when the launcher is missing and uid separation applies.
func TestApplyBasicSandbox_UIDSeparationRequiresLauncher(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("a root server drops the uid itself (SysProcAttr), not via the launcher")
	}
	bin := buildTestBinary(t)
	dir := filepath.Dir(bin)
	lib := filepath.Join(t.TempDir(), "libevey-sandbox.so")
	require.NoError(t, os.WriteFile(lib, nil, 0o644))
	t.Setenv("EVEY_PLUGIN_LAUNCHER", filepath.Join(t.TempDir(), "missing-launcher"))
	p := &plugin.Plugin{ID: "plg-basic", Name: bin, InstallDir: dir}

	run := func(r *Runner) (*exec.Cmd, error) {
		r.sandboxLibOverride = lib
		cmd := exec.Command(bin)
		return cmd, r.applyBasicSandbox(context.Background(), p, cmd)
	}

	t.Run("uid separation on, launcher absent → refused", func(t *testing.T) {
		_, err := run(NewRunner(dir).WithUIDSeparation(uidSepOn))
		require.ErrorIs(t, err, ErrSandboxUnavailable)
	})

	t.Run("uid separation off → LD_PRELOAD fallback kept", func(t *testing.T) {
		cmd, err := run(NewRunner(dir).WithUIDSeparation(uidSepOff))
		require.NoError(t, err)
		assert.Contains(t, cmd.Env, "LD_PRELOAD="+lib)
	})

	t.Run("dev override → allowed", func(t *testing.T) {
		_, err := run(NewRunner(dir).WithUIDSeparation(uidSepAuto).WithAllowMissingSandbox(true))
		require.NoError(t, err)
	})
}
