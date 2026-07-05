package maintenance_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReindexer is a state-machine fake of mkv.Reindexer: HasSeekIndex/HasBackup
// answer from canned maps keyed by path, and the mutating calls record what they
// were asked to do. ReindexInPlace can be gated on a channel to hold a batch
// open deterministically (for the concurrency-guard test) and can fail per path.
type fakeReindexer struct {
	mu        sync.Mutex
	hasIndex  map[string]bool
	hasBackup map[string]bool
	failWith  map[string]error
	skip      map[string]bool // when true, ReindexInPlace reports Skipped (already indexed)
	panicOn   string          // when set, ReindexInPlace panics on this path
	reindexed []string
	restored  []string
	gate      chan struct{} // when non-nil, ReindexInPlace blocks until it's closed
}

func (f *fakeReindexer) HasSeekIndex(_ context.Context, p string) (bool, error) {
	return f.hasIndex[p], nil
}
func (f *fakeReindexer) HasBackup(_ context.Context, p string) (bool, error) {
	return f.hasBackup[p], nil
}
func (f *fakeReindexer) ReindexInPlace(_ context.Context, p string, _ mkv.ReindexOptions) (mkv.ReindexResult, error) {
	if f.gate != nil {
		<-f.gate
	}
	if f.panicOn == p {
		panic("reindexer exploded on " + p)
	}
	if err := f.failWith[p]; err != nil {
		return mkv.ReindexResult{}, err
	}
	if f.skip[p] {
		return mkv.ReindexResult{Skipped: true}, nil
	}
	f.mu.Lock()
	f.reindexed = append(f.reindexed, p)
	f.mu.Unlock()
	return mkv.ReindexResult{BackupPath: p + ".mkvgobak", SrcBytes: 100, DstBytes: 90}, nil
}
func (f *fakeReindexer) RestoreBackup(_ context.Context, p string) error {
	f.mu.Lock()
	f.restored = append(f.restored, p)
	f.mu.Unlock()
	return nil
}

type fakeCatalog struct {
	files []appmaintenance.MediaRef
}

func (c fakeCatalog) ListMediaFiles(_ context.Context, _ string) ([]appmaintenance.MediaRef, error) {
	return c.files, nil
}
func (c fakeCatalog) GetMediaFile(_ context.Context, mediaID string) (appmaintenance.MediaRef, error) {
	for _, f := range c.files {
		if f.MediaID == mediaID {
			return f, nil
		}
	}
	return appmaintenance.MediaRef{}, os.ErrNotExist
}

// waitIdle polls Status until no job is running (every entry point is async).
func waitIdle(t *testing.T, svc *appmaintenance.SeekIndexService) appmaintenance.JobStatus {
	t.Helper()
	var st appmaintenance.JobStatus
	require.Eventually(t, func() bool {
		st = svc.Status()
		return !st.Running
	}, 5*time.Second, 5*time.Millisecond, "seek-index job did not finish")
	return st
}

