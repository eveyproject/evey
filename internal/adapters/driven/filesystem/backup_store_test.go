package filesystem

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/backup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBackupStore(t *testing.T) {
	s := NewBackupStore("/tmp/test-backups")
	require.NotNil(t, s)
	assert.Equal(t, "/tmp/test-backups", s.dir)
}

func TestBackupStore_Create_And_List(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	m := &backup.BackupManifest{
		ID:        "bk-001",
		CreatedAt: time.Now(),
		Version:   "1.0",
		Tables:    []string{"users", "media"},
	}
	data := bytes.NewReader([]byte("CREATE TABLE users; INSERT INTO users VALUES(1);"))

	err := store.Create(context.Background(), m, data)
	require.NoError(t, err)
	assert.Positive(t, m.Size)

	// Verify archive + sidecar exist
	_, statErr := os.Stat(filepath.Join(dir, "bk-001.tar.gz"))
	require.NoError(t, statErr)
	_, statErr = os.Stat(filepath.Join(dir, "bk-001.json"))
	require.NoError(t, statErr)

	// List
	list, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "bk-001", list[0].ID)
	assert.Positive(t, list[0].Size)
}

func TestBackupStore_List_Empty(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	list, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestBackupStore_List_NonExistent(t *testing.T) {
	store := NewBackupStore("/tmp/evey-backup-nonexistent-test-xyz")
	list, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Nil(t, list)
}

func TestBackupStore_Get(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	m := &backup.BackupManifest{
		ID:        "bk-002",
		CreatedAt: time.Now(),
		Version:   "1.0",
		Tables:    []string{"users"},
	}
	require.NoError(t, store.Create(context.Background(), m, bytes.NewReader([]byte("dump data"))))

	got, rc, err := store.Get(context.Background(), "bk-002")
	require.NoError(t, err)
	require.NotNil(t, rc)
	defer rc.Close()
	assert.Equal(t, "bk-002", got.ID)
	assert.Positive(t, got.Size)
}

func TestBackupStore_Get_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	_, _, err := store.Get(context.Background(), "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestBackupStore_Get_InvalidID(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	tests := []string{"../etc/passwd", "foo/bar", "a\\b", "id..evil"}
	for _, id := range tests {
		_, _, err := store.Get(context.Background(), id)
		require.Error(t, err, "id=%s should be rejected", id)
	}
}

func TestBackupStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	m := &backup.BackupManifest{
		ID:        "bk-003",
		CreatedAt: time.Now(),
		Version:   "1.0",
		Tables:    []string{"users"},
	}
	require.NoError(t, store.Create(context.Background(), m, bytes.NewReader([]byte("data"))))

	err := store.Delete(context.Background(), "bk-003")
	require.NoError(t, err)

	// Files should be gone
	_, statErr := os.Stat(filepath.Join(dir, "bk-003.tar.gz"))
	assert.True(t, os.IsNotExist(statErr))
	_, statErr = os.Stat(filepath.Join(dir, "bk-003.json"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestBackupStore_Delete_InvalidID(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	err := store.Delete(context.Background(), "../escape")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestBackupStore_Delete_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	err := store.Delete(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	// sentinel: callers must be able to distinguish not-found from real I/O errors
	assert.ErrorIs(t, err, backup.ErrNotFound)
}

func TestBackupStore_ResolveArchivePath_TarGz(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	tarPath := filepath.Join(dir, "bk-1.tar.gz")
	require.NoError(t, os.WriteFile(tarPath, []byte("archive"), 0640))

	path, ok := store.resolveArchivePath("bk-1")
	assert.True(t, ok)
	assert.Equal(t, tarPath, path)
}

func TestBackupStore_ResolveArchivePath_LegacyGz(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	gzPath := filepath.Join(dir, "bk-legacy.gz")
	require.NoError(t, os.WriteFile(gzPath, []byte("archive"), 0640))

	path, ok := store.resolveArchivePath("bk-legacy")
	assert.True(t, ok)
	assert.Equal(t, gzPath, path)
}

func TestBackupStore_ResolveArchivePath_Missing(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	_, ok := store.resolveArchivePath("missing")
	assert.False(t, ok)
}

func TestBackupStore_Create_MultipleAndListOrder(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	now := time.Now()
	m1 := &backup.BackupManifest{ID: "bk-old", CreatedAt: now.Add(-time.Hour), Version: "1.0", Tables: []string{"t"}}
	m2 := &backup.BackupManifest{ID: "bk-new", CreatedAt: now, Version: "1.0", Tables: []string{"t"}}

	require.NoError(t, store.Create(context.Background(), m1, bytes.NewReader([]byte("d1"))))
	require.NoError(t, store.Create(context.Background(), m2, bytes.NewReader([]byte("d2"))))

	list, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "bk-new", list[0].ID, "newest should be first")
	assert.Equal(t, "bk-old", list[1].ID)
}

type pureReader struct {
	data []byte
	pos  int
}

func (r *pureReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestStreamTarEntry_NonSeeker(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)

	m := &backup.BackupManifest{
		ID:        "bk-pipe",
		CreatedAt: time.Now(),
		Version:   "1.0",
		Tables:    []string{"t"},
	}
	// pureReader implements io.Reader but NOT io.ReadSeeker
	r := &pureReader{data: []byte("piped dump data")}

	err := store.Create(context.Background(), m, r)
	require.NoError(t, err)

	got, rc, getErr := store.Get(context.Background(), "bk-pipe")
	require.NoError(t, getErr)
	defer rc.Close()
	assert.Equal(t, "bk-pipe", got.ID)
}
