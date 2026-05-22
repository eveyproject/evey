package pluginrunner

import (
	"context"
	"os/exec"
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
	assert.Contains(t, cmd.Args, "/dev", "pseudo-devices must be on the allowlist (/dev/null, /dev/urandom)")
	assert.Contains(t, cmd.Args, "-require-landlock")
	assert.Contains(t, cmd.Args, "--")

	// network permission → TCP egress restricted but HTTP(S) ports allowed.
	assert.Contains(t, cmd.Args, "-restrict-net")
	assert.Contains(t, cmd.Args, "-net-connect")
	assert.Contains(t, cmd.Args, "80")
	assert.Contains(t, cmd.Args, "443")

	// The original command is preserved at the tail, after the "--".
	assert.Equal(t, "/usr/bin/node", cmd.Args[len(cmd.Args)-2])
	assert.Equal(t, "/data/plugins/weather/main.js", cmd.Args[len(cmd.Args)-1])

	// HOME/TMPDIR are repointed to the (writable) plugin dir, not /tmp.
	assert.Contains(t, cmd.Env, "HOME=/data/plugins/weather")
	assert.Contains(t, cmd.Env, "TMPDIR=/data/plugins/weather")
	assert.Contains(t, cmd.Env, "EVEY_PLUGIN_ID=weather", "unrelated env preserved")
}

func TestApplyLandlockSandbox_NoNetworkPerm_CutsOffTCP(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")

	r := NewRunner().WithSandboxMode(sandboxModeLandlock).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{ID: "themer", InstallDir: "/data/plugins/themer"} // no permissions
	cmd := exec.Command("/usr/bin/node", "/data/plugins/themer/main.js")

	_, err := r.applyLandlockSandbox(context.Background(), p, cmd)
	require.NoError(t, err)

	// restrict-net is always set; with no network permission there must be no
	// -net-connect, so the launcher denies all TCP.
	assert.Contains(t, cmd.Args, "-restrict-net")
	assert.NotContains(t, cmd.Args, "-net-connect")
}

func TestApplyLandlockSandbox_NoSubprocessPerm_RequiresSeccompLib(t *testing.T) {
	t.Setenv("EVEY_PLUGIN_LAUNCHER", "/bin/true")
	// allowMissingSandbox defaults to false → fail-closed when the lib is absent
	// (it is, on the test box: /app/libevey-sandbox.so does not exist).
	r := NewRunner().WithSandboxMode(sandboxModeLandlock)
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

func TestLandlockConnectPorts(t *testing.T) {
	// Defaults: HTTP + HTTPS only.
	assert.Equal(t, []int{80, 443}, landlockConnectPorts(nil))
	assert.Equal(t, []int{80, 443}, landlockConnectPorts([]string{"api.open-meteo.com", "example.com"}))
	// Explicit ":port" entries are added (deduped, 80/443 kept first).
	got := landlockConnectPorts([]string{"api.example.com:8443", "other.com:443", " host.io:9000 "})
	assert.Equal(t, []int{80, 443, 8443, 9000}, got)
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
