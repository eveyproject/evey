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
	// failStrictWith fails a path only when the run is NOT a resync - the shape
	// of a source whose cluster stream the strict rewrite refuses to parse and
	// only the tolerant one gets past.
	failStrictWith map[string]error
	skip      map[string]bool // when true, ReindexInPlace reports Skipped (already indexed)
	// restoreFail makes putting the original back fail - the case where the
	// damaged file stays on disk and the journal must not claim otherwise.
	restoreFail error
	panicOn     string // when set, ReindexInPlace panics on this path
	reindexed   []string
	restored    []string
	deleted     []string
	gate        chan struct{} // when non-nil, ReindexInPlace blocks until it's closed
	// onReindexed fires after a successful rewrite, before the service
	// re-diagnoses the file - the hook a test needs to make the fake diagnoser
	// answer differently once the file has actually been repaired.
	onReindexed func(path string)
}

func (f *fakeReindexer) HasSeekIndex(_ context.Context, p string) (bool, error) {
	return f.hasIndex[p], nil
}
func (f *fakeReindexer) HasBackup(_ context.Context, p string) (bool, error) {
	return f.hasBackup[p], nil
}
func (f *fakeReindexer) ReindexInPlace(_ context.Context, p string, opts mkv.ReindexOptions) (mkv.ReindexResult, error) {
	if f.gate != nil {
		<-f.gate
	}
	if f.panicOn == p {
		panic("reindexer exploded on " + p)
	}
	if err := f.failWith[p]; err != nil {
		return mkv.ReindexResult{}, err
	}
	if err := f.failStrictWith[p]; err != nil && !opts.Resync {
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
	hook := f.onReindexed
	f.mu.Unlock()
	if hook != nil {
		hook(p)
	}
	return mkv.ReindexResult{BackupPath: p + ".mkvgobak", SrcBytes: 100, DstBytes: 90}, nil
}
func (f *fakeReindexer) SupportsInPlace() bool { return false }

func (f *fakeReindexer) RestoreBackup(_ context.Context, p string) error {
	f.mu.Lock()
	f.restored = append(f.restored, p)
	err := f.restoreFail
	f.mu.Unlock()
	return err
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

// fakeDiagnoser answers from a canned map; an unknown path errors so the scan
// drops it (a file it cannot parse at all). calls counts Diagnose invocations
// per path so a test can prove a cached diagnosis was REUSED (not re-probed).
type fakeDiagnoser struct {
	byPath map[string]*mkv.Diagnosis
	calls  map[string]int
	gate   *scanGate // optional: holds the diagnosis of one path open until released
}

// scanGate lets a test keep a scan busy on one file while it acts on the
// cached result - the shape of an operator acknowledging a verdict while a
// long scan runs. Arm it after the first scan so the initial pass is free.
type scanGate struct {
	path     string
	armed    atomic.Bool
	started  chan struct{} // closed when the gated path is being diagnosed
	release  chan struct{} // closed by the test to let the scan finish
	onceOpen sync.Once
}

func newScanGate(path string) *scanGate {
	return &scanGate{path: path, started: make(chan struct{}), release: make(chan struct{})}
}

func (f fakeDiagnoser) Diagnose(_ context.Context, p string) (*mkv.Diagnosis, error) {
	if f.calls != nil {
		f.calls[p]++
	}
	if f.gate != nil && f.gate.armed.Load() && p == f.gate.path {
		f.gate.onceOpen.Do(func() { close(f.gate.started) })
		<-f.gate.release
	}
	d, ok := f.byPath[p]
	if !ok {
		return nil, os.ErrInvalid
	}
	return d, nil
}

// With a Diagnoser wired, one Diagnose per file replaces the HasSeekIndex +
// audio-delay probes AND adds the value that routes the repair: a
// present-but-unusable index (audio-keyed cues) is seen as a defect, and
// corruption is split into truncated (re-acquire) vs damaged (resync). The
// reindexer's HasSeekIndex must NOT be consulted - Diagnose is authoritative.
func TestSeekIndexService_ScanWithDiagnoser(t *testing.T) {
	dir := t.TempDir()
	misskeyed := filepath.Join(dir, "misskeyed.mkv") // Cues present but audio-keyed
	truncated := filepath.Join(dir, "truncated.mkv") // incomplete download
	damaged := filepath.Join(dir, "damaged.mkv")     // mid-file corruption
	healthy := filepath.Join(dir, "healthy.mkv")
	for _, p := range []string{misskeyed, truncated, damaged, healthy} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}

	// The reindexer would call every file indexed - proving Diagnose overrides it.
	reidx := &fakeReindexer{hasIndex: map[string]bool{
		misskeyed: true, truncated: true, damaged: true, healthy: true,
	}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "Misskeyed", Path: misskeyed},
		{MediaID: "m2", Title: "Truncated", Path: truncated},
		{MediaID: "m3", Title: "Damaged", Path: damaged},
		{MediaID: "m4", Title: "Healthy", Path: healthy},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
			misskeyed: {Findings: []mkv.DiagnosisFinding{{Kind: "index-misskeyed"}}},
			truncated: {Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, Truncated: true, DamagedBytes: 4096},
			damaged:   {Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, DamagedBytes: 4096},
			healthy:   {Healthy: true, AudioDelaysMs: map[uint64]int64{2: 900}},
		}})

	require.NoError(t, svc.StartScan(context.Background(), ""))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Empty(t, st.Last.Err)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 4, res.Total)
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}

	// A present-but-unusable index counts as missing (must be reindexed).
	assert.False(t, byID["m1"].HasIndex, "misskeyed cues are a defect, not a usable index")
	// Truncated vs damaged split.
	assert.True(t, byID["m2"].Truncated, "damage running to EOF is a truncated download")
	assert.False(t, byID["m3"].Truncated)
	assert.EqualValues(t, 4096, byID["m3"].DamagedBytes, "mid-file damage is resync-repairable")
	// The healthy file's raw audio delay flows through the same floor as before.
	assert.InDelta(t, 0.9, byID["m4"].AudioDelaySec, 1e-9)
	assert.True(t, byID["m4"].HasIndex)

	assert.Equal(t, 1, res.MissingIndex, "only misskeyed lacks a usable index")
	assert.Equal(t, 1, res.Truncated)
	assert.Equal(t, 1, res.Damaged, "truncated is counted as truncated, not damaged")
	assert.Equal(t, 1, res.AudioDelayed)
	assert.Empty(t, reidx.reindexed, "a scan never mutates files")
}

