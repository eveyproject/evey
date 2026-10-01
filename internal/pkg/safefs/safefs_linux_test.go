//go:build linux

package safefs

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forEachResolver runs fn with openat2 and with the forced component walk.
func forEachResolver(t *testing.T, fn func(t *testing.T)) {
	t.Run("openat2", func(t *testing.T) {
		if noOpenat2.Load() {
			t.Skip("openat2 unavailable on this kernel")
		}
		fn(t)
		assert.False(t, noOpenat2.Load(), "openat2 fell back to the walk mid-test")
	})
	t.Run("walk", func(t *testing.T) {
		prev := noOpenat2.Load()
		noOpenat2.Store(true)
		t.Cleanup(func() { noOpenat2.Store(prev) })
		fn(t)
	})
}

func seedTree(t *testing.T) (root, outside string) {
	t.Helper()
	root, outside = t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "p", "dist", "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "p", "dist", "ok.js"), []byte("OK"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "p", "dist", "sub", "deep.js"), []byte("DEEP"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("SECRET"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "p", "dist", "leak.js")))
	require.NoError(t, os.Symlink("ok.js", filepath.Join(root, "p", "dist", "alias.js")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "p", "dist", "linkdir")))
	require.NoError(t, syscall.Mkfifo(filepath.Join(root, "p", "dist", "fifo"), 0o644))
	return root, outside
}

func TestReadFile_RegularFilesServed(t *testing.T) {
	root, _ := seedTree(t)
	forEachResolver(t, func(t *testing.T) {
		data, info, err := ReadFile(root, "p/dist/ok.js", 0)
		require.NoError(t, err)
		assert.Equal(t, "OK", string(data))
		assert.EqualValues(t, 2, info.Size())
		data, _, err = ReadFile(root, "p/dist/sub/deep.js", 0)
		require.NoError(t, err)
		assert.Equal(t, "DEEP", string(data))
		info, err = StatRegular(root, "p/dist/sub/deep.js")
		require.NoError(t, err)
		assert.EqualValues(t, 4, info.Size())
	})
}

func TestReadFile_RefusesLinksAndSpecialFiles(t *testing.T) {
	root, _ := seedTree(t)
	forEachResolver(t, func(t *testing.T) {
		for _, rel := range []string{"p/dist/leak.js", "p/dist/alias.js", "p/dist/linkdir/secret", "p/dist/fifo", "p/dist/sub", "p/dist/missing"} {
			done := make(chan struct{})
			go func() {
				defer close(done)
				data, _, err := ReadFile(root, rel, 0)
				assert.Error(t, err, rel)
				assert.Empty(t, data, rel)
				_, err = StatRegular(root, rel)
				assert.Error(t, err, rel)
			}()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatalf("%s blocked", rel)
			}
		}
	})
}

func TestReadFile_RejectsInvalidRel(t *testing.T) {
	root, _ := seedTree(t)
	for _, rel := range []string{"", "/etc/passwd", "p/../p/dist/ok.js", "p/./dist/ok.js", "p//dist/ok.js", ".."} {
		_, _, err := ReadFile(root, rel, 0)
		assert.ErrorIs(t, err, ErrInvalidPath, rel)
	}
}

func TestReadFile_SizeLimit(t *testing.T) {
	root, _ := seedTree(t)
	_, info, err := ReadFile(root, "p/dist/sub/deep.js", 3)
	require.ErrorIs(t, err, ErrTooLarge)
	require.NotNil(t, info)
	data, _, err := ReadFile(root, "p/dist/sub/deep.js", 4)
	require.NoError(t, err)
	assert.Equal(t, "DEEP", string(data))
}

func TestWriteFileReplace_ReplacesPlantedLink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("ORIGINAL"), 0o600))
	require.NoError(t, os.Symlink(victim, filepath.Join(dir, "stub.py")))

	require.NoError(t, WriteFileReplace(dir, "stub.py", []byte("STUB"), 0o644))

	got, err := os.ReadFile(victim)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(got))
	fi, err := os.Lstat(filepath.Join(dir, "stub.py"))
	require.NoError(t, err)
	assert.True(t, fi.Mode().IsRegular())
	got, err = os.ReadFile(filepath.Join(dir, "stub.py"))
	require.NoError(t, err)
	assert.Equal(t, "STUB", string(got))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temp file left behind")
}

func TestWriteFileReplace_RefusesLinkedDirAndBadName(t *testing.T) {
	outside := t.TempDir()
	link := filepath.Join(t.TempDir(), "install")
	require.NoError(t, os.Symlink(outside, link))
	err := WriteFileReplace(link, "stub.py", []byte("STUB"), 0o644)
	assert.True(t, errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.ENOTDIR), "got %v", err)
	_, statErr := os.Stat(filepath.Join(outside, "stub.py"))
	assert.True(t, os.IsNotExist(statErr))
	assert.ErrorIs(t, WriteFileReplace(t.TempDir(), "../x", nil, 0o644), ErrInvalidPath)
}
