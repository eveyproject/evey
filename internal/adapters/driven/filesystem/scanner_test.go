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
	assert.Len(t, found, 3)

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
	assert.Len(t, found, 1)
	assert.Equal(t, "visible.mkv", filepath.Base(found[0].Path))
}

func TestScan_SkipsHiddenFiles(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden.mkv"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "normal.mkv"), []byte("x"), 0644))

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Len(t, found, 1)
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
	assert.Len(t, found, 3)
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
		// .ts and .m2ts now require the MPEG-TS sync byte (0x47) at offset 0.
		content := []byte("x")
		if ext == ".ts" || ext == ".m2ts" {
			content = []byte{0x47}
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "video"+ext), content, 0644))
	}

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Len(t, found, len(exts))
}

func TestScan_RejectsTextFileWithTsExtension(t *testing.T) {
	dir := t.TempDir()

	// TypeScript source masquerading as a transport stream.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "front-bridge.ts"), []byte("export interface FrontModule {}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.m2ts"), []byte("import x from 'y'\n"), 0644))
	// Real TS / M2TS streams pass through.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.ts"), []byte{0x47, 0x00, 0x00, 0x00, 0x00}, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.m2ts"), []byte{0x00, 0x00, 0x00, 0x00, 0x47}, 0644))

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, found, 2)

	names := map[string]bool{}
	for _, f := range found {
		names[filepath.Base(f.Path)] = true
	}
	assert.True(t, names["real.ts"])
	assert.True(t, names["real.m2ts"])
}

func TestScan_CaseInsensitiveExtension(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "movie.MKV"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "movie.Mp4"), []byte("x"), 0644))

	s := NewScanner()
	found, err := s.Scan(context.Background(), dir)
	require.NoError(t, err)
	assert.Len(t, found, 2)
}

func TestScan_NonexistentDir(t *testing.T) {
	s := NewScanner()
	found, err := s.Scan(context.Background(), "/nonexistent/path/12345")
	require.Error(t, err)
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
	require.Len(t, found, 1)
	assert.Equal(t, int64(1024), found[0].Size)
}

func TestParseYear_FileThenFolder(t *testing.T) {
	s := NewScanner()
	require.NotNil(t, s.ParseYear("/media/Caminandes Llama Drama (2013).mkv"))
	assert.Equal(t, 2013, *s.ParseYear("/media/Caminandes Llama Drama (2013).mkv"))
	require.NotNil(t, s.ParseYear("/media/Dune (2021)/movie.mkv"))
	assert.Equal(t, 2021, *s.ParseYear("/media/Dune (2021)/movie.mkv"))
	assert.Nil(t, s.ParseYear("/media/untitled/movie.mkv"))
}
