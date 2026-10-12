package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubReindexer reports every file as un-indexed and records the repairs it is
// asked to run.
type stubReindexer struct {
	mu        sync.Mutex
	reindexed []string
}

func (s *stubReindexer) HasSeekIndex(context.Context, string) (bool, error) { return false, nil }
func (s *stubReindexer) HasBackup(context.Context, string) (bool, error)    { return false, nil }
func (s *stubReindexer) ReindexInPlace(_ context.Context, p string, _ mkv.ReindexOptions) (mkv.ReindexResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reindexed = append(s.reindexed, p)
	return mkv.ReindexResult{Method: mkv.ReindexMethodRewrite}, nil
}
func (s *stubReindexer) RestoreBackup(context.Context, string) error { return nil }
func (s *stubReindexer) DeleteBackup(context.Context, string) error  { return nil }
func (s *stubReindexer) SupportsInPlace() bool                       { return false }

type stubCatalog struct{ files []MediaRef }

func (c stubCatalog) ListMediaFiles(context.Context, string) ([]MediaRef, error) {
	return c.files, nil
}
func (c stubCatalog) GetMediaFile(_ context.Context, id string) (MediaRef, error) {
	for _, f := range c.files {
		if f.MediaID == id {
			return f, nil
		}
	}
	return MediaRef{}, os.ErrNotExist
}

func waitJobIdle(t *testing.T, svc *SeekIndexService) JobStatus {
	t.Helper()
	var st JobStatus
	require.Eventually(t, func() bool {
		st = svc.Status()
		return !st.Running
	}, 5*time.Second, 5*time.Millisecond, "seek-index job did not finish")
	return st
}

// stubShortVolume makes every batch look over-committed: the candidates' sum
// dwarfs the volume's free space. Restored by t.Cleanup.
func stubShortVolume(t *testing.T, root string) {
	t.Helper()
	restore := summarizeVolumes
	summarizeVolumes = func(items []SeekStatus) []SeekVolume {
		var sum, largest int64
		for _, it := range items {
			sum += it.SizeBytes
			if it.SizeBytes > largest {
				largest = it.SizeBytes
			}
		}
		return []SeekVolume{{
			Root:                root,
			FreeBytes:           1 << 30,
			MissingBytes:        sum + 40<<30,
			LargestMissingBytes: largest + 40<<30,
			MissingFiles:        len(items),
		}}
	}
	t.Cleanup(func() { summarizeVolumes = restore })
}

// A backup-keeping library reindex whose candidates cannot ALL be duplicated
// on their volume must refuse upfront - before touching a single file -
// instead of letting the .mkvgobak accumulation start failing files hours into
// the batch. The per-file guard inside the rewrite cannot see this coming: it
// reads the LIVE free space, which only shrinks as the batch progresses.
func TestReindexLibrary_BackupOvercommitRefusedUpfront(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	stubShortVolume(t, dir)

	reidx := &stubReindexer{}
	svc := NewSeekIndexService(reidx, stubCatalog{files: []MediaRef{{MediaID: "m1", Path: p}}}, nil)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", ReindexRunOptions{}))
	st := waitJobIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Contains(t, st.Last.Err, "free disk space", "the refusal names the space problem")
	assert.Contains(t, st.Last.Err, "without backups", "the refusal names the way out")
	assert.Contains(t, st.Last.Err, dir, "the refusal names the short volume")
	assert.Zero(t, st.Last.Done, "no file was touched")
	assert.Empty(t, reidx.reindexed)
	assert.Equal(t, 1, st.Last.Total, "the refused batch still reports its candidate count")
}

// The same over-committed batch without backups has nothing to accumulate: it
// must run. The per-file rewrite preflight (largest candidate as a temp copy)
// keeps the authority on transient space.
func TestReindexLibrary_SkipBackupBypassesOvercommitGuard(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	stubShortVolume(t, dir)

	reidx := &stubReindexer{}
	svc := NewSeekIndexService(reidx, stubCatalog{files: []MediaRef{{MediaID: "m1", Path: p}}}, nil)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", ReindexRunOptions{SkipBackup: true}))
	st := waitJobIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Empty(t, st.Last.Err)
	assert.Equal(t, []string{p}, reidx.reindexed)
}

