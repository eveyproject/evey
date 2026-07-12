package maintenance_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	deleted   []string
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
	if f.hasBackup == nil {
		f.hasBackup = map[string]bool{}
	}
	f.hasBackup[p] = true // the rewrite leaves a .mkvgobak, like the real adapter
	f.mu.Unlock()
	return mkv.ReindexResult{BackupPath: p + ".mkvgobak", SrcBytes: 100, DstBytes: 90}, nil
}
func (f *fakeReindexer) SupportsInPlace() bool { return false }

func (f *fakeReindexer) RestoreBackup(_ context.Context, p string) error {
	f.mu.Lock()
	f.restored = append(f.restored, p)
	f.mu.Unlock()
	return nil
}

type fakeCatalog struct {
	files []appmaintenance.MediaRef
	lists *int32 // optional: counts ListMediaFiles calls
}

func (c fakeCatalog) ListMediaFiles(_ context.Context, _ string) ([]appmaintenance.MediaRef, error) {
	if c.lists != nil {
		atomic.AddInt32(c.lists, 1)
	}
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

	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1", appmaintenance.ReindexRunOptions{}))
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

	// A media id with no file is a clean, synchronous error - not a job.
	require.Error(t, svc.StartReindexMedia(context.Background(), "missing-id", appmaintenance.ReindexRunOptions{}))
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
// keeps the keyframe index instead of re-probing - no transcode window.
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

	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	require.Len(t, fs.calls, 1, "refreshed the reindexed file's fingerprint exactly once")
	assert.Equal(t, p, fs.calls[0].path)
	assert.Equal(t, fi.Size(), fs.calls[0].size, "stamps the real on-disk size")
	assert.True(t, fi.ModTime().Equal(fs.calls[0].mt), "stamps the real on-disk mtime")
}

// A skipped reindex (file already indexed) must NOT refresh - nothing changed
// on disk, so re-stamping would be a lie that could mask a genuine future edit.
func TestSeekIndexService_ReindexMedia_SkippedDoesNotRefresh(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: true}, hasBackup: map[string]bool{}, skip: map[string]bool{p: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	fs := &fakeFileState{}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithFileStateRefresher(fs)

	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1", appmaintenance.ReindexRunOptions{}))
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

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "lib1", appmaintenance.ReindexRunOptions{}))

	// The job is blocked on the gate → any concurrent start is refused.
	err := svc.StartReindexLibrary(context.Background(), "lib1", appmaintenance.ReindexRunOptions{})
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

// With a backup window, a whole-library repair never holds more than the N
// most recent backups: as the job advances past the window, the oldest
// backups are purged automatically (bounded disk usage), and the job's end
// drains the remainder - the volume finishes with ZERO .mkvgobak, no manual
// purge needed. The cache reflects every deletion.
func TestSeekIndexService_ReindexLibrary_RollingBackupWindow(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 5)
	files := make([]appmaintenance.MediaRef, 5)
	hasIndex := map[string]bool{}
	for i := range paths {
		p := filepath.Join(dir, fmt.Sprintf("f%d.mkv", i))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
		paths[i] = p
		files[i] = appmaintenance.MediaRef{MediaID: fmt.Sprintf("m%d", i), Path: p}
		hasIndex[p] = false
	}
	reidx := &fakeReindexer{hasIndex: hasIndex, hasBackup: map[string]bool{}}
	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: files}, nil)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{BackupWindow: 2}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 5, st.Last.Done)
	assert.Zero(t, st.Last.Failed)
	assert.Equal(t, paths, reidx.deleted,
		"every backup purged: the three oldest in order as the window slides, then the end-of-job drain")

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	for _, it := range res.Items {
		assert.False(t, it.HasBackup, "no backup survives the job: %s", it.Path)
	}
	assert.Zero(t, res.WithBackup, "rollup reflects the drained window")
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

	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1", appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Contains(t, st.Last.Err, "panicked", "the panic surfaces as the job error")

	// The slot is free again: a new job starts cleanly.
	reidx.panicOn = ""
	require.NoError(t, svc.StartReindexMedia(context.Background(), "m1", appmaintenance.ReindexRunOptions{}))
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

// fakeAudioDelayProber answers from a canned map; unknown paths error (the
// scan must keep the file and report 0).
type fakeAudioDelayProber struct {
	delays map[string]float64
}

func (f fakeAudioDelayProber) ProbeMaxAudioStartDelaySec(_ context.Context, p string) (float64, error) {
	d, ok := f.delays[p]
	if !ok {
		return 0, os.ErrInvalid
	}
	return d, nil
}