// An audio track ending BEFORE the container's declared duration (mux defect:
// prod 2026-07-26, "Scary Movie", fr track dead for the last 4 min of credits)
// must surface as its own category: it is invisible to the index/damage checks
// (the file is structurally healthy) yet it used to kill playback at the wall.
// There is deliberately NO automated repair - the content simply is not in the
// file; playback fills the tail with silence, the operator re-acquires for a
// real fix.
func TestSeekIndexService_AudioTrackShortfall(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short_audio.mkv") // audio dies 249s before the end
	near := filepath.Join(dir, "near_end.mkv")     // audio ends 2s early: normal mux slack
	fake := filepath.Join(dir, "reallymp4.mkv")    // wrong container: probe is pointless
	failing := filepath.Join(dir, "probe_fails.mkv")
	for _, p := range []string{short, near, fake, failing} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{short: true, near: true, fake: false, failing: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "Short", Path: short},
		{MediaID: "m2", Title: "Near", Path: near},
		{MediaID: "m3", Title: "FakeMkv", Path: fake},
		{MediaID: "m4", Title: "ProbeFails", Path: failing},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
			short: {Healthy: true, AudioShortfallMs: 249_600},
			near:  {Healthy: true, AudioShortfallMs: 2_000},
			// a wrong container's Matroska duration is not one: whatever the
			// measurement says, no shortfall may be reported on it
			fake:    {Findings: []mkv.DiagnosisFinding{{Kind: "wrong-container"}}, AudioShortfallMs: 500_000},
			failing: {Healthy: true},
		}})

	require.NoError(t, svc.StartScan(context.Background(), ""))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Empty(t, st.Last.Err)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}

	assert.InDelta(t, 249.6, byID["m1"].AudioShortfallSec, 1e-9,
		"a track ending far before the declared duration is the defect")
	assert.Zero(t, byID["m2"].AudioShortfallSec,
		"a couple of seconds of mux slack is normal, not a defect")
	assert.Zero(t, byID["m3"].AudioShortfallSec,
		"a wrong-container file's Matroska duration is not one: a shortfall measured on it is meaningless")
	assert.Zero(t, byID["m4"].AudioShortfallSec, "no measurement, no defect")
	assert.Equal(t, 4, res.Total)

	assert.Equal(t, 1, res.AudioShort, "rollup counts only the real shortfall")
	assert.Empty(t, reidx.reindexed, "a scan never mutates files")
}

