package filesystem_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheStore_GetCacheInfo_Empty(t *testing.T) {
	dir := t.TempDir()
	store := filesystem.NewCacheStore(dir)

	info, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.TotalSize)
	assert.Equal(t, 0, info.FileCount)
	assert.Empty(t, info.Files)
}

func TestCacheStore_GetCacheInfo_NonExistent(t *testing.T) {
	store := filesystem.NewCacheStore("/tmp/evey-cache-nonexistent-dir-test")

	info, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.TotalSize)
}

func TestCacheStore_GetCacheInfo_FlatFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.ts"), make([]byte, 1024), 0644)
	os.WriteFile(filepath.Join(dir, "b.ts"), make([]byte, 2048), 0644)

	store := filesystem.NewCacheStore(dir)
	info, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(3072), info.TotalSize)
	assert.Equal(t, 2, info.FileCount)
}

func TestCacheStore_GetCacheInfo_Subdirectories(t *testing.T) {
	dir := t.TempDir()
	// Simulate HLS session dir
	hlsDir := filepath.Join(dir, "hls")
	sessionDir := filepath.Join(hlsDir, "media-123")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))
	os.WriteFile(filepath.Join(sessionDir, "master.m3u8"), make([]byte, 256), 0644)
	os.WriteFile(filepath.Join(sessionDir, "stream_0_001.ts"), make([]byte, 4096), 0644)
	os.WriteFile(filepath.Join(sessionDir, "stream_0_002.ts"), make([]byte, 4096), 0644)

	// Also a flat file
	os.WriteFile(filepath.Join(dir, "job1.m3u8"), make([]byte, 100), 0644)

	store := filesystem.NewCacheStore(dir)
	info, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(256+4096+4096+100), info.TotalSize)
	assert.Equal(t, 2, info.FileCount) // "media-123" session + "job1.m3u8"

	nameMap := map[string]int64{}
	for _, f := range info.Files {
		nameMap[f.Name] = f.Size
	}
	assert.Equal(t, int64(256+4096+4096), nameMap["media-123"])
	assert.Equal(t, int64(100), nameMap["job1.m3u8"])
}

func TestCacheStore_GetCacheInfo_EmptySubdir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "empty-dir"), 0755)

	store := filesystem.NewCacheStore(dir)
	info, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, info.FileCount)
}

func TestCacheStore_ClearCache(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.ts"), make([]byte, 1024), 0644)
	subDir := filepath.Join(dir, "hls", "session1")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "seg.ts"), make([]byte, 2048), 0644)

	store := filesystem.NewCacheStore(dir)
	require.NoError(t, store.ClearCache(context.Background()))

	info, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, info.FileCount)
	assert.Equal(t, int64(0), info.TotalSize)
}

func TestCacheStore_ClearCache_NonExistent(t *testing.T) {
	store := filesystem.NewCacheStore("/tmp/evey-cache-nonexistent-clear-test")
	assert.NoError(t, store.ClearCache(context.Background()))
}

func TestCacheStore_DeleteFile_Flat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.ts"), make([]byte, 1024), 0644)
	os.WriteFile(filepath.Join(dir, "b.ts"), make([]byte, 2048), 0644)

	store := filesystem.NewCacheStore(dir)
	require.NoError(t, store.DeleteFile(context.Background(), "a.ts"))

	info, _ := store.GetCacheInfo(context.Background())
	assert.Equal(t, 1, info.FileCount)
}

func TestCacheStore_DeleteFile_Directory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "session1")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "seg.ts"), make([]byte, 2048), 0644)

	store := filesystem.NewCacheStore(dir)
	require.NoError(t, store.DeleteFile(context.Background(), "session1"))

	_, err := os.Stat(subDir)
	assert.True(t, os.IsNotExist(err))
}

func TestCacheStore_DeleteFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := filesystem.NewCacheStore(dir)
	err := store.DeleteFile(context.Background(), "missing.ts")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCacheStore_DeleteFile_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	store := filesystem.NewCacheStore(dir)

	require.Error(t, store.DeleteFile(context.Background(), "../etc/passwd"))
	require.Error(t, store.DeleteFile(context.Background(), "foo/bar"))
	require.Error(t, store.DeleteFile(context.Background(), "foo\\bar"))
}