// The audio-delay diagnosis is optional and per-file: with a prober wired,
// scans carry each file's delay and roll up the count above the flag floor; a
// probe failure keeps the file in the scan with delay 0 (secondary
// diagnostics never drop items).
func TestSeekIndexService_ScanFlagsAudioDelay(t *testing.T) {
	dir := t.TempDir()
	delayed := filepath.Join(dir, "delayed.mkv")
	aligned := filepath.Join(dir, "aligned.mkv")
	probeFails := filepath.Join(dir, "probefails.mkv")
	for _, p := range []string{delayed, aligned, probeFails} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}

	reidx := &fakeReindexer{
		hasIndex: map[string]bool{delayed: true, aligned: true, probeFails: true},
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "Delayed", Path: delayed},
		{MediaID: "m2", Title: "Aligned", Path: aligned},
		{MediaID: "m3", Title: "ProbeFails", Path: probeFails},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{
			delayed: 0.9,
			aligned: 0.03, // AAC priming territory: under the flag floor
		}})

	require.NoError(t, svc.StartScan(context.Background(), ""))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Empty(t, st.Last.Err)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 3, res.Total, "the probe failure must not drop the file")
	assert.Equal(t, 1, res.AudioDelayed, "only the file above the flag floor counts")
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}
	assert.InDelta(t, 0.9, byID["m1"].AudioDelaySec, 1e-9)
	assert.InDelta(t, 0.03, byID["m2"].AudioDelaySec, 1e-9)
	assert.Zero(t, byID["m3"].AudioDelaySec)
}

// Without a prober wired (the default), scans behave exactly as before.
func TestSeekIndexService_ScanWithoutAudioDelayProber(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Zero(t, res.AudioDelayed)
	require.Len(t, res.Items, 1)
	assert.Zero(t, res.Items[0].AudioDelaySec)
}

func (f *fakeReindexer) DeleteBackup(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.hasBackup[p] {
		return errors.New("no backup at " + p)
	}
	delete(f.hasBackup, p)
	f.deleted = append(f.deleted, p)
	return nil
}

// DeleteBackupMedia is synchronous (an unlink), updates the cached scan, and
// errors cleanly on a media without a backup.
func TestSeekIndexService_DeleteBackupMedia(t *testing.T) {
	dir := t.TempDir()
	withBak := filepath.Join(dir, "a.mkv")
	noBak := filepath.Join(dir, "b.mkv")
	for _, p := range []string{withBak, noBak} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{
		hasIndex:  map[string]bool{withBak: true, noBak: true},
		hasBackup: map[string]bool{withBak: true},
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: withBak},
		{MediaID: "m2", Path: noBak},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.DeleteBackupMedia(context.Background(), "m1"))
	assert.Equal(t, []string{withBak}, reidx.deleted)
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 0, res.WithBackup, "the cached scan reflects the deletion without a re-scan")

	require.Error(t, svc.DeleteBackupMedia(context.Background(), "m2"), "no backup → explicit error")
	require.Error(t, svc.DeleteBackupMedia(context.Background(), "ghost"), "unknown media → explicit error")
}

// The purge job deletes every backup in scope and nothing else.
func TestSeekIndexService_PurgeBackups(t *testing.T) {
	dir := t.TempDir()
	bak1 := filepath.Join(dir, "a.mkv")
	bak2 := filepath.Join(dir, "b.mkv")
	clean := filepath.Join(dir, "c.mkv")
	for _, p := range []string{bak1, bak2, clean} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{
		hasIndex:  map[string]bool{bak1: true, bak2: true, clean: true},
		hasBackup: map[string]bool{bak1: true, bak2: true},
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: bak1}, {MediaID: "m2", Path: bak2}, {MediaID: "m3", Path: clean},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	require.NoError(t, svc.StartPurgeBackups(context.Background(), ""))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobPurge, st.Last.Kind)
	assert.Equal(t, 2, st.Last.Done)
	assert.Equal(t, 0, st.Last.Failed)
	assert.ElementsMatch(t, []string{bak1, bak2}, reidx.deleted)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 0, res.WithBackup)
}

// A completed scan must survive a server restart: a new service instance
// pointed at the same state dir serves the cached result without touching the
// disk again - re-scanning 6k files after every reboot is minutes of probes.
func TestSeekIndexService_ScanCacheSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	state := t.TempDir()
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: false}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "F", Path: p}}}

	svc1 := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithStateDir(state)
	require.NoError(t, svc1.StartScan(context.Background(), ""))
	waitIdle(t, svc1)
	res1, at1, ok := svc1.CachedScan("")
	require.True(t, ok)

	// "Restart": a fresh instance over the same state dir.
	svc2 := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithStateDir(state)
	res2, at2, ok := svc2.CachedScan("")
	require.True(t, ok, "the persisted scan is served after a restart")
	assert.Equal(t, res1.Items, res2.Items)
	assert.Equal(t, res1.MissingIndex, res2.MissingIndex)
	assert.WithinDuration(t, at1, at2, time.Second, "the scan timestamp survives")
}