// The shortfall must NOT put the file into any repair bucket: the audio's tail
// does not exist in the file, so neither a reindex nor an audio remux can bring
// it back - "Repair all" must leave the file untouched.
func TestSeekIndexService_AudioTrackShortfall_NotRepairable(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short_audio.mkv")
	require.NoError(t, os.WriteFile(short, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{short: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "Short", Path: short}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{short: {Healthy: true, AudioShortfallMs: 249_600}}})

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	assert.Empty(t, reidx.reindexed, "a shortfall is not an index defect")
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 1, res.AudioShort, "the flag survives the repair pass untouched")
}

// After a restart the persisted scan must restore EVERY rollup, not just the
// original three: truncated/damaged/wrong-container/trailing-junk (and the
// audio shortfall) all read 0 on the stat cards after a reboot while the rows
// still carried their badges - the load path had its own, incomplete rollup
// loop.
func TestSeekIndexService_PersistedScan_RestoresAllRollups(t *testing.T) {
	dir := t.TempDir()
	names := []string{"trunc.mkv", "damaged.mkv", "fake.mkv", "junk.mkv", "short.mkv"}
	paths := make([]string, len(names))
	refs := make([]appmaintenance.MediaRef, len(names))
	for i, n := range names {
		paths[i] = filepath.Join(dir, n)
		require.NoError(t, os.WriteFile(paths[i], []byte("x"), 0o644))
		refs[i] = appmaintenance.MediaRef{MediaID: fmt.Sprintf("m%d", i+1), Title: n, Path: paths[i]}
	}
	state := t.TempDir()
	reidx := &fakeReindexer{hasIndex: map[string]bool{}}
	cat := fakeCatalog{files: refs}
	svc1 := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
			paths[0]: {Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, Truncated: true, DamagedBytes: 1},
			paths[1]: {Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, DamagedBytes: 2048},
			paths[2]: {Findings: []mkv.DiagnosisFinding{{Kind: "wrong-container"}}},
			paths[3]: {Findings: []mkv.DiagnosisFinding{{Kind: "trailing-junk"}}},
			paths[4]: {Healthy: true, AudioShortfallMs: 300_000},
		}}).
		WithStateDir(state)

	require.NoError(t, svc1.StartScan(context.Background(), ""))
	waitIdle(t, svc1)
	res1, _, ok := svc1.CachedScan("")
	require.True(t, ok)

	svc2 := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithStateDir(state)
	res2, _, ok := svc2.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, res1.Truncated, res2.Truncated, "truncated rollup must survive the restart")
	assert.Equal(t, res1.Damaged, res2.Damaged, "damaged rollup must survive the restart")
	assert.Equal(t, res1.WrongContainer, res2.WrongContainer, "wrong-container rollup must survive the restart")
	assert.Equal(t, res1.TrailingJunk, res2.TrailingJunk, "trailing-junk rollup must survive the restart")
	assert.Equal(t, res1.AudioShort, res2.AudioShort, "audio-shortfall rollup must survive the restart")
	assert.Positive(t, res2.Truncated)
	assert.Positive(t, res2.Damaged)
	assert.Positive(t, res2.WrongContainer)
	assert.Positive(t, res2.TrailingJunk)
	assert.Positive(t, res2.AudioShort)
}

