package filesystem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

type failingReader struct {
	n   int
	err error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, r.err
	}
	n := min(len(p), r.n)
	for i := range n {
		p[i] = 'x'
	}
	r.n -= n
	return n, nil
}

// An archive is a backup only once it is whole on disk: a dump that stops
// halfway (or a process that dies) must not leave a truncated file under the
// final name, where a restore would take it for a valid backup, nor a
// temporary one that nothing lists or purges.
func TestBackupStore_Create_FailedDumpLeavesNoArchive(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)
	m := &backup.BackupManifest{ID: "bk-cut", CreatedAt: time.Now(), Version: "1.0", Tables: []string{"t"}}

	err := store.Create(context.Background(), m, &failingReader{n: 4096, err: errors.New("pg_dump died")})
	require.Error(t, err)

	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	for _, e := range entries {
		t.Errorf("a failed backup left %q behind", e.Name())
	}
	list, listErr := store.List(context.Background())
	require.NoError(t, listErr)
	assert.Empty(t, list)
}

// A finished backup is the final name alone: the temporary name it was
// written under is gone.
func TestBackupStore_Create_LeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	store := NewBackupStore(dir)
	m := &backup.BackupManifest{ID: "bk-whole", CreatedAt: time.Now(), Version: "1.0", Tables: []string{"t"}}
	require.NoError(t, store.Create(context.Background(), m, bytes.NewReader([]byte("dump"))))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{"bk-whole.tar.gz", "bk-whole.json"}, names)
}

// The sidecar is what List reads, so it names the archive: it must never be
// published without the archive (a listed backup nothing can restore), and
// an archive whose sidecar cannot be published must not stay behind as a
// complete, invisible file nothing lists or purges.
func TestBackupStore_Create_PublishesArchiveAndSidecarTogether(t *testing.T) {
	t.Run("sidecar cannot be published", func(t *testing.T) {
		dir := t.TempDir()
		store := NewBackupStore(dir)
		// A folder squatting the sidecar's name makes its rename fail.
		require.NoError(t, os.Mkdir(filepath.Join(dir, "bk-side.json"), 0o700))
		m := &backup.BackupManifest{ID: "bk-side", CreatedAt: time.Now(), Version: "1.0", Tables: []string{"t"}}

		require.Error(t, store.Create(context.Background(), m, bytes.NewReader([]byte("dump"))))

		_, err := os.Stat(filepath.Join(dir, "bk-side.tar.gz"))
		assert.True(t, os.IsNotExist(err), "an archive without a sidecar must not stay behind")
		entries, readErr := os.ReadDir(dir)
		require.NoError(t, readErr)
		for _, e := range entries {
			assert.False(t, strings.HasSuffix(e.Name(), ".tmp"), "temporary file left: %s", e.Name())
		}
	})
	t.Run("archive cannot be published", func(t *testing.T) {
		dir := t.TempDir()
		store := NewBackupStore(dir)
		require.NoError(t, os.Mkdir(filepath.Join(dir, "bk-arch.tar.gz"), 0o700))
		m := &backup.BackupManifest{ID: "bk-arch", CreatedAt: time.Now(), Version: "1.0", Tables: []string{"t"}}

		require.Error(t, store.Create(context.Background(), m, bytes.NewReader([]byte("dump"))))

		_, err := os.Stat(filepath.Join(dir, "bk-arch.json"))
		assert.True(t, os.IsNotExist(err), "a sidecar must never exist without its archive")
		list, listErr := store.List(context.Background())
		require.NoError(t, listErr)
		assert.Empty(t, list)
	})
}