// In-place mode never duplicates anything on disk: the guard must not fire.
func TestReindexLibrary_InPlaceBypassesOvercommitGuard(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	stubShortVolume(t, dir)

	reidx := &stubReindexer{}
	svc := NewSeekIndexService(reidx, stubCatalog{files: []MediaRef{{MediaID: "m1", Path: p}}}, nil)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", ReindexRunOptions{Mode: mkv.ModeInPlace}))
	st := waitJobIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Empty(t, st.Last.Err)
	assert.Equal(t, []string{p}, reidx.reindexed)
}

// When a scan is already cached for the scope, the over-commit refusal is
// SYNCHRONOUS: StartReindexLibrary returns the typed error (so the API layer
// can translate it with the per-volume numbers) and no job starts at all.
func TestReindexLibrary_BackupOvercommitRefusedSynchronouslyOnCachedScan(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	stubShortVolume(t, dir)

	reidx := &stubReindexer{}
	svc := NewSeekIndexService(reidx, stubCatalog{files: []MediaRef{{MediaID: "m1", Path: p}}}, nil)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitJobIdle(t, svc)

	err := svc.StartReindexLibrary(context.Background(), "", ReindexRunOptions{})
	require.Error(t, err)
	require.ErrorIs(t, err, mkv.ErrInsufficientSpace)
	var spaceErr *BackupSpaceError
	require.ErrorAs(t, err, &spaceErr)
	require.Len(t, spaceErr.Volumes, 1)
	assert.Contains(t, spaceErr.Detail(), dir, "the detail names the short volume")
	assert.False(t, svc.Status().Running, "no job was started")
	assert.Empty(t, reidx.reindexed)

	// The refusal must not consume the job slot: a skip-backup run starts fine.
	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", ReindexRunOptions{SkipBackup: true}))
	waitJobIdle(t, svc)
	assert.Equal(t, []string{p}, reidx.reindexed)
}

// A volume whose free space could not be probed reports FreeBytes 0: the guard
// must stay advisory there (the per-file preflight keeps the authority) and
// only flag volumes with a proven shortfall.
func TestBackupShortfalls(t *testing.T) {
	short := backupShortfalls([]SeekVolume{
		{Root: "/probe-failed", FreeBytes: 0, MissingBytes: 10 << 30, MissingFiles: 1},
		{Root: "/fits", FreeBytes: 100 << 30, MissingBytes: 10 << 30, MissingFiles: 2},
		{Root: "/short", FreeBytes: 1 << 30, MissingBytes: 40 << 30, MissingFiles: 3},
		{Root: "/empty", FreeBytes: 5 << 30, MissingBytes: 0, MissingFiles: 0},
	})
	require.Len(t, short, 1)
	assert.Equal(t, "/short", short[0].Root)
}

// Pin the exact comparison (MissingBytes+headroom > FreeBytes): a strict '>'
// means an exact fit is NOT a shortfall, and the headroom addition must count.
// A mutant flipping '>' to '>=' or dropping volumeHeadroomBytes survives the
// far-from-boundary cases above but dies here.
func TestBackupShortfalls_Boundary(t *testing.T) {
	free := int64(1<<30) + volumeHeadroomBytes
	short := backupShortfalls([]SeekVolume{
		{Root: "/exact", FreeBytes: free, MissingBytes: 1 << 30, MissingFiles: 1},      // sum == free: fits, not short
		{Root: "/over", FreeBytes: free, MissingBytes: (1 << 30) + 1, MissingFiles: 1}, // sum == free+1: short
	})
	require.Len(t, short, 1)
	assert.Equal(t, "/over", short[0].Root)
}
