package pluginrunner

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createZipWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return zipPath
}

func TestZipExtract(t *testing.T) {
	zipPath := createZipWithFiles(t, map[string]string{
		"plugin.yaml": "name: test\nversion: 1.0",
		"main.go":     "package main",
		"lib/util.go": "package lib",
	})
	dest := t.TempDir()
	installer := NewZipInstaller()

	err := installer.Extract(zipPath, dest)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dest, "plugin.yaml"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dest, "main.go"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dest, "lib", "util.go"))
	require.NoError(t, err)
}

func TestZipExtractRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	fw, err := w.Create("../../etc/passwd")
	require.NoError(t, err)
	_, err = fw.Write([]byte("root:x:0:0"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	dest := t.TempDir()
	installer := NewZipInstaller()

	err = installer.Extract(zipPath, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestZipExtractCreatesDestDir(t *testing.T) {
	zipPath := createZipWithFiles(t, map[string]string{
		"hello.txt": "world",
	})
	dest := filepath.Join(t.TempDir(), "newdir", "subdir")
	installer := NewZipInstaller()

	err := installer.Extract(zipPath, dest)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dest, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "world", string(data))
}

func TestZipExtractInvalidZip(t *testing.T) {
	installer := NewZipInstaller()
	err := installer.Extract("/nonexistent.zip", t.TempDir())
	assert.Error(t, err)
}