// A scan job inspects only EBML files that exist on disk, rolls up the missing-
// index / has-backup counts and caches the result for CachedScan.
func TestSeekIndexService_ScanJob(t *testing.T) {
	dir := t.TempDir()
	indexed := filepath.Join(dir, "indexed.mkv")
	missing := filepath.Join(dir, "missing.mkv") // exists on disk, no Cues, has backup
	notEBML := filepath.Join(dir, "movie.mp4")   // not an EBML container → skipped
	absent := filepath.Join(dir, "ghost.mkv")    // recorded in DB but not on disk → skipped
	for _, p := range []string{indexed, missing, notEBML} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}

	reidx := &fakeReindexer{
		hasIndex:  map[string]bool{indexed: true, missing: false},
		hasBackup: map[string]bool{missing: true},
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "Indexed", Path: indexed},
		{MediaID: "m2", Title: "Missing", Path: missing},
		{MediaID: "m3", Title: "Mp4", Path: notEBML},
		{MediaID: "m4", Title: "Ghost", Path: absent},
		{MediaID: "m5", Title: "NoFile", Path: ""},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	_, _, ok := svc.CachedScan("")
	assert.False(t, ok, "no scan cached before the first job")

	require.NoError(t, svc.StartScan(context.Background(), ""))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobScan, st.Last.Kind)
	assert.Empty(t, st.Last.Err)
	assert.Equal(t, 3, st.Last.Total, "the three on-disk-or-not EBML candidates were walked")

	res, at, ok := svc.CachedScan("")
	require.True(t, ok, "scan result is cached")
	assert.False(t, at.IsZero())
	assert.Equal(t, 2, res.Total, "only the two on-disk EBML files are counted")
	assert.Equal(t, 1, res.MissingIndex, "exactly the un-indexed file")
	assert.Equal(t, 1, res.WithBackup, "exactly the file with a .mkvgobak")
	require.Len(t, res.Items, 2)
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}
	assert.True(t, byID["m1"].HasIndex)
	assert.Equal(t, "mkv", byID["m1"].Container)
	assert.False(t, byID["m2"].HasIndex)
	assert.True(t, byID["m2"].HasBackup)
}

func TestSeekIndexService_ReindexMedia_AsyncAndUpdatesCache(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: false}, hasBackup: map[string]bool{}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	// Seed the cache so the repair has something to update.
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobReindex, st.Last.Kind)
	assert.Equal(t, 1, st.Last.Done)
	assert.Zero(t, st.Last.Failed)
	assert.Equal(t, []string{p}, reidx.reindexed, "reindexed the resolved path")

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	require.Len(t, res.Items, 1)
	assert.True(t, res.Items[0].HasIndex, "cache reflects the repair without a re-scan")
	assert.True(t, res.Items[0].HasBackup)
	assert.Zero(t, res.MissingIndex, "rollups recomputed")

	// A media id with no file is a clean, synchronous error — not a job.
	require.Error(t, svc.StartReindexMedia(context.Background(), "missing-id"))
}

// fakeFileState records the fingerprint refreshes the reindex asks for.
type fakeFileState struct {
	mu    sync.Mutex
	calls []fileStateCall
	err   error
}
type fileStateCall struct {
	path string
	size int64
	mt   time.Time
}

func (f *fakeFileState) RefreshFileState(_ context.Context, path string, size int64, mt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fileStateCall{path, size, mt})
	return f.err
}

// After a successful reindex the service re-stamps the file's scan fingerprint
// (its real on-disk size+mtime) so the next library scan sees it UNCHANGED and
// keeps the keyframe index instead of re-probing — no transcode window.
func TestSeekIndexService_ReindexMedia_RefreshesFileState(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("reindexed-bytes"), 0o644))
	fi, err := os.Stat(p)
	require.NoError(t, err)

	reidx := &fakeReindexer{hasIndex: map[string]bool{p: false}, hasBackup: map[string]bool{}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	fs := &fakeFileState{}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithFileStateRefresher(fs)

	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1"))
	waitIdle(t, svc)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	require.Len(t, fs.calls, 1, "refreshed the reindexed file's fingerprint exactly once")
	assert.Equal(t, p, fs.calls[0].path)
	assert.Equal(t, fi.Size(), fs.calls[0].size, "stamps the real on-disk size")
	assert.True(t, fi.ModTime().Equal(fs.calls[0].mt), "stamps the real on-disk mtime")
}

// A skipped reindex (file already indexed) must NOT refresh — nothing changed
// on disk, so re-stamping would be a lie that could mask a genuine future edit.
func TestSeekIndexService_ReindexMedia_SkippedDoesNotRefresh(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: true}, hasBackup: map[string]bool{}, skip: map[string]bool{p: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	fs := &fakeFileState{}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithFileStateRefresher(fs)

	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1"))
	waitIdle(t, svc)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	assert.Empty(t, fs.calls, "a skipped (already-indexed) reindex refreshes nothing")
}

