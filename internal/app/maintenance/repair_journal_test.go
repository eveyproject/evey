package maintenance_test

// Every rewrite must leave a trace. The 2026-07-13 batch rewrote 98 files and
// left nothing behind but their mtime, which is why the damage took seven weeks
// to surface and why most of it can never be undone. These tests pin the
// contract: what was done, on what evidence, and enough to reverse it.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
)

type fakeJournal struct {
	mu      sync.Mutex
	entries []appmaintenance.RepairRecord
	fail    error
}

func (f *fakeJournal) RecordRepair(_ context.Context, rec appmaintenance.RepairRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, rec)
	return f.fail
}

func (f *fakeJournal) all() []appmaintenance.RepairRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]appmaintenance.RepairRecord(nil), f.entries...)
}

// journalLibrary builds a one-file library whose only defect is a missing
// index, so a repair means exactly one rewrite and exactly one entry.
func journalLibrary(t *testing.T, j appmaintenance.RepairJournal, prober appmaintenance.AudibleTracksProber) (string, *appmaintenance.SeekIndexService, *fakeReindexer) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "movie.mkv")
	require.NoError(t, os.WriteFile(p, []byte("some bytes"), 0o644))

	reidx := &fakeReindexer{hasIndex: map[string]bool{}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
		p: {Findings: []mkv.DiagnosisFinding{{Kind: "no-index"}}},
	}}
	reidx.onReindexed = func(path string) { diag.byPath[path] = &mkv.Diagnosis{Healthy: true} }

	svc := appmaintenance.NewSeekIndexService(reidx,
		fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "Movie", Path: p}}}, nil).
		WithDiagnoser(diag).
		WithRepairJournal(j)
	if prober != nil {
		svc = svc.WithAudibleTracksProber(prober)
	}
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	return p, svc, reidx
}

// The entry a repair owes: which file, which operation, and the backup that
// makes it reversible.
func TestRepair_IsJournalled(t *testing.T) {
	j := &fakeJournal{}
	p, svc, _ := journalLibrary(t, j, nil)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	entries := j.all()
	require.Len(t, entries, 1, "a rewrite with no entry is exactly what made July unexplainable")
	e := entries[0]
	assert.Equal(t, p, e.Path)
	assert.Equal(t, "m1", e.MediaID)
	assert.Equal(t, appmaintenance.RepairReindex, e.Op)
	assert.Equal(t, "applied", e.Outcome)
	assert.NotEmpty(t, e.BackupPath, "the entry must say where the original went")
	assert.False(t, e.Started.IsZero())
	assert.NotEmpty(t, e.Evidence.Method, "an action with no stated evidence is the July mistake")
}

// A file the repair looked at and left alone is recorded too: "nothing was
// done" is an answer someone will need, and its absence reads as a gap.
func TestRepair_SkippedFileIsJournalledAsSkipped(t *testing.T) {
	j := &fakeJournal{}
	p, svc, reidx := journalLibrary(t, j, nil)
	reidx.skip = map[string]bool{p: true}

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	entries := j.all()
	require.Len(t, entries, 1)
	assert.Equal(t, "skipped", entries[0].Outcome)
	assert.Empty(t, entries[0].BackupPath)
}

// A repair that cost the file its sound is rolled back - and the entry says so,
// with both counts, so the rollback is not something a reader has to infer.
func TestRepair_RollbackIsJournalledWithTheCounts(t *testing.T) {
	j := &fakeJournal{}
	prober := &fakeAudibleProber{counts: []int{2, 0}}
	_, svc, _ := journalLibrary(t, j, prober)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	entries := j.all()
	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, "rolled_back", e.Outcome)
	assert.Equal(t, 2, e.AudibleBefore)
	assert.Equal(t, 0, e.AudibleAfter)
	assert.Contains(t, e.Error, "audible")
}

// A failing repair is recorded with its reason. A batch where half the files
// failed must be readable afterwards without digging through process logs.
func TestRepair_FailureIsJournalled(t *testing.T) {
	j := &fakeJournal{}
	p, svc, reidx := journalLibrary(t, j, nil)
	reidx.failWith = map[string]error{p: errors.New("disk went away")}

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	entries := j.all()
	require.Len(t, entries, 1)
	assert.Equal(t, "failed", entries[0].Outcome)
	assert.Contains(t, entries[0].Error, "disk went away")
}

// The journal must never cost the operator a repair: if it refuses the entry,
// the file is still repaired. The loss is loud in the logs, not in the work.
func TestRepair_JournalFailureDoesNotAbortTheRepair(t *testing.T) {
	j := &fakeJournal{fail: errors.New("database down")}
	_, svc, reidx := journalLibrary(t, j, nil)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	assert.Len(t, reidx.reindexed, 1, "the repair must happen even when the journal is down")
	assert.Empty(t, svc.Status().Last.Failures)
}