// A re-scan must not re-diagnose files that have not changed since the last
// completed scan: in prod, diagnosing 6651 files takes ~1h30 (per-file head
// seeks + first-cluster reads on NAS disks), and virtually none of them change
// between two scans. (size, mtime) unchanged → the cached SeekStatus is
// reused; only the backup stat (cheap) is refreshed. A touched file is
// re-diagnosed for real.
func TestSeekIndexService_Rescan_ReusesUnchangedDiagnoses(t *testing.T) {
	dir := t.TempDir()
	stable := filepath.Join(dir, "stable.mkv")
	touched := filepath.Join(dir, "touched.mkv")
	for _, p := range []string{stable, touched} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{stable: true, touched: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "Stable", Path: stable},
		{MediaID: "m2", Title: "Touched", Path: touched},
	}}
	diag := fakeDiagnoser{calls: map[string]int{}, byPath: map[string]*mkv.Diagnosis{
		stable:  {Findings: []mkv.DiagnosisFinding{{Kind: "trailing-junk"}}, AudioDelaysMs: map[uint64]int64{1: 900}, AudioShortfallMs: 300_000},
		touched: {Healthy: true},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(diag)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	require.Equal(t, 1, diag.calls[stable])

	// Touch one file (mtime changes), leave the other alone.
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(touched, future, future))

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	assert.Equal(t, 1, diag.calls[stable], "an unchanged file must NOT be re-diagnosed")
	assert.Equal(t, 2, diag.calls[touched], "a touched file must be re-diagnosed")

	// The reused entry keeps every diagnosed fact and the rollups stay right.
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}
	assert.True(t, byID["m1"].TrailingJunk)
	assert.InDelta(t, 0.9, byID["m1"].AudioDelaySec, 1e-9)
	assert.InDelta(t, 300, byID["m1"].AudioShortfallSec, 1e-9)
	assert.Equal(t, 1, res.TrailingJunk)
	assert.Equal(t, 1, res.AudioShort)
}

// The backup flag must stay LIVE on a reused entry: a rollback/purge between
// two scans changes it without touching the media file itself.
func TestSeekIndexService_Rescan_RefreshesBackupOnReusedEntry(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: true}, hasBackup: map[string]bool{p: false}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "F", Path: p}}}
	diag := fakeDiagnoser{calls: map[string]int{}, byPath: map[string]*mkv.Diagnosis{p: {Healthy: true}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithDiagnoser(diag)

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	// A backup appears without the media file changing.
	reidx.mu.Lock()
	reidx.hasBackup[p] = true
	reidx.mu.Unlock()

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.True(t, res.Items[0].HasBackup, "backup status must be re-stat'ed even on a reused diagnosis")
	assert.Equal(t, 1, res.WithBackup)
	assert.Equal(t, 1, diag.calls[p], "the reuse must still have skipped the diagnose")
}

// A .mkv that is really MP4/MOV (mkvgo's wrong-container finding) has no
// Matroska seek index to build: it must be kept OUT of the missing-index bucket
// (so "Repair all" never wastes a doomed reindex on it) and surfaced in its own
// wrong-container count instead.
func TestSeekIndexService_WrongContainer(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "reallymp4.mkv")
	healthy := filepath.Join(dir, "ok.mkv")
	for _, p := range []string{fake, healthy} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{fake: false, healthy: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "FakeMkv", Path: fake},
		{MediaID: "m2", Title: "Ok", Path: healthy},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
			fake:    {Findings: []mkv.DiagnosisFinding{{Kind: "wrong-container", Remedy: "rename/remux"}}},
			healthy: {Healthy: true},
		}})

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}
	assert.True(t, byID["m1"].WrongContainer, "the disguised MP4 must be flagged wrong-container")
	assert.True(t, byID["m1"].HasIndex, "wrong-container is kept out of the reindex list (no Cues to build)")
	assert.Equal(t, 1, res.WrongContainer)
	assert.Equal(t, 0, res.MissingIndex, "a wrong-container file is NOT a missing-index file")
	assert.Empty(t, reidx.reindexed, "a scan never reindexes")
}

