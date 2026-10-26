package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBwrapCmd_BasicReturnsNil(t *testing.T) {
	result := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", sandboxModeBasic)
	assert.Nil(t, result, "basic mode must return nil (no bwrap prefix)")
}

func TestBuildBwrapCmd_AlwaysUnsharesNet(t *testing.T) {
	// Direct plugin egress is cut unconditionally now: EVERY sandboxed plugin
	// gets --unshare-net regardless of the network permission. Network access is
	// mediated by the host Fetch broker over the AF_UNIX host socket.
	for _, mode := range []string{sandboxModeBubblewrap, sandboxModeStrict} {
		args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", mode)
		require.NotNil(t, args)
		assert.Equal(t, "bwrap", args[0])
		assert.Contains(t, args, "--unshare-net", "every plugin must get --unshare-net (mode %s)", mode)
	}
}

func TestBuildBwrapCmd_StrictHasBwrapFlags(t *testing.T) {
	// strict mode should produce the same bwrap prefix as bubblewrap
	// (the LD_PRELOAD addition happens in applyBwrapSandbox, not here).
	args := buildBwrapCmd("/data/plugins/bar", "/tmp/evey/plugins/test", sandboxModeStrict)
	require.NotNil(t, args)
	assert.Equal(t, "bwrap", args[0])
	assert.Contains(t, args, "--unshare-user")
	assert.Contains(t, args, "--unshare-pid")
	assert.Contains(t, args, "--unshare-net")
}

func TestBuildBwrapCmd_ContainsROBindAndPluginBind(t *testing.T) {
	pluginDir := "/data/plugins/my-plugin"
	args := buildBwrapCmd(pluginDir, "/tmp/evey/plugins/test", sandboxModeBubblewrap)
	require.NotNil(t, args)

	// Must have at least one --ro-bind (selective whitelist, not / /)
	roIdx := slices.Index(args, "--ro-bind")
	require.NotEqual(t, -1, roIdx, "--ro-bind flag must be present")

	// Must have --bind <pluginDir> <pluginDir>
	bindIdx := slices.Index(args, "--bind")
	require.NotEqual(t, -1, bindIdx, "--bind flag must be present")
	require.Less(t, bindIdx+2, len(args), "--bind must have two following arguments")
	assert.Equal(t, pluginDir, args[bindIdx+1])
	assert.Equal(t, pluginDir, args[bindIdx+2])
}

func TestBuildBwrapCmd_EndsWithDoubleDash(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", sandboxModeBubblewrap)
	require.NotNil(t, args)
	assert.Equal(t, "--", args[len(args)-1], "last arg before plugin command must be --")
}

func TestBuildBwrapCmd_ContainsProcDevTmpfs(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", sandboxModeBubblewrap)
	require.NotNil(t, args)
	assert.Contains(t, args, "--proc")
	assert.Contains(t, args, "--dev")
	assert.NotContains(t, args, "--dev-bind", "must use synthesised --dev, not host /dev bind-mount")
	assert.Contains(t, args, "--tmpfs")
	assert.Contains(t, args, "--die-with-parent")
}

func TestBuildBwrapCmd_NoEtcHostsBind(t *testing.T) {
	// The /etc/hosts DNS whitelist was retired: with no network namespace there
	// is nothing to resolve, and host allow-listing now lives in the broker.
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", sandboxModeBubblewrap)
	require.NotNil(t, args)
	assert.NotContains(t, args, "/etc/hosts", "no /etc/hosts bind should be emitted anymore")
}

func TestBwrapMountWhitelist(t *testing.T) {
	// pluginDir intentionally NOT under /data - the test asserts that no
	// host-secret path is bind-mounted; the plugin's own writable workdir
	// is a legitimate --bind regardless of where the operator places it.
	pluginDir := "/srv/evey-plugins/foo"
	args := buildBwrapCmd(pluginDir, "/tmp/evey/plugins/test", sandboxModeBubblewrap)
	require.NotNil(t, args)

	// The broad root bind must be gone.
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--ro-bind" && args[i+1] == "/" && args[i+2] == "/" {
			t.Fatal("--ro-bind / / must not appear: it exposes the entire host filesystem")
		}
	}

	// /usr must be present (covers python, node, php, lua interpreters on Alpine/Debian).
	foundUsr := false
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--ro-bind" && args[i+1] == "/usr" && args[i+2] == "/usr" {
			foundUsr = true
			break
		}
	}
	assert.True(t, foundUsr, "--ro-bind /usr /usr must be present for runtime interpreters")

	// /data must not appear as a bind source (secrets are stored there).
	for i := 0; i < len(args)-1; i++ {
		if (args[i] == "--ro-bind" || args[i] == "--ro-bind-try" || args[i] == "--bind") &&
			len(args[i+1]) >= 5 && args[i+1][:5] == "/data" {
			t.Fatalf("bind of /data path must not appear, got: %s %s", args[i], args[i+1])
		}
	}

	// /var must not appear as a bind source.
	for i := 0; i < len(args)-1; i++ {
		if (args[i] == "--ro-bind" || args[i] == "--ro-bind-try" || args[i] == "--bind") &&
			len(args[i+1]) >= 4 && args[i+1][:4] == "/var" {
			t.Fatalf("bind of /var path must not appear, got: %s %s", args[i], args[i+1])
		}
	}
}

