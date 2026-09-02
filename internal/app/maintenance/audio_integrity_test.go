package maintenance_test

// A rewrite that MOVES or DROPS audio bytes is judged on the whole track, by
// comparison with the same measurement taken before it. The two failures the
// sampled guard cannot see (prod, 2026-09-01) are frozen here: a hole between
// its windows, and a frame-format change that decodes without an error and
// plays silent.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
)

// fakeIntegrityProber answers scripted measurements in call order, so a test
// can say "this before the rewrite, that after".
type fakeIntegrityProber struct {
	mu      sync.Mutex
	answers [][]mkv.TrackIntegrity
	errs    []error
	calls   int
}

func (f *fakeIntegrityProber) ProbeAudioIntegrity(_ context.Context, _ string) ([]mkv.TrackIntegrity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	}
	if i < len(f.answers) {
		return f.answers[i], err
	}
	return nil, err
}

func sound(track int, audible float64, stretches, frameSizes int) mkv.TrackIntegrity {
	return mkv.TrackIntegrity{Track: track, Codec: "ac3", AudibleSec: audible, SilentStretches: stretches, FrameSizes: frameSizes}
}

// resyncLibrary builds a one-file library whose defect is mid-file damage, so
// "repair with consent" means one resync rewrite and nothing else.
func resyncLibrary(t *testing.T, prober appmaintenance.AudioIntegrityProber) (string, *appmaintenance.SeekIndexService, *fakeReindexer) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "movie.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{p: true}, hasBackup: map[string]bool{}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
		p: {Findings: []mkv.DiagnosisFinding{{Kind: "damaged"}}, DamagedBytes: 4096},
	}}
	reidx.onReindexed = func(path string) { diag.byPath[path] = &mkv.Diagnosis{Healthy: true} }
	svc := appmaintenance.NewSeekIndexService(reidx,
		fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "Movie", Path: p}}}, nil).
		WithDiagnoser(diag).
		WithAudioIntegrityProber(prober)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	return p, svc, reidx
}

