package filesystem_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	"github.com/evey-project/evey/internal/domain/transcoding"
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
	require.ErrorIs(t, err, transcoding.ErrCacheFileNotFound)
}

func TestCacheStore_DeleteFile_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	store := filesystem.NewCacheStore(dir)

	require.ErrorIs(t, store.DeleteFile(context.Background(), "../etc/passwd"), transcoding.ErrCacheFilenameInvalid)
	require.Error(t, store.DeleteFile(context.Background(), "foo/bar"))
	require.Error(t, store.DeleteFile(context.Background(), "foo\\bar"))
}

// The admin cache page triggers a full recursive walk of the transcode cache.
// On the weak-NAS target that walk is the expensive part, so a repeat open
// inside the TTL must reuse the previous answer instead of re-walking.
func TestCacheStore_GetCacheInfo_MemoizedWithinTTL(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.ts"), make([]byte, 1024), 0644))
	store := filesystem.NewCacheStore(dir)

	first, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1024), first.TotalSize)

	// A file appearing behind our back must NOT be picked up while the memo is
	// warm - that is what proves the walk was skipped.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.ts"), make([]byte, 4096), 0644))
	second, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1024), second.TotalSize, "second call inside the TTL should reuse the memo")
}

// Invalidation is not optional: an operator who clears the cache and still sees
// the old size would reasonably conclude the button is broken.
func TestCacheStore_ClearCache_InvalidatesMemo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.ts"), make([]byte, 1024), 0644))
	store := filesystem.NewCacheStore(dir)

	warm, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1024), warm.TotalSize)

	require.NoError(t, store.ClearCache(context.Background()))
	after, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), after.TotalSize)
	assert.Equal(t, 0, after.FileCount)
}

// Same reasoning for a single-file delete: the page that offers the button is
// the page that shows the total.
func TestCacheStore_DeleteFile_InvalidatesMemo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.ts"), make([]byte, 1024), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.ts"), make([]byte, 2048), 0644))
	store := filesystem.NewCacheStore(dir)

	warm, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(3072), warm.TotalSize)

	require.NoError(t, store.DeleteFile(context.Background(), "b.ts"))
	after, err := store.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1024), after.TotalSize)
}