// A job that dies with the process (deploy/restart) must not vanish without a
// trace: the next boot surfaces it as the last job, interrupted, with the
// progress it had reached - so the operator knows the library is NOT done.
func TestSeekIndexService_InterruptedJobSurfacesAfterRestart(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv")
	b := filepath.Join(dir, "b.mkv")
	for _, p := range []string{a, b} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	state := t.TempDir()
	gate := make(chan struct{})
	reidx := &fakeReindexer{hasIndex: map[string]bool{a: false, b: false}, gate: gate}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: a}, {MediaID: "m2", Path: b}}}

	svc1 := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithStateDir(state)
	require.NoError(t, svc1.StartReindexLibrary(context.Background(), "lib9", appmaintenance.ReindexRunOptions{}))
	// Wait until the job is PARKED on the gate (processing, first file in
	// flight): only then is the marker stable - the scan phase rewrites it on
	// every probed file, and consuming it mid-scan would race a rewrite.
	require.Eventually(t, func() bool {
		st := svc1.Status()
		return st.Running && st.Phase == appmaintenance.PhaseProcessing && st.CurrentPath != ""
	}, 5*time.Second, 5*time.Millisecond)
	// The process "dies" here: svc1 is abandoned mid-job (its goroutine is
	// parked on the gate; released and drained at cleanup so it cannot race
	// the TempDir removal with marker writes).
	t.Cleanup(func() {
		close(gate)
		require.Eventually(t, func() bool { return !svc1.Status().Running },
			5*time.Second, 5*time.Millisecond, "abandoned job did not drain")
	})

	svc2 := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithStateDir(state)
	st := svc2.Status()
	require.NotNil(t, st.Last, "the interrupted job surfaces as the last summary")
	assert.Equal(t, appmaintenance.JobReindex, st.Last.Kind)
	assert.Equal(t, "lib9", st.Last.Scope)
	assert.Contains(t, st.Last.Err, "interrupted", "the summary names the interruption")
	assert.False(t, svc2.Status().Running, "the marker does not resurrect a running job")

	// The marker is consumed: another restart reports nothing stale.
	svc3 := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithStateDir(state)
	if l := svc3.Status().Last; l != nil {
		assert.NotContains(t, l.Err, "interrupted", "the marker is one-shot")
	}
}

// A batch job must reuse the cached scan as its phase 1 instead of re-walking
// the whole library (the scan is minutes of per-file probes; every repair
// already re-checks its own file, so a cached candidate list is safe).
func TestSeekIndexService_BatchReusesCachedScan(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	var lists int32
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: false}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}, lists: &lists}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	require.EqualValues(t, 1, atomic.LoadInt32(&lists))

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 1, st.Last.Done)
	assert.Equal(t, []string{p}, reidx.reindexed)
	assert.EqualValues(t, 1, atomic.LoadInt32(&lists), "the batch used the cached scan, no re-walk")

	// Without a cache (new scope), the batch scans as before.
	require.NoError(t, svc.StartReindexLibrary(context.Background(), "lib-uncached", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)
	assert.EqualValues(t, 2, atomic.LoadInt32(&lists), "no cache for the scope - the batch scans")
}

// fakeAudioRemuxer records the remuxes it is asked to run.
type fakeAudioRemuxer struct {
	mu       sync.Mutex
	remuxed  []string
	failWith map[string]error
}

func (f *fakeAudioRemuxer) RemuxAudioDelay(_ context.Context, p string) (string, bool, error) {
	if err := f.failWith[p]; err != nil {
		return "", false, err
	}
	f.mu.Lock()
	f.remuxed = append(f.remuxed, p)
	f.mu.Unlock()
	return p + ".mkvgobak", false, nil
}

