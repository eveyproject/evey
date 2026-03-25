package filesystem

import (
	"os"
	"path/filepath"
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

func TestPluginFileSystem_ChownPluginDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.js"), []byte("x"), 0644))

	fs := NewPluginFileSystem()
	// Should not panic even if chown fails (non-root)
	fs.ChownPluginDir(dir)
}