// Trailing junk (surplus bytes past the Segment end) is complete media: the
// scan flags it on its own axis, keeps it in the "has index" set (nothing to
// reindex), rolls up its own count, and never confuses it with a damaged file.
func TestSeekIndexService_TrailingJunk(t *testing.T) {
	dir := t.TempDir()
	junky := filepath.Join(dir, "junky.mkv")
	damaged := filepath.Join(dir, "damaged.mkv")
	healthy := filepath.Join(dir, "ok.mkv")
	for _, p := range []string{junky, damaged, healthy} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{junky: true, damaged: true, healthy: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "Junky", Path: junky},
		{MediaID: "m2", Title: "Damaged", Path: damaged},
		{MediaID: "m3", Title: "Ok", Path: healthy},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithDiagnoser(fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
			junky:   {Findings: []mkv.DiagnosisFinding{{Kind: "trailing-junk", Detail: "8192 byte(s) beyond the declared Segment end"}}},
			damaged: {Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, DamagedBytes: 4096},
			healthy: {Healthy: true},
		}})

	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}
	assert.True(t, byID["m1"].TrailingJunk, "surplus tail bytes must raise trailing-junk")
	assert.True(t, byID["m1"].HasIndex, "trailing junk carries a valid index - nothing to reindex")
	assert.Zero(t, byID["m1"].DamagedBytes, "trailing junk is not damage")
	assert.False(t, byID["m2"].TrailingJunk, "a mid-Segment damaged file is not trailing junk")
	assert.Equal(t, 1, res.TrailingJunk, "only the junky file counts")
	assert.Equal(t, 1, res.Damaged, "the damaged file counts on its own axis")
	assert.Empty(t, reidx.reindexed, "a scan never reindexes")
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

// The combined "repair all" job reindexes un-indexed files AND remuxes
// audio-delayed files in a single pass when includeAudio is set; with it
// unset it behaves exactly like a reindex batch. A file carrying BOTH defects
// is only reindexed this run (its true original backup must not be clobbered
// by a second swap) - its audio is caught on the next run.
func TestSeekIndexService_RepairLibrary_Combined(t *testing.T) {
	dir := t.TempDir()
	noIndex := filepath.Join(dir, "noindex.mkv") // reindex only
	delayed := filepath.Join(dir, "delayed.mkv") // remux only
	both := filepath.Join(dir, "both.mkv")       // reindex this run, audio deferred
	healthy := filepath.Join(dir, "healthy.mkv") // untouched
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
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{delayed: 0.9, both: 0.9}})
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 2, st.Last.Total, "the 2 reindex candidates; the audio-only file is not a candidate at all")
	assert.Zero(t, st.Last.Failed)
	assert.ElementsMatch(t, []string{noIndex, both}, reidx.reindexed, "both un-indexed files reindexed - and nothing else was rewritten")

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Zero(t, res.MissingIndex)
}

// The combined entry point must NEVER rewrite a file for an audio start
// offset. It used to, "without being asked" - and that is precisely how a
// deliberate 12.000 s dub alignment was erased in prod (2026-07-13): the
// threshold had no upper bound, so an intentional offset looked exactly like a
// mux slip. The offset is measured and shown; the file is not touched.
func TestSeekIndexService_RepairLibrary_AudioIsNeverRewritten(t *testing.T) {
	dir := t.TempDir()
	noIndex := filepath.Join(dir, "n.mkv")
	delayed := filepath.Join(dir, "d.mkv")
	for _, p := range []string{noIndex, delayed} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{noIndex: false, delayed: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: noIndex}, {MediaID: "m2", Path: delayed}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{delayed: 0.9}})
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 1, st.Last.Total, "the index only - the offset file is not a candidate")
	assert.Equal(t, []string{noIndex}, reidx.reindexed, "no bulk job may rewrite a file to move its audio")
}

// A bare service (no optional prober wired at all) still runs a reindex batch.
func TestSeekIndexService_RepairLibrary_BareService(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "n.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: false}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, []string{p}, reidx.reindexed)
}

