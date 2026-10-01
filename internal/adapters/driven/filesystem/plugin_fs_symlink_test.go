package filesystem

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The plugin owns its install dir (symlinks included) and CopySDKStubs runs
// at every Start with the server's rights: a stub name planted as a link
// must be replaced, never written through.
func TestCopySDKStubs_DoesNotWriteThroughPlantedSymlink(t *testing.T) {
	sdk := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(sdk, "python"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sdk, "python", "evey_sdk.py"), []byte("STUB-CONTENT"), 0o644))
	prev := sdkBasePath
	sdkBasePath = sdk
	t.Cleanup(func() { sdkBasePath = prev })

	victim := filepath.Join(t.TempDir(), "authorized_keys")
	require.NoError(t, os.WriteFile(victim, []byte("ORIGINAL"), 0o600))
	installDir := t.TempDir()
	dst := filepath.Join(installDir, "evey_sdk.py")
	require.NoError(t, os.Symlink(victim, dst))

	require.NoError(t, NewPluginFileSystem().CopySDKStubs("python", installDir))

	got, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(got), "the link target must be untouched")
	fi, err := os.Lstat(dst)
	require.NoError(t, err)
	assert.True(t, fi.Mode().IsRegular(), "the stub must replace the link with a regular file")
	got, err = os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "STUB-CONTENT", string(got))
}

func TestCopySDKStubs_RefusesSymlinkedInstallDir(t *testing.T) {
	sdk := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(sdk, "python"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sdk, "python", "evey_sdk.py"), []byte("STUB-CONTENT"), 0o644))
	prev := sdkBasePath
	sdkBasePath = sdk
	t.Cleanup(func() { sdkBasePath = prev })

	outside := t.TempDir()
	link := filepath.Join(t.TempDir(), "plugin")
	require.NoError(t, os.Symlink(outside, link))

	require.Error(t, NewPluginFileSystem().CopySDKStubs("python", link))
	_, err := os.Stat(filepath.Join(outside, "evey_sdk.py"))
	assert.True(t, os.IsNotExist(err), "nothing may be written through a linked install dir")
}

// ChownPluginDir must change the link itself, not its target. Provable as a
// non-root user: chown of a root-owned target fails, lchown of our own link
// to our own uid succeeds.
func TestChownPluginDir_DoesNotFollowSymlinks(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("as root chown always succeeds, the failure signal is gone")
	}
	target := "/etc/hostname"
	if fi, err := os.Stat(target); err != nil || fi.Sys() == nil {
		t.Skip("no root-owned file to point at")
	}
	prevUID := pluginUID
	pluginUID = os.Getuid()
	t.Cleanup(func() { pluginUID = prevUID })

	dir := t.TempDir()
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "evil")))

	var buf bytes.Buffer
	prevLog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	NewPluginFileSystem().ChownPluginDir(dir)
	assert.NotContains(t, buf.String(), "chown plugin dir", "the walk must not chown through the link")
}
