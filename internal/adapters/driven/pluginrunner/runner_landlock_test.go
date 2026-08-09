package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyLandlockSandbox_BuildsArgv(t *testing.T) {
	// /bin/true is a real executable so launcherPath() resolves the override.
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")

	r := NewRunner().WithSandboxMode(sandboxModeLandlock).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{
		ID:          "weather",
		InstallDir:  "/data/plugins/weather",
		Permissions: []string{"network"},
	}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/weather/main.js")
	cmd.Env = []string{"HOME=/tmp", "TMPDIR=/tmp", "EVEY_PLUGIN_ID=weather"}

	cleanup, err := r.applyLandlockSandbox(context.Background(), p, cmd)
	require.NoError(t, err)
	if cleanup != nil {
		cleanup()
	}

	assert.Equal(t, "/bin/true", cmd.Path, "cmd.Path must be the launcher")
	assert.Equal(t, "/bin/true", cmd.Args[0], "argv[0] must be the launcher")

	// The system dirs are allowlisted read-only and the plugin dirs read-write.
	assert.Contains(t, cmd.Args, "-ro")
	assert.Contains(t, cmd.Args, "/usr")
	assert.Contains(t, cmd.Args, "-rw")
	assert.Contains(t, cmd.Args, "/data/plugins/weather")
	assert.Contains(t, cmd.Args, "/tmp/evey/plugins/weather")
	// Only the harmless pseudo-devices are allowlisted, individually via -dev -
	// NOT the whole /dev directory, so GPU render nodes (/dev/dri/*), /dev/dxg
	// and /dev/nvidia* stay denied by Landlock on a hardware-transcode box.
	assert.Contains(t, cmd.Args, "-dev")
	assert.Contains(t, cmd.Args, "/dev/null")
	assert.Contains(t, cmd.Args, "/dev/urandom")
	assert.NotContains(t, cmd.Args, "/dev", "the whole /dev dir must not be granted (would expose GPU device nodes)")
	// This plugin lacks hardware.gpu → the GPU render node must NOT be granted.
	assert.NotContains(t, cmd.Args, "-render-dev", "render node granted only with hardware.gpu")
	assert.Contains(t, cmd.Args, "-require-landlock")
	assert.Contains(t, cmd.Args, "--")

	// Direct TCP egress is cut even WITH the network permission: -restrict-net
	// with no -net-connect grants, plus -deny-inet. The network permission
	// unlocks the host Fetch broker (AF_UNIX), not a direct socket.
	assert.Contains(t, cmd.Args, "-restrict-net")
	assert.NotContains(t, cmd.Args, "-net-connect", "no plugin gets a direct TCP grant anymore")
	assert.Contains(t, cmd.Args, "-deny-inet", "AF_INET/AF_INET6 sockets denied for every plugin")

	// The original command is preserved at the tail, after the "--".
	assert.Equal(t, "/usr/bin/node", cmd.Args[len(cmd.Args)-2])
	assert.Equal(t, "/data/plugins/weather/main.js", cmd.Args[len(cmd.Args)-1])

	// HOME/TMPDIR are repointed to the (writable) plugin dir, not /tmp.
	assert.Contains(t, cmd.Env, "HOME=/data/plugins/weather")
	assert.Contains(t, cmd.Env, "TMPDIR=/data/plugins/weather")
	assert.Contains(t, cmd.Env, "EVEY_PLUGIN_ID=weather", "unrelated env preserved")
}

// The device allowlist must stay stateless pseudo-devices only. /dev/tty is the
// SERVER's controlling terminal: granting it to every plugin, with no
// permission, hands them ioctl(TIOCSTI) - characters pushed into the terminal's
// input queue run as whoever is attached to it. dev.tty.legacy_tiocsti only
// exists from Linux 6.2 while Landlock needs >=5.13, so the whole 5.13-6.1 range
// (an ordinary NAS kernel) is exposed, and ioctl is not in the seccomp denylist.
// A plugin is a daemon with stdout/stderr already wired; it has no terminal to
// want.
func TestBuildLauncherArgv_DeviceAllowlistIsStatelessPseudoDevicesOnly(t *testing.T) {
	p := &plugin.Plugin{ID: "weather", InstallDir: "/data/plugins/weather"}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/weather/main.js")
	args := buildLauncherArgv("/bin/true", p, cmd, false, 0, 0)

	assert.NotContains(t, args, "/dev/tty", "the server's controlling terminal must never be in the allowlist")
	for _, dev := range safeDevNodes {
		assert.Contains(t, args, dev)
	}
	// A tty by any other path is the same device.
	for _, banned := range []string{"/dev/tty", "/dev/console", "/dev/ptmx", "/dev/pts"} {
		assert.NotContains(t, safeDevNodes, banned)
	}
}

