package pluginrunner

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBwrapCmd_BasicReturnsNil(t *testing.T) {
	result := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", true, sandboxModeBasic, "")
	assert.Nil(t, result, "basic mode must return nil (no bwrap prefix)")
}

func TestBuildBwrapCmd_BubblewrapWithoutNetworkHasUnshareNet(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", false, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.Equal(t, "bwrap", args[0])
	assert.Contains(t, args, "--unshare-net", "plugin without network perm must get --unshare-net")
}

func TestBuildBwrapCmd_BubblewrapWithNetworkNoUnshareNet(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", true, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.NotContains(t, args, "--unshare-net", "plugin with network perm must NOT get --unshare-net")
}

func TestBuildBwrapCmd_StrictHasBwrapFlags(t *testing.T) {
	// strict mode should produce the same bwrap prefix as bubblewrap
	// (the LD_PRELOAD addition happens in applyBwrapSandbox, not here).
	args := buildBwrapCmd("/data/plugins/bar", "/tmp/evey/plugins/test", false, sandboxModeStrict, "")
	require.NotNil(t, args)
	assert.Equal(t, "bwrap", args[0])
	assert.Contains(t, args, "--unshare-user")
	assert.Contains(t, args, "--unshare-pid")
	assert.Contains(t, args, "--unshare-net")
}

func TestBuildBwrapCmd_ContainsROBindAndPluginBind(t *testing.T) {
	pluginDir := "/data/plugins/my-plugin"
	args := buildBwrapCmd(pluginDir, "/tmp/evey/plugins/test", false, sandboxModeBubblewrap, "")
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
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", false, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.Equal(t, "--", args[len(args)-1], "last arg before plugin command must be --")
}

func TestBuildBwrapCmd_ContainsProcDevTmpfs(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", true, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.Contains(t, args, "--proc")
	assert.Contains(t, args, "--dev")
	assert.NotContains(t, args, "--dev-bind", "must use synthesised --dev, not host /dev bind-mount")
	assert.Contains(t, args, "--tmpfs")
	assert.Contains(t, args, "--die-with-parent")
}

func TestBuildBwrapCmd_WithHostsFile(t *testing.T) {
	hostsFile := "/tmp/evey-plugin-hosts-test123"
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", true, sandboxModeBubblewrap, hostsFile)
	require.NotNil(t, args)

	// Find the --ro-bind for hostsFile
	found := false
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--ro-bind" && args[i+1] == hostsFile && args[i+2] == "/etc/hosts" {
			found = true
			break
		}
	}
	assert.True(t, found, "args must contain --ro-bind <hostsFile> /etc/hosts")
}

func TestBuildBwrapCmd_NoHostsFile(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", "/tmp/evey/plugins/test", true, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.NotContains(t, args, "/etc/hosts", "no hostsFile means /etc/hosts must not appear in args")
}

func TestBwrapMountWhitelist(t *testing.T) {
	// pluginDir intentionally NOT under /data — the test asserts that no
	// host-secret path is bind-mounted; the plugin's own writable workdir
	// is a legitimate --bind regardless of where the operator places it.
	pluginDir := "/srv/evey-plugins/foo"
	args := buildBwrapCmd(pluginDir, "/tmp/evey/plugins/test", false, sandboxModeBubblewrap, "")
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