func TestSeekIndexService_RollbackMedia_Async(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.mkv")
	reidx := &fakeReindexer{hasIndex: map[string]bool{}, hasBackup: map[string]bool{}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	require.NoError(t, svc.StartRollbackMedia(context.Background(), "m1"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobRollback, st.Last.Kind)
	assert.Equal(t, []string{p}, reidx.restored)
}

// A library reindex job scans then repairs only un-indexed files, refuses a
// second concurrent job while running, and reports per-file failures (with the
// read-only flag) in the summary.
func TestSeekIndexService_ReindexLibrary_GuardsConcurrencyAndReportsFailures(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv") // un-indexed → repaired
	b := filepath.Join(dir, "b.mkv") // indexed → not a candidate
	c := filepath.Join(dir, "c.mkv") // un-indexed but on a :ro mount → failure
	for _, p := range []string{a, b, c} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	gate := make(chan struct{})
	reidx := &fakeReindexer{
		hasIndex:  map[string]bool{a: false, b: true, c: false},
		hasBackup: map[string]bool{},
		failWith:  map[string]error{c: syscall.EROFS},
		gate:      gate, // hold the background batch open
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: a}, {MediaID: "m2", Path: b}, {MediaID: "m3", Path: c},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "lib1"))

	// The job is blocked on the gate → any concurrent start is refused.
	err := svc.StartReindexLibrary(context.Background(), "lib1")
	require.ErrorIs(t, err, appmaintenance.ErrJobRunning)
	require.ErrorIs(t, svc.StartScan(context.Background(), ""), appmaintenance.ErrJobRunning,
		"jobs are serialized globally, scan included")

	close(gate) // let the background batch drain
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobReindex, st.Last.Kind)
	assert.Equal(t, "lib1", st.Last.Scope)
	assert.Equal(t, 2, st.Last.Total, "only the two un-indexed files were candidates")
	assert.Equal(t, 2, st.Last.Done)
	assert.Equal(t, 1, st.Last.Failed)
	assert.True(t, st.Last.ReadOnly, "EROFS failure flags the summary read-only")
	require.Len(t, st.Last.Failures, 1)
	assert.Equal(t, c, st.Last.Failures[0].Path)
	assert.True(t, st.Last.Failures[0].ReadOnly)
	assert.Equal(t, []string{a}, reidx.reindexed, "only the healthy candidate was repaired")

	// The scan ran as phase 1 → its result is cached for the scope, and the
	// successful repair is folded into it.
	res, _, ok := svc.CachedScan("lib1")
	require.True(t, ok)
	assert.Equal(t, 3, res.Total)
	assert.Equal(t, 1, res.MissingIndex, "a repaired, only c still missing")
}

// A panic inside the reindexer must release the job slot (otherwise every
// later start is refused with ErrJobRunning until restart) and surface as the
// job-level error of the summary.
func TestSeekIndexService_PanicReleasesJobSlot(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: false}, hasBackup: map[string]bool{}, panicOn: p}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Contains(t, st.Last.Err, "panicked", "the panic surfaces as the job error")

	// The slot is free again: a new job starts cleanly.
	reidx.panicOn = ""
	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1"))
	waitIdle(t, svc)
}

// A rollback job restores only backed-up files and flips the cache entries.
func TestSeekIndexService_RollbackLibrary(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv") // has backup → restored
	b := filepath.Join(dir, "b.mkv") // no backup → untouched
	for _, p := range []string{a, b} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{
		hasIndex:  map[string]bool{a: true, b: true},
		hasBackup: map[string]bool{a: true},
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: a}, {MediaID: "m2", Path: b},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	require.NoError(t, svc.StartRollbackLibrary(context.Background(), ""))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobRollback, st.Last.Kind)
	assert.Equal(t, 1, st.Last.Done)
	assert.Equal(t, []string{a}, reidx.restored)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	byPath := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byPath[it.Path] = it
	}
	assert.False(t, byPath[a].HasIndex, "restored original is back to no index")
	assert.False(t, byPath[a].HasBackup)
	assert.True(t, byPath[b].HasIndex)
}