// glibc getenv returns the FIRST match, so appending LD_PRELOAD behind an
// existing one leaves the seccomp shim unloaded - silently, with no error and no
// log. buildPluginEnv is a closed allowlist today, so no such entry exists; this
// pins the behaviour so the shim does not quietly stop loading the day plugins
// can supply their own environment.
func TestSandboxLib_LDPreloadOverridesRatherThanAppends(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "libevey-sandbox.so")
	require.NoError(t, os.WriteFile(lib, []byte("stub"), 0o600))
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true") // real executable: launcherPath resolves

	for _, tc := range []struct {
		name  string
		apply func(r *Runner, p *plugin.Plugin, cmd *exec.Cmd)
	}{
		{"basic mode", func(r *Runner, p *plugin.Plugin, cmd *exec.Cmd) {
			require.NoError(t, r.applyBasicSandbox(context.Background(), p, cmd))
		}},
		{"landlock mode", func(r *Runner, p *plugin.Plugin, cmd *exec.Cmd) {
			_, err := r.applyLandlockSandbox(context.Background(), p, cmd)
			require.NoError(t, err)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Runner{sandboxLibOverride: lib, allowMissingSandbox: true}
			p := &plugin.Plugin{ID: "weather", InstallDir: "/data/plugins/weather"}
			cmd := exec.Command("/usr/bin/node", "/data/plugins/weather/main.js")
			cmd.Env = []string{"LD_PRELOAD=/tmp/attacker.so", "EVEY_PLUGIN_ID=weather"}

			tc.apply(r, p, cmd)

			var preloads []string
			for _, e := range cmd.Env {
				if strings.HasPrefix(e, "LD_PRELOAD=") {
					preloads = append(preloads, e)
				}
			}
			require.Len(t, preloads, 1, "a second LD_PRELOAD entry is dead weight: glibc reads the first")
			assert.Equal(t, "LD_PRELOAD="+lib, preloads[0], "the shim must win, not the pre-existing value")
		})
	}
}

func TestBuildLauncherArgv_UIDSeparation(t *testing.T) {
	p := &plugin.Plugin{ID: "weather", InstallDir: "/data/plugins/weather"}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/weather/main.js")

	t.Run("no -uid when dropUID is 0 (default, unchanged behaviour)", func(t *testing.T) {
		args := buildLauncherArgv("/bin/true", p, cmd, false, 0, 0)
		assert.NotContains(t, args, "-uid")
		assert.NotContains(t, args, "-gid")
	})

	t.Run("appends -uid/-gid as flags before the -- separator", func(t *testing.T) {
		args := buildLauncherArgv("/bin/true", p, cmd, false, 1001, 1000)
		uidIdx := slices.Index(args, "-uid")
		require.GreaterOrEqual(t, uidIdx, 0, "-uid must be present")
		assert.Equal(t, "1001", args[uidIdx+1])
		gidIdx := slices.Index(args, "-gid")
		require.GreaterOrEqual(t, gidIdx, 0, "-gid must be present")
		assert.Equal(t, "1000", args[gidIdx+1])
		sep := slices.Index(args, "--")
		require.GreaterOrEqual(t, sep, 0)
		assert.Less(t, uidIdx, sep, "-uid must precede the -- separator (it is a launcher flag)")
		assert.Contains(t, args, "-uid-besteffort", "uid drop is best-effort (warn+continue, not fail-closed)")
		// the original command stays at the tail, untouched
		assert.Equal(t, "/usr/bin/node", args[len(args)-2])
		assert.Equal(t, "/data/plugins/weather/main.js", args[len(args)-1])
	})
}

// TestBuildLauncherArgv_DenyFork pins the escape hatch that keeps legitimate
// plugin development workable: process creation is denied in the kernel by
// default, and declaring the "subprocess" permission - the same one that
// already governs the LD_PRELOAD shim and the execve denial - lifts it. Without
// this pairing, a plugin that legitimately spawns a child would be unfixable
// by its author.
func TestBuildLauncherArgv_DenyFork(t *testing.T) {
	cmd := exec.Command("/usr/bin/node", "/data/plugins/weather/main.js")

	t.Run("denied by default", func(t *testing.T) {
		p := &plugin.Plugin{ID: "weather", InstallDir: "/data/plugins/weather"}
		args := buildLauncherArgv("/bin/true", p, cmd, false, 0, 0)
		idx := slices.Index(args, "-deny-fork")
		require.GreaterOrEqual(t, idx, 0, "-deny-fork must be present for a plugin without subprocess")
		sep := slices.Index(args, "--")
		require.GreaterOrEqual(t, sep, 0)
		assert.Less(t, idx, sep, "-deny-fork must precede the -- separator (it is a launcher flag)")
	})

	t.Run("lifted by the subprocess permission", func(t *testing.T) {
		p := &plugin.Plugin{
			ID: "runner", InstallDir: "/data/plugins/runner",
			Permissions: []string{subprocessPerm},
		}
		args := buildLauncherArgv("/bin/true", p, cmd, false, 0, 0)
		assert.NotContains(t, args, "-deny-fork",
			"a plugin that declared subprocess must keep the right to spawn children")
	})
}

