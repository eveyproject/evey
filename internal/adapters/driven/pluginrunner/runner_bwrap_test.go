package pluginrunner

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBwrapCmd_BasicReturnsNil(t *testing.T) {
	result := buildBwrapCmd("/data/plugins/foo", true, sandboxModeBasic, "")
	assert.Nil(t, result, "basic mode must return nil (no bwrap prefix)")
}

func TestBuildBwrapCmd_BubblewrapWithoutNetworkHasUnshareNet(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", false, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.Equal(t, "bwrap", args[0])
	assert.Contains(t, args, "--unshare-net", "plugin without network perm must get --unshare-net")
}

func TestBuildBwrapCmd_BubblewrapWithNetworkNoUnshareNet(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", true, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.NotContains(t, args, "--unshare-net", "plugin with network perm must NOT get --unshare-net")
}

func TestBuildBwrapCmd_StrictHasBwrapFlags(t *testing.T) {
	// strict mode should produce the same bwrap prefix as bubblewrap
	// (the LD_PRELOAD addition happens in applyBwrapSandbox, not here).
	args := buildBwrapCmd("/data/plugins/bar", false, sandboxModeStrict, "")
	require.NotNil(t, args)
	assert.Equal(t, "bwrap", args[0])
	assert.Contains(t, args, "--unshare-user")
	assert.Contains(t, args, "--unshare-pid")
	assert.Contains(t, args, "--unshare-net")
}

func TestBuildBwrapCmd_ContainsROBindAndPluginBind(t *testing.T) {
	pluginDir := "/data/plugins/my-plugin"
	args := buildBwrapCmd(pluginDir, false, sandboxModeBubblewrap, "")
	require.NotNil(t, args)

	// Must have --ro-bind / /
	roIdx := slices.Index(args, "--ro-bind")
	require.NotEqual(t, -1, roIdx, "--ro-bind flag must be present")
	require.Less(t, roIdx+2, len(args), "--ro-bind must have two following arguments")
	assert.Equal(t, "/", args[roIdx+1])
	assert.Equal(t, "/", args[roIdx+2])

	// Must have --bind <pluginDir> <pluginDir>
	bindIdx := slices.Index(args, "--bind")
	require.NotEqual(t, -1, bindIdx, "--bind flag must be present")
	require.Less(t, bindIdx+2, len(args), "--bind must have two following arguments")
	assert.Equal(t, pluginDir, args[bindIdx+1])
	assert.Equal(t, pluginDir, args[bindIdx+2])
}

func TestBuildBwrapCmd_EndsWithDoubleDash(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", false, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.Equal(t, "--", args[len(args)-1], "last arg before plugin command must be --")
}

func TestBuildBwrapCmd_ContainsProcDevTmpfs(t *testing.T) {
	args := buildBwrapCmd("/data/plugins/foo", true, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.Contains(t, args, "--proc")
	assert.Contains(t, args, "--dev-bind")
	assert.Contains(t, args, "--tmpfs")
	assert.Contains(t, args, "--die-with-parent")
}

func TestBuildBwrapCmd_WithHostsFile(t *testing.T) {
	hostsFile := "/tmp/evey-plugin-hosts-test123"
	args := buildBwrapCmd("/data/plugins/foo", true, sandboxModeBubblewrap, hostsFile)
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
	args := buildBwrapCmd("/data/plugins/foo", true, sandboxModeBubblewrap, "")
	require.NotNil(t, args)
	assert.NotContains(t, args, "/etc/hosts", "no hostsFile means /etc/hosts must not appear in args")
}
