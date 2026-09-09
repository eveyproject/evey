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
	// SDK dir /app/sdk/nodejs won't exist in test env - should return nil (no-op)
	err := fs.CopySDKStubs("nodejs", t.TempDir())
	require.NoError(t, err)
}

func TestPluginFileSystem_CopySDKStubs_NodeAlias(t *testing.T) {
	fs := NewPluginFileSystem()
	// "node" should be normalized to "nodejs" - SDK dir won't exist, no-op
	err := fs.CopySDKStubs("node", t.TempDir())
	require.NoError(t, err)
}

func TestIsSafePluginLanguage(t *testing.T) {
	tests := []struct {
		name string
		lang string
		want bool
	}{
		{"plain lowercase ident", "nodejs", true},
		{"plain ident with digits", "go1", true},
		{"empty", "", false},
		{"dot", ".", false},
		{"dotdot", "..", false},
		{"parent traversal", "../../etc", false},
		{"parent traversal no leading slash", "..", false},
		{"nested traversal", "foo/../../etc", false},
		{"forward slash absolute", "/etc/passwd", false},
		{"backslash traversal", `..\..\etc`, false},
		{"embedded forward slash", "a/b", false},
		{"embedded backslash", `a\b`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSafePluginLanguage(tt.lang), "language %q", tt.lang)
		})
	}
}

// TestPluginFileSystem_CopySDKStubs_PathTraversal_Rejected guards against a
// plugin manifest smuggling ".." (or a raw separator) into the language
// field: filepath.Join(sdkBasePath, lang) would otherwise resolve outside
// sdkBasePath ("/app/sdk/../../etc" -> "/etc") and CopySDKStubs would happily
// copy real host files into a plugin's installDir. It must instead behave
// like the existing "unknown language" branch: no error, nothing copied.
func TestPluginFileSystem_CopySDKStubs_PathTraversal_Rejected(t *testing.T) {
	fs := NewPluginFileSystem()

	for _, lang := range []string{"../../etc", "..\\..\\etc", "foo/../../etc", "/etc"} {
		installDir := t.TempDir()

		err := fs.CopySDKStubs(lang, installDir)
		require.NoError(t, err, "language %q", lang)

		entries, readErr := os.ReadDir(installDir)
		require.NoError(t, readErr)
		assert.Empty(t, entries, "language %q must not copy anything into installDir", lang)
	}
}

// TestSDKStubs_InSyncWithSourceProto guards the plugin SDK contract:
// CopySDKStubs overwrites a plugin's bundled plugin_api.proto with the
// server-shipped sdk/ copy at every install, so a stale sdk/ silently strips
// newer host RPCs from installed plugins (prod 2026-07-11: "host.Fetch is not
// a function" - the sdk/nodejs proto predated the Fetch broker). Run
// `make plugin-sdk` after touching proto/plugin/v1/plugin_api.proto.
func TestSDKStubs_InSyncWithSourceProto(t *testing.T) {
	const repoRoot = "../../../.."
	source, err := os.ReadFile(filepath.Join(repoRoot, "proto/plugin/v1/plugin_api.proto"))
	require.NoError(t, err)

	for _, lang := range []string{"nodejs", "php", "ruby", "java", "swift", "dart"} {
		got, readErr := os.ReadFile(filepath.Join(repoRoot, "sdk", lang, "plugin_api.proto"))
		require.NoError(t, readErr, "sdk/%s/plugin_api.proto missing", lang)
		assert.Equal(t, string(source), string(got),
			"sdk/%s/plugin_api.proto differs from proto/plugin/v1/plugin_api.proto - run `make plugin-sdk`", lang)
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
			"sdk/python stubs are missing RPC %s - run `make plugin-sdk`", rpcName)
	}
}

func TestPluginFileSystem_ChownPluginDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.js"), []byte("x"), 0644))

	fs := NewPluginFileSystem()
	// Should not panic even if chown fails (non-root)
	fs.ChownPluginDir(dir)
}
