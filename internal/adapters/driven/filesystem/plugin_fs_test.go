package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPluginFileSystem(t *testing.T) {
	fs := NewPluginFileSystem()
	require.NotNil(t, fs)
}

func TestPluginFileSystem_RemoveAll(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "plugin-data")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "file.js"), []byte("code"), 0644))

	fs := NewPluginFileSystem()
	require.NoError(t, fs.RemoveAll(sub))

	_, err := os.Stat(sub)
	assert.True(t, os.IsNotExist(err))
}

func TestPluginFileSystem_MkdirTemp(t *testing.T) {
	fs := NewPluginFileSystem()
	dir, err := fs.MkdirTemp("", "evey-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	info, statErr := os.Stat(dir)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestPluginFileSystem_CopySDKStubs_CSS(t *testing.T) {
	fs := NewPluginFileSystem()
	// CSS should be a no-op
	err := fs.CopySDKStubs("css", t.TempDir())
	require.NoError(t, err)
}

func TestPluginFileSystem_CopySDKStubs_MissingSDKDir(t *testing.T) {
	fs := NewPluginFileSystem()
	// SDK dir /app/sdk/nodejs won't exist in test env — should return nil (no-op)
	err := fs.CopySDKStubs("nodejs", t.TempDir())
	require.NoError(t, err)
}

func TestPluginFileSystem_CopySDKStubs_NodeAlias(t *testing.T) {
	fs := NewPluginFileSystem()
	// "node" should be normalized to "nodejs" — SDK dir won't exist, no-op
	err := fs.CopySDKStubs("node", t.TempDir())
	require.NoError(t, err)
}

// TestSDKStubs_InSyncWithSourceProto guards the plugin SDK contract:
// CopySDKStubs overwrites a plugin's bundled plugin_api.proto with the
// server-shipped sdk/ copy at every install, so a stale sdk/ silently strips
// newer host RPCs from installed plugins (prod 2026-07-11: "host.Fetch is not
// a function" — the sdk/nodejs proto predated the Fetch broker). Run
// `make plugin-sdk` after touching proto/plugin/v1/plugin_api.proto.
func TestSDKStubs_InSyncWithSourceProto(t *testing.T) {
	const repoRoot = "../../../.."
	source, err := os.ReadFile(filepath.Join(repoRoot, "proto/plugin/v1/plugin_api.proto"))
	require.NoError(t, err)

	for _, lang := range []string{"nodejs", "php", "ruby", "java", "swift", "dart"} {
		got, readErr := os.ReadFile(filepath.Join(repoRoot, "sdk", lang, "plugin_api.proto"))
		require.NoError(t, readErr, "sdk/%s/plugin_api.proto missing", lang)
		assert.Equal(t, string(source), string(got),
			"sdk/%s/plugin_api.proto differs from proto/plugin/v1/plugin_api.proto — run `make plugin-sdk`", lang)
	}

	// Python ships generated stubs, not the proto: check the service surface
	// includes every RPC the source proto declares.
	pyGRPC, err := os.ReadFile(filepath.Join(repoRoot, "sdk/python/plugin_api_pb2_grpc.py"))
	require.NoError(t, err)
	for line := range strings.SplitSeq(string(source), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "rpc ") {
			continue
		}
		rpcName := strings.TrimSpace(strings.SplitN(strings.TrimPrefix(line, "rpc "), "(", 2)[0])
		assert.Contains(t, string(pyGRPC), rpcName,
			"sdk/python stubs are missing RPC %s — run `make plugin-sdk`", rpcName)
	}
}

func TestPluginFileSystem_ChownPluginDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.js"), []byte("x"), 0644))

	fs := NewPluginFileSystem()
	// Should not panic even if chown fails (non-root)
	fs.ChownPluginDir(dir)
}