// A file with a HEALTHY index but real mid-file corruption (Diagnose already
// proved it: DamagedBytes > 0) is unplayable in practice (mkvgo's PlanHLS
// refuses to synthesize a plan for it) yet was never selected by ANY repair
// batch, because selection only ever looked at HasIndex - DamagedBytes/
// Truncated were reporting-only. Resync mode is the operator's explicit
// consent to trade bytes for playability (same bar as everywhere else this
// repair runs), so it - and only it - now also picks up these files. The
// default (non-Resync) batch must stay exactly as safe as before: it never
// touches a file whose index is already fine, whatever else is wrong with it.
func TestSeekIndexService_RepairLibrary_ResyncIncludesDamagedIndexedFiles(t *testing.T) {
	dir := t.TempDir()
	damaged := filepath.Join(dir, "damaged.mkv")     // healthy index, mid-file corruption
	truncated := filepath.Join(dir, "truncated.mkv") // healthy index, damage runs to EOF
	healthy := filepath.Join(dir, "healthy.mkv")
	for _, p := range []string{damaged, truncated, healthy} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{damaged: true, truncated: true, healthy: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: damaged}, {MediaID: "m2", Path: truncated}, {MediaID: "m3", Path: healthy},
	}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
		damaged:   {Healthy: false, Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, DamagedBytes: 4096},
		truncated: {Healthy: false, Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, DamagedBytes: 4096, Truncated: true},
		healthy:   {Healthy: true},
	}}

	// Default (non-Resync) repair: never touches an already-indexed file,
	// damaged or not - the existing, always-safe behavior must not regress.
	svcDefault := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithDiagnoser(diag)
	require.NoError(t, svcDefault.StartScan(context.Background(), ""))
	waitIdle(t, svcDefault)
	require.NoError(t, svcDefault.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svcDefault)
	assert.Empty(t, reidx.reindexed, "non-Resync repair must not touch an already-indexed file")

	// Explicit Resync repair: the operator has opted into trading bytes for
	// playability - the damaged-but-indexed file is now in scope. The
	// truncated file stays out: its tail is unrecoverably gone, re-acquiring
	// the source is the only real fix (existing, unchanged philosophy).
	reidx2 := &fakeReindexer{hasIndex: map[string]bool{damaged: true, truncated: true, healthy: true}}
	cat2 := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: damaged}, {MediaID: "m2", Path: truncated}, {MediaID: "m3", Path: healthy},
	}}
	svcResync := appmaintenance.NewSeekIndexService(reidx2, cat2, nil).WithDiagnoser(diag)
	require.NoError(t, svcResync.StartScan(context.Background(), ""))
	waitIdle(t, svcResync)
	require.NoError(t, svcResync.StartRepairLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{Resync: true}))
	waitIdle(t, svcResync)
	assert.Equal(t, []string{damaged}, reidx2.reindexed,
		"Resync repair must pick up the damaged-but-indexed file, and only that one")
}

// A truncated file with NO index at all (missing tail AND no Cues - a real
// combination: found against an actual truncated file in the library) must
// stay out of the batch just as much as a truncated-but-indexed one. The old
// selection (`!HasIndex` alone, no Truncated check) picked it up regardless,
// and a real repair attempt on one failed a verify step ("cue mismatch") -
// harmless (the file is left intact) but a spurious failure for something
// only re-acquiring the source can actually fix.
func TestSeekIndexService_RepairLibrary_ExcludesTruncatedEvenWithoutIndex(t *testing.T) {
	dir := t.TempDir()
	truncNoIndex := filepath.Join(dir, "trunc_noindex.mkv")
	healthy := filepath.Join(dir, "healthy.mkv")
	for _, p := range []string{truncNoIndex, healthy} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{truncNoIndex: false, healthy: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: truncNoIndex}, {MediaID: "m2", Path: healthy},
	}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
		truncNoIndex: {Findings: []mkv.DiagnosisFinding{{Kind: "no-index"}, {Kind: "damaged"}}, DamagedBytes: 16, Truncated: true},
		healthy:      {Healthy: true},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithDiagnoser(diag)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	// Both modes: a truncated file is never a candidate, index or no index.
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)
	assert.Empty(t, reidx.reindexed, "truncated-without-index must not be reindexed even in the default mode")

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{Resync: true}))
	waitIdle(t, svc)
	assert.Empty(t, reidx.reindexed, "truncated-without-index must not be reindexed under Resync either")
}

// A "repair all" that does NOT keep backups (a rolling window) must leave zero
// backup behind - and must not have rewritten the audio-offset file at all.
func TestSeekIndexService_RepairLibrary_BackupsHonorWindow(t *testing.T) {
	dir := t.TempDir()
	noIndex := filepath.Join(dir, "n.mkv")
	delayed := filepath.Join(dir, "d.mkv")
	for _, p := range []string{noIndex, delayed} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	shared := map[string]bool{}
	reidx := &fakeReindexer{hasIndex: map[string]bool{noIndex: false, delayed: true}, hasBackup: shared}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: noIndex}, {MediaID: "m2", Path: delayed}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).
		WithAudioDelayProber(fakeAudioDelayProber{delays: map[string]float64{delayed: 0.9}})
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)

	// BackupWindow > 0 == "don't keep backups": rolling + drain.
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "",
		appmaintenance.ReindexRunOptions{BackupWindow: 2}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 1, st.Last.Total, "the reindex only - the offset file was left alone")
	assert.Zero(t, st.Last.Failed)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	for _, it := range res.Items {
		assert.False(t, it.HasBackup, "no backup survives the drain: %s", it.Path)
	}
	assert.Zero(t, res.WithBackup)
}

