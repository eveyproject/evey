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
	assert.ErrorContains(t, err, "open")
}

func TestZipExtract_FileAtMaxSize_Passes(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "big.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	header := &zip.FileHeader{Name: "big.txt", Method: zip.Store}
	fw, err := w.CreateHeader(header)
	require.NoError(t, err)
	// Write exactly maxZipFileSize bytes (100MB)
	data := make([]byte, maxZipFileSize)
	for i := range data {
		data[i] = 'A'
	}
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	dest := t.TempDir()
	installer := NewZipInstaller()
	err = installer.Extract(zipPath, dest)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dest, "big.txt"))
	require.NoError(t, err)
	assert.Equal(t, int64(maxZipFileSize), info.Size())
}

func TestZipExtract_FileOverMaxSize_Fails(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "toobig.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	header := &zip.FileHeader{Name: "toobig.txt", Method: zip.Store}
	fw, err := w.CreateHeader(header)
	require.NoError(t, err)
	data := make([]byte, maxZipFileSize+1)
	for i := range data {
		data[i] = 'B'
	}
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	dest := t.TempDir()
	installer := NewZipInstaller()
	err = installer.Extract(zipPath, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds max size")
}

func TestZipExtract_TooManyFiles_Fails(t *testing.T) {
	files := make(map[string]string, maxZipFiles+1)
	for i := 0; i <= maxZipFiles; i++ {
		files[filepath.Join("bulk", "f"+fmt_itoa(i)+".txt")] = "x"
	}
	zipPath := createZipWithFiles(t, files)
	dest := t.TempDir()
	installer := NewZipInstaller()
	err := installer.Extract(zipPath, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max files")
}

func TestZipExtract_TotalBytesExceeded_Fails(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "fat.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	perFile := 60 * 1024 * 1024 // 60 MB each × 9 = 540 MB > 500 MB cap
	payload := make([]byte, perFile)
	for i := range payload {
		payload[i] = 'C'
	}
	for i := 0; i < 9; i++ {
		fw, err := w.CreateHeader(&zip.FileHeader{Name: "file" + fmt_itoa(i) + ".bin", Method: zip.Store})
		require.NoError(t, err)
		_, err = fw.Write(payload)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	dest := t.TempDir()
	installer := NewZipInstaller()
	err = installer.Extract(zipPath, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max total size")
}

func TestZipExtract_ZipBomb_Fails(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bomb.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	// 1 MB of zeros — high compression ratio
	payload := make([]byte, 1*1024*1024)
	fw, err := w.CreateHeader(&zip.FileHeader{Name: "zeros.bin", Method: zip.Deflate})
	require.NoError(t, err)
	_, err = fw.Write(payload)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	dest := t.TempDir()
	installer := NewZipInstaller()
	err = installer.Extract(zipPath, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compression ratio")
}

func TestZipExtract_StripsSetuid(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "suid.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	// 0o4755 = setuid + rwxr-xr-x. Expect extract to strip setuid.
	header := &zip.FileHeader{Name: "suid.sh", Method: zip.Store}
	header.SetMode(0o4755)
	fw, err := w.CreateHeader(header)
	require.NoError(t, err)
	_, err = fw.Write([]byte("#!/bin/sh\n"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	dest := t.TempDir()
	installer := NewZipInstaller()
	require.NoError(t, installer.Extract(zipPath, dest))

	info, err := os.Stat(filepath.Join(dest, "suid.sh"))
	require.NoError(t, err)
	// setuid bit (0o4000) must NOT be set.
	assert.Zero(t, info.Mode()&os.ModeSetuid, "setuid must be stripped")
}

func TestExtractZip_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "symlink.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)

	w := zip.NewWriter(f)
	// ExternalAttrs high 16 bits hold the Unix mode word.
	// Type bits 0120000 = symlink; perms 0644 → ExternalAttrs = 0o120644 << 16.
	header := &zip.FileHeader{Name: "evil-link", Method: zip.Store}
	header.ExternalAttrs = 0o120644 << 16
	fw, err := w.CreateHeader(header)
	require.NoError(t, err)
	_, err = fw.Write([]byte("/etc/passwd"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	dest := t.TempDir()
	installer := NewZipInstaller()
	err = installer.Extract(zipPath, dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlinks not allowed")
}

// fmt_itoa is a small helper so tests don't pull fmt for one %d.
func fmt_itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	buf := make([]byte, 0, 12)
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func TestZipExtract_PreservesContent(t *testing.T) {
	content := "line1\nline2\nline3\nspecial chars: éàü 日本語\n"
	zipPath := createZipWithFiles(t, map[string]string{
		"data.txt":     content,
		"sub/inner.py": "import os\nprint('hello')\n",
	})
	dest := t.TempDir()
	installer := NewZipInstaller()

	err := installer.Extract(zipPath, dest)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(dest, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, content, string(got))

	got2, err := os.ReadFile(filepath.Join(dest, "sub", "inner.py"))
	require.NoError(t, err)
	assert.Equal(t, "import os\nprint('hello')\n", string(got2))
}