// The sandboxed process keeps no capability inside its user namespace, and no
// new session: setsid would take the plugin out of the process group every
// signal (stop, kill, the boot sweep) is sent to.
func TestBuildBwrapCmd_DropsCapsKeepsSession(t *testing.T) {
	for _, mode := range []string{sandboxModeBubblewrap, sandboxModeStrict} {
		args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", mode)
		require.NotNil(t, args)
		i := slices.Index(args, "--cap-drop")
		require.GreaterOrEqual(t, i, 0, "%s: --cap-drop missing", mode)
		assert.Equal(t, "ALL", args[i+1])
		assert.NotContains(t, args, "--new-session", mode)
	}
}

func TestBuildBwrapCmd_NoReaperBeforeThePlugin(t *testing.T) {
	// Without --as-pid-1 bwrap forks a reaper and the plugin is its child: the
	// PID bwrap reports would then not be the one that talks to the host socket.
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", sandboxModeBubblewrap)
	assert.Contains(t, args, "--as-pid-1")
}

func TestWithBwrapInfoFD_GoesBeforeTheSeparator(t *testing.T) {
	prefix := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", sandboxModeBubblewrap)
	args := withBwrapInfoFD(prefix, 3)
	require.Len(t, args, len(prefix)+2)
	assert.Equal(t, []string{"--info-fd", "3", "--"}, args[len(args)-3:])
	assert.Equal(t, "--", prefix[len(prefix)-1], "the builder's own prefix must stay untouched")
}

func TestReadBwrapChildPID(t *testing.T) {
	read := func(t *testing.T, payload string) (int, error) {
		t.Helper()
		r, w, err := os.Pipe()
		require.NoError(t, err)
		_, err = w.WriteString(payload)
		require.NoError(t, err)
		require.NoError(t, w.Close())
		return readBwrapChildPID(r)
	}

	pid, err := read(t, "{\n    \"child-pid\": 4242,\n    \"cgroup-namespace\": 4026532301\n}\n")
	require.NoError(t, err)
	assert.Equal(t, 4242, pid)

	_, err = read(t, "")
	require.Error(t, err, "bwrap dying before the clone closes the fd with nothing written")

	_, err = read(t, "{}")
	require.Error(t, err, "a report without child-pid must not arm PID 0")
}

// TestApplyBwrapSandbox_PayloadRunsThroughLauncher: bubblewrap alone has no
// syscall filter, so the payload inside it is the launcher, which installs the
// seccomp denylist (incl. -deny-inet) before exec'ing the plugin argv.
func TestApplyBwrapSandbox_PayloadRunsThroughLauncher(t *testing.T) {
	binDir := t.TempDir()
	fakeBwrap := filepath.Join(binDir, "bwrap")
	require.NoError(t, os.WriteFile(fakeBwrap, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // test stub must be executable
	t.Setenv("PATH", binDir)
	launcher := filepath.Join(binDir, launcherBinaryName)
	require.NoError(t, os.WriteFile(launcher, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // test stub must be executable
	t.Setenv("EVEY_PLUGIN_LAUNCHER", launcher)

	r := NewRunner(t.TempDir()).WithAllowMissingSandbox(true)
	p := &plugin.Plugin{ID: "bwrap-launcher", InstallDir: "/data/plugins/bwrap-launcher"}
	cmd := exec.Command("/usr/bin/node", "main.js")
	_, info, err := r.applyBwrapSandbox(context.Background(), p, cmd, sandboxModeBubblewrap)
	require.NoError(t, err)
	t.Cleanup(func() { _ = info.Close() })

	sep := slices.Index(cmd.Args, "--")
	require.Positive(t, sep)
	bwrapFlags, payload := cmd.Args[:sep], cmd.Args[sep+1:]
	assert.Contains(t, strings.Join(bwrapFlags, " "), "--ro-bind "+launcher+" "+launcher,
		"the launcher must be visible inside the sandbox")
	require.NotEmpty(t, payload)
	assert.Equal(t, launcher, payload[0])
	assert.Contains(t, payload, "-seccomp")
	assert.Contains(t, payload, "-deny-inet")
	assert.Equal(t, []string{"/usr/bin/node", "main.js"}, payload[len(payload)-2:])
	assert.NotContains(t, payload, "-uid", "bwrap maps its own user namespace; no setuid inside")
	assert.Contains(t, cmd.Env, "TMPDIR=/data/plugins/bwrap-launcher")
}

// TestBuildBwrapCmd_MasksTTY: /dev/tty (the terminal the server was started
// from, TIOCSTI) is masked by an unopenable bind of /dev/null.
func TestBuildBwrapCmd_MasksTTY(t *testing.T) {
	for _, mode := range []string{sandboxModeBubblewrap, sandboxModeStrict} {
		args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", mode)
		joined := strings.Join(args, " ")
		assert.Contains(t, joined, "--ro-bind /dev/null /dev/tty", mode)
		assert.Less(t, strings.Index(joined, "--dev /dev"), strings.Index(joined, "--ro-bind /dev/null /dev/tty"),
			"%s: the mask must come after --dev builds /dev", mode)
	}
}