// An acknowledgement posted WHILE a scan runs (hours on a NAS) must survive
// the scan's completion: the scan reuses statuses from a snapshot taken at
// its start, so without a merge step its final write silently discarded every
// ack that landed in the meantime (audit 2026-09-01).
func TestSeekIndexService_AckDuringScan_Survives(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.mkv")
	slow := filepath.Join(dir, "slow.mkv")
	for _, p := range []string{junk, slow} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{junk: true, slow: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m-junk", Title: "Junk", Path: junk},
		{MediaID: "m-slow", Title: "Slow", Path: slow},
	}}
	gate := newScanGate(slow)
	diag := fakeDiagnoser{gate: gate, byPath: map[string]*mkv.Diagnosis{
		junk: {Findings: []mkv.DiagnosisFinding{{Kind: "trailing-junk"}}},
		slow: {Healthy: true},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithDiagnoser(diag).WithStateDir(t.TempDir())
	ctx := context.Background()

	require.NoError(t, svc.StartScan(ctx, ""))
	waitIdle(t, svc)

	// The second scan re-diagnoses the touched file and parks on it.
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(slow, future, future))
	gate.armed.Store(true)
	require.NoError(t, svc.StartScan(ctx, ""))
	select {
	case <-gate.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the scan never reached the gated file")
	}

	// The operator acknowledges the junk file's verdict while the scan runs.
	require.NoError(t, svc.AckMedia(ctx, "m-junk", true))

	close(gate.release)
	waitIdle(t, svc)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Equal(t, 1, res.Acked, "the ack posted during the scan must survive its completion")
	for _, it := range res.Items {
		if it.Path == junk {
			assert.True(t, it.Acked(), "junk stays acknowledged")
		}
	}
}

// The carry-over is for the SAME bytes only: a file that changed during the
// scan (touched, re-downloaded) is diagnosed afresh and its old
// acknowledgement must not follow it - the operator acknowledged a verdict
// about other bytes.
func TestSeekIndexService_AckDuringScan_DroppedWhenTheFileChanged(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "junk.mkv")
	slow := filepath.Join(dir, "slow.mkv")
	for _, p := range []string{junk, slow} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	reidx := &fakeReindexer{hasIndex: map[string]bool{junk: true, slow: true}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m-junk", Title: "Junk", Path: junk},
		{MediaID: "m-slow", Title: "Slow", Path: slow},
	}}
	gate := newScanGate(slow)
	diag := fakeDiagnoser{gate: gate, byPath: map[string]*mkv.Diagnosis{
		junk: {Findings: []mkv.DiagnosisFinding{{Kind: "trailing-junk"}}},
		slow: {Healthy: true},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil).WithDiagnoser(diag).WithStateDir(t.TempDir())
	ctx := context.Background()

	require.NoError(t, svc.StartScan(ctx, ""))
	waitIdle(t, svc)

	// Both files change: junk is re-diagnosed by this scan (same finding, new
	// bytes), slow parks the scan so the ack can land in the meantime.
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.WriteFile(junk, []byte("xy"), 0o644))
	require.NoError(t, os.Chtimes(junk, future, future))
	require.NoError(t, os.Chtimes(slow, future, future))
	gate.armed.Store(true)
	require.NoError(t, svc.StartScan(ctx, ""))
	select {
	case <-gate.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the scan never reached the gated file")
	}
	require.NoError(t, svc.AckMedia(ctx, "m-junk", true))
	close(gate.release)
	waitIdle(t, svc)

	res, _, ok := svc.CachedScan("")
	require.True(t, ok)
	assert.Zero(t, res.Acked, "an ack about the old bytes must not follow the changed file")
	for _, it := range res.Items {
		if it.Path == junk {
			assert.False(t, it.Acked())
		}
	}
}