func TestUIDSepApplies(t *testing.T) {
	cases := []struct {
		mode    string
		isBasic bool
		want    bool
	}{
		{uidSepOn, true, true},
		{uidSepOn, false, true},
		{uidSepAuto, true, true},   // basic → enable (the case it protects)
		{uidSepAuto, false, false}, // landlock/bwrap → redundant, skip
		{uidSepOff, true, false},
		{uidSepOff, false, false},
		{"", true, false}, // unset → never
	}
	for _, c := range cases {
		if got := uidSepApplies(c.mode, c.isBasic); got != c.want {
			t.Errorf("uidSepApplies(%q, basic=%v) = %v, want %v", c.mode, c.isBasic, got, c.want)
		}
	}
}

func TestApplyLandlockSandbox_NoNetworkPerm_CutsOffTCP(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")

	r := NewRunner().WithSandboxMode(sandboxModeLandlock).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{ID: "themer", InstallDir: "/data/plugins/themer"} // no permissions
	cmd := exec.Command("/usr/bin/node", "/data/plugins/themer/main.js")

	_, err := r.applyLandlockSandbox(context.Background(), p, cmd)
	require.NoError(t, err)

	// restrict-net is always set, there is never a -net-connect grant, and
	// -deny-inet blocks AF_INET sockets outright.
	assert.Contains(t, cmd.Args, "-restrict-net")
	assert.NotContains(t, cmd.Args, "-net-connect")
	assert.Contains(t, cmd.Args, "-deny-inet")
}

func TestApplyLandlockSandbox_NoSubprocessPerm_RequiresSeccompLib(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")
	// allowMissingSandbox defaults to false → fail-closed when the lib is absent.
	// Force "lib absent" deterministically: the prod-like container builds the
	// real /app/libevey-sandbox.so, so we must not rely on its ambient absence.
	r := NewRunner().WithSandboxMode(sandboxModeLandlock).
		WithSandboxLibPath(t.TempDir() + "/absent-libevey-sandbox.so")
	p := &plugin.Plugin{ID: "x", InstallDir: "/data/plugins/x"} // no subprocess permission
	cmd := exec.Command("/usr/bin/node", "/data/plugins/x/main.js")

	_, err := r.applyLandlockSandbox(context.Background(), p, cmd)
	require.ErrorIs(t, err, ErrSandboxUnavailable, "a plugin without 'subprocess' must be refused when the seccomp lib is missing")
}

func TestApplyLandlockSandbox_SubprocessPerm_SkipsSeccompLib(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")
	// A plugin that declares "subprocess" is allowed to spawn → the seccomp lib
	// is skipped, so a missing lib must NOT block it even with fail-closed on.
	r := NewRunner().WithSandboxMode(sandboxModeLandlock)
	p := &plugin.Plugin{ID: "x", InstallDir: "/data/plugins/x", Permissions: []string{subprocessPerm}}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/x/main.js")

	_, err := r.applyLandlockSandbox(context.Background(), p, cmd)
	require.NoError(t, err)
	for _, e := range cmd.Env {
		assert.NotContains(t, e, "LD_PRELOAD", "subprocess plugins must not get the execve-blocking filter")
	}
}

func TestApplyLandlockSandbox_MissingLauncherFailsClosed(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/nonexistent/evey-plugin-launcher")

	r := NewRunner().WithSandboxMode(sandboxModeLandlock).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{ID: "x", InstallDir: "/data/plugins/x"}
	cmd := exec.Command("/usr/bin/node", "/data/plugins/x/main.js")

	_, err := r.applyLandlockSandbox(context.Background(), p, cmd)
	require.Error(t, err, "a missing launcher must refuse to start the plugin")
}

func TestOverrideEnv(t *testing.T) {
	in := []string{"HOME=/tmp", "PATH=/usr/bin", "TMPDIR=/tmp"}
	out := overrideEnv(in, map[string]string{"HOME": "/data/x", "TMPDIR": "/data/x"})
	assert.Contains(t, out, "HOME=/data/x")
	assert.Contains(t, out, "TMPDIR=/data/x")
	assert.Contains(t, out, "PATH=/usr/bin", "unrelated keys untouched")
	assert.NotContains(t, out, "HOME=/tmp", "old value replaced in place, not duplicated")

	// Absent key gets appended.
	out2 := overrideEnv([]string{"PATH=/usr/bin"}, map[string]string{"HOME": "/data/x"})
	assert.Contains(t, out2, "HOME=/data/x")
	assert.Contains(t, out2, "PATH=/usr/bin")
}
