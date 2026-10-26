package pluginrunner

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPluginDropArgs_OneUIDPerPlugin: under uid separation two plugins must not
// share an identity, or one can signal, ptrace-check or read the other's files.
func TestPluginDropArgs_OneUIDPerPlugin(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("a root server drops through SysProcAttr, not the launcher args")
	}
	r := NewRunner(t.TempDir()).WithUIDSeparation(uidSepOn)
	uidA, gidA := r.pluginDropArgs("plg-alpha", true)
	uidB, _ := r.pluginDropArgs("plg-beta", true)
	require.NotZero(t, uidA)
	assert.NotEqual(t, uidA, uidB, "two plugins share a uid")
	assert.Equal(t, os.Getgid(), gidA, "the shared server group keeps the socket dir reachable")

	again, _ := r.pluginDropArgs("plg-alpha", true)
	assert.Equal(t, uidA, again, "the uid must be stable across restarts")
	for _, uid := range []int{uidA, uidB} {
		assert.GreaterOrEqual(t, uid, pluginUIDBase)
		assert.Less(t, uid, pluginUIDBase+pluginUIDSpan)
		assert.True(t, isPluginUID(uid), "reap and signal must recognise %d", uid)
	}
	assert.True(t, isPluginUID(pluginUID), "plugins started by a root server keep the shared uid")
	assert.False(t, isPluginUID(os.Getuid()))
}
