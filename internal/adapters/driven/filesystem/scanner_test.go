package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScan_FindsVideoFiles(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"movie.mkv", "show.mp4", "clip.avi",
		"document.pdf", "image.jpg", "readme.txt",
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644))
	}

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, 3, len(found))

	names := make(map[string]bool)
	for _, f := range found {
		names[filepath.Base(f.Path)] = true
	}
	assert.True(t, names["movie.mkv"])
	assert.True(t, names["show.mp4"])
	assert.True(t, names["clip.avi"])
}

func TestScan_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".hidden"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden", "secret.mkv"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "visible.mkv"), []byte("x"), 0644))

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, 1, len(found))
	assert.Equal(t, "visible.mkv", filepath.Base(found[0].Path))
}

func TestScan_SkipsHiddenFiles(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden.mkv"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "normal.mkv"), []byte("x"), 0644))

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, 1, len(found))
	assert.Equal(t, "normal.mkv", filepath.Base(found[0].Path))
}

func TestScan_RecursiveSubdirs(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "Season 1"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "Season 2"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Season 1", "ep1.mkv"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Season 2", "ep1.mkv"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "movie.mp4"), []byte("x"), 0644))

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, 3, len(found))
}

func TestScan_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestScan_AllVideoExtensions(t *testing.T) {
	dir := t.TempDir()

	exts := []string{".mkv", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts", ".m2ts", ".mpg", ".mpeg", ".3gp", ".ogv", ".vob"}
	for _, ext := range exts {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "video"+ext), []byte("x"), 0644))
	}

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, len(exts), len(found))
}

func TestScan_CaseInsensitiveExtension(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "movie.MKV"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "movie.Mp4"), []byte("x"), 0644))

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, 2, len(found))
}

func TestScan_NonexistentDir(t *testing.T) {
	s := NewScanner()
	found, err := s.Scan(context.Background(), "/nonexistent/path/12345")
	assert.Error(t, err)
	assert.Empty(t, found)
}

func TestScan_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("x"), 0644))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := NewScanner()
	_, err := s.Scan(ctx, dir)
	assert.Error(t, err)
}

func TestScan_FileSize(t *testing.T) {
	dir := t.TempDir()

	content := make([]byte, 1024)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "movie.mkv"), content, 0644))

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, 1, len(found))
	assert.Equal(t, int64(1024), found[0].Size)
}