// The comparison rule, case by case. Nothing is inferred from the AFTER
// measurement alone: a film that was silent for ten minutes may stay so.
func TestAudioIntegrityLost(t *testing.T) {
	cases := map[string]struct {
		before, after []mkv.TrackIntegrity
		lost          bool
	}{
		"unchanged":                   {[]mkv.TrackIntegrity{sound(0, 6000, 3, 1)}, []mkv.TrackIntegrity{sound(0, 6000, 3, 1)}, false},
		"a new hole":                  {[]mkv.TrackIntegrity{sound(0, 6000, 3, 1)}, []mkv.TrackIntegrity{sound(0, 5700, 4, 1)}, true},
		"frame format now changes":    {[]mkv.TrackIntegrity{sound(0, 6000, 3, 1)}, []mkv.TrackIntegrity{sound(0, 6000, 3, 2)}, true},
		"lost audible time, no hole":  {[]mkv.TrackIntegrity{sound(0, 6000, 3, 1)}, []mkv.TrackIntegrity{sound(0, 5990, 3, 1)}, true},
		"within slack":                {[]mkv.TrackIntegrity{sound(0, 6000, 3, 1)}, []mkv.TrackIntegrity{sound(0, 5998, 3, 1)}, false},
		"a track is gone":             {[]mkv.TrackIntegrity{sound(0, 6000, 3, 1), sound(1, 6000, 3, 1)}, []mkv.TrackIntegrity{sound(0, 6000, 3, 1)}, true},
		"variable-frame codec before": {[]mkv.TrackIntegrity{sound(0, 6000, 3, 5)}, []mkv.TrackIntegrity{sound(0, 6000, 3, 7)}, false},
		"silent film stays silent":    {[]mkv.TrackIntegrity{sound(0, 0, 1, 1)}, []mkv.TrackIntegrity{sound(0, 0, 1, 1)}, false},
		"fewer holes is not a loss":   {[]mkv.TrackIntegrity{sound(0, 6000, 4, 1)}, []mkv.TrackIntegrity{sound(0, 6100, 3, 1)}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := appmaintenance.AudioIntegrityLostForTest(tc.before, tc.after)
			if tc.lost {
				require.ErrorIs(t, err, appmaintenance.ErrAudioIntegrityLost)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// A resync that opens a hole in a track is undone from its backup, and the
// journal says so.
func TestResync_HoleIsRolledBack(t *testing.T) {
	prober := &fakeIntegrityProber{answers: [][]mkv.TrackIntegrity{
		{sound(0, 6000, 2, 1)}, // before
		{sound(0, 5400, 3, 1)}, // after: a ten-minute hole
	}}
	p, svc, reidx := resyncLibrary(t, prober)
	j := &fakeJournal{}
	svc.WithRepairJournal(j)

	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{ResyncDamaged: true, BackupWindow: 1}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Equal(t, 1, st.Last.Failed, "the file is reported as failed, not silently kept")
	assert.Equal(t, []string{p}, reidx.restored, "the original went back")
	assert.Equal(t, 2, prober.calls, "measured before and after")

	entries := j.all()
	require.Len(t, entries, 1)
	assert.Equal(t, "rolled_back", entries[0].Outcome)
	assert.Contains(t, entries[0].Error, "silent stretches")
}

// A resync that leaves every track as it was stands, and the guard costs
// exactly two measurements.
func TestResync_IntactTrackStands(t *testing.T) {
	prober := &fakeIntegrityProber{answers: [][]mkv.TrackIntegrity{
		{sound(0, 6000, 2, 1)},
		{sound(0, 5999, 2, 1)},
	}}
	p, svc, reidx := resyncLibrary(t, prober)
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{ResyncDamaged: true, BackupWindow: 1}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Failed)
	assert.Equal(t, []string{p}, reidx.reindexed)
	assert.Empty(t, reidx.restored)
}

// A prober that cannot measure is no evidence: the resync runs as before and
// nothing is rolled back on a guess.
func TestResync_UnmeasurableIsNotEvidence(t *testing.T) {
	prober := &fakeIntegrityProber{errs: []error{errors.New("ffmpeg gone"), errors.New("ffmpeg gone")}}
	p, svc, reidx := resyncLibrary(t, prober)
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{ResyncDamaged: true, BackupWindow: 1}))
	st := waitIdle(t, svc)
	require.NotNil(t, st.Last)
	assert.Zero(t, st.Last.Failed)
	assert.Equal(t, []string{p}, reidx.reindexed)
	assert.Empty(t, reidx.restored)
}

// An ordinary index rebuild copies the audio untouched and is NOT measured
// end to end: the exhaustive guard costs minutes per file, the sampled one
// covers this case, and a whole-library repair must stay affordable.
func TestReindex_DoesNotPayForTheExhaustiveGuard(t *testing.T) {
	prober := &fakeIntegrityProber{answers: [][]mkv.TrackIntegrity{{sound(0, 6000, 2, 1)}, {sound(0, 6000, 2, 1)}}}
	dir := t.TempDir()
	p := filepath.Join(dir, "movie.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	reidx := &fakeReindexer{hasIndex: map[string]bool{}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{p: {Findings: []mkv.DiagnosisFinding{{Kind: "no-index"}}}}}
	reidx.onReindexed = func(path string) { diag.byPath[path] = &mkv.Diagnosis{Healthy: true} }
	svc := appmaintenance.NewSeekIndexService(reidx, fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}, nil).
		WithDiagnoser(diag).WithAudioIntegrityProber(prober)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	require.NoError(t, svc.StartRepairLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)
	assert.Equal(t, []string{p}, reidx.reindexed)
	assert.Zero(t, prober.calls)
}

// Undoing a retime moves audio on the timeline, so it is measured end to end
// too - and a damaged result is put back by replaying the shift that was being
// undone, which is exact in both directions.
func TestUndo_DamagedResultIsPutBack(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "movie.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	j := &readableJournal{history: []appmaintenance.RepairRecord{retimeEntry(p, map[uint64]float64{2: -2.538})}}
	r := &fakeRetimer{}
	prober := &fakeIntegrityProber{answers: [][]mkv.TrackIntegrity{
		{sound(0, 6000, 2, 1)},
		{sound(0, 6000, 2, 2)}, // the frames now change format mid-track
	}}
	svc := undoService(j, r).WithAudioIntegrityProber(prober)

	err := svc.UndoLastRepair(context.Background(), p)
	require.ErrorIs(t, err, appmaintenance.ErrAudioIntegrityLost)
	require.Len(t, r.calls, 2, "the inverse shift, then the original shift to put it back")
	assert.Equal(t, map[uint64]float64{2: 2.538}, r.calls[0])
	assert.Equal(t, map[uint64]float64{2: -2.538}, r.calls[1])
	require.Len(t, j.written, 1)
	assert.Equal(t, "rolled_back", j.written[0].Outcome)
	assert.WithinDuration(t, time.Now(), j.written[0].Started, time.Minute)
}