// The audio-remux batch targets ONLY the files the scan flagged with a start
// delay above the defect floor, repairs them through the remuxer port, and
// folds the fix into the cached scan (delay cleared, backup visible) - the
// same lifecycle as the reindex batch.
func TestSeekIndexService_RemuxAudioLibrary(t *testing.T) {
	dir := t.TempDir()
	delayed := filepath.Join(dir, "delayed.mkv")
	aligned := filepath.Join(dir, "aligned.mkv")
	failing := filepath.Join(dir, "failing.mkv")
	for _, p := range []string{delayed, aligned, failing} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{delayed: true, aligned: true, failing: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: delayed}, {MediaID: "m2", Path: aligned}, {MediaID: "m3", Path: failing},
	}}
	remux := &fakeAudioRemuxer{failWith: map[string]error{failing: errors.New("streams disagree")}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{
			delayed: 0.9, aligned: 0.02, failing: 1.4,
		}}).
		WithAudioRemuxer(remux)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.StartRemuxAudioLibrary(context.Background(), ""))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobRemuxAudio, st.Last.Kind)
	assert.Equal(t, 2, st.Last.Total, "only the two files above the defect floor are candidates")
	assert.Equal(t, 1, st.Last.Failed)
	assert.Equal(t, []string{delayed}, remux.remuxed)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	byPath := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byPath[it.Path] = it
	}
	assert.Zero(t, byPath[delayed].AudioDelaySec, "the repaired file no longer reports a delay")
	assert.True(t, byPath[delayed].HasBackup, "the remux backup is visible for rollback/purge")
	assert.InDelta(t, 1.4, byPath[failing].AudioDelaySec, 1e-9, "a failed remux keeps its diagnosis")
	assert.Zero(t, res.AudioDelayed-1, "rollup reflects the single remaining defective file")
}

// Single-media remux: async job, unknown media fails fast, unconfigured
// remuxer is a clean synchronous error (the UI hides the button, the API
// still answers something actionable).
func TestSeekIndexService_RemuxAudioMedia(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	remux := &fakeAudioRemuxer{}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithAudioRemuxer(remux)

	require.NoError(t, svc.StartRemuxAudioMedia(context.Background(), "m1"))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobRemuxAudio, st.Last.Kind)
	assert.Equal(t, []string{p}, remux.remuxed)

	require.Error(t, svc.StartRemuxAudioMedia(context.Background(), "ghost"), "unknown media fails fast")

	bare := appmaintenance.NewSeekIndexService(reidx, cat, nil)
	require.Error(t, bare.StartRemuxAudioLibrary(context.Background(), ""), "unconfigured remuxer is a clean error")
	require.Error(t, bare.StartRemuxAudioMedia(context.Background(), "m1"))
}

// The combined "repair all" job reindexes un-indexed files AND remuxes
// audio-delayed files in a single pass when includeAudio is set; with it
// unset it behaves exactly like a reindex batch. A file carrying BOTH defects
// is only reindexed this run (its true original backup must not be clobbered
// by a second swap) - its audio is caught on the next run.
func TestSeekIndexService_RepairLibrary_Combined(t *testing.T) {
	dir := t.TempDir()
	noIndex := filepath.Join(dir, "noindex.mkv")   // reindex only
	delayed := filepath.Join(dir, "delayed.mkv")   // remux only
	both := filepath.Join(dir, "both.mkv")         // reindex this run, audio deferred
	healthy := filepath.Join(dir, "healthy.mkv")   // untouched
	for _, p := range []string{noIndex, delayed, both, healthy} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{
		hasIndex:  map[string]bool{noIndex: false, delayed: true, both: false, healthy: true},
		hasBackup: map[string]bool{},
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: noIndex}, {MediaID: "m2", Path: delayed},
		{MediaID: "m3", Path: both}, {MediaID: "m4", Path: healthy},
	}}
	remux := &fakeAudioRemuxer{}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{delayed: 0.9, both: 0.9}}).
		WithAudioRemuxer(remux)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{}, true))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 3, st.Last.Total, "2 reindex candidates + 1 audio-only")
	assert.Zero(t, st.Last.Failed)
	assert.ElementsMatch(t, []string{noIndex, both}, reidx.reindexed, "both un-indexed files reindexed")
	assert.Equal(t, []string{delayed}, remux.remuxed, "only the audio-ONLY file remuxed; the both-defect file's audio is deferred")

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Zero(t, res.MissingIndex)
}

// Without includeAudio the combined entry point is a pure reindex batch.
func TestSeekIndexService_RepairLibrary_IndexOnly(t *testing.T) {
	dir := t.TempDir()
	noIndex := filepath.Join(dir, "n.mkv")
	delayed := filepath.Join(dir, "d.mkv")
	for _, p := range []string{noIndex, delayed} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{noIndex: false, delayed: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: noIndex}, {MediaID: "m2", Path: delayed}}}
	remux := &fakeAudioRemuxer{}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{delayed: 0.9}}).
		WithAudioRemuxer(remux)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{}, false))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 1, st.Last.Total)
	assert.Equal(t, []string{noIndex}, reidx.reindexed)
	assert.Empty(t, remux.remuxed, "audio untouched when includeAudio is false")
}

// includeAudio with no remuxer wired quietly degrades to a reindex-only batch
// (a build without ffmpeg) rather than failing the whole repair.
func TestSeekIndexService_RepairLibrary_AudioWantedButUnavailable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "n.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: false}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil) // no remuxer
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}, true))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, []string{p}, reidx.reindexed)
}
