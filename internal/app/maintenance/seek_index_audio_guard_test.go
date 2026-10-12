package maintenance_test

// A repair that rewrites a file must not be allowed to cost the operator a
// soundtrack. On prod (2026-09-01) two films came back from a manual audio
// repair with their French track completely silent, and nothing in the
// pipeline noticed: the rewrite verified STRUCTURE (tracks present, index
// rebuilt, duration coherent) and a silent track satisfies all of that.
//
// The guard these tests pin is the missing question: does the file still have
// as many audible audio tracks as it had before? When the answer is no, the
// backup goes back in place by itself and the file is reported as failed.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/domain/mkv"
)

// errProbeFailed stands for any probe failure: the guard routes on "could not
// measure", never on which tool broke.
var errProbeFailed = errors.New("ffmpeg unavailable")

// fakeAudibleProber answers with a scripted count per call, so a test can say
// "two tracks before the rewrite, none after".
type fakeAudibleProber struct {
	mu     sync.Mutex
	counts []int   // consumed in order
	errs   []error // parallel to counts; nil means no error
	calls  []string
}

func (f *fakeAudibleProber) ProbeAudibleAudioTracks(_ context.Context, path string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, path)
	i := len(f.calls) - 1
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	}
	if i < len(f.counts) {
		return f.counts[i], err
	}
	return 0, err
}

// guardLibrary builds a one-file library whose only defect is a missing index,
// so "repair" means one rewrite and nothing else.
func guardLibrary(t *testing.T, prober appmaintenance.AudibleTracksProber) (string, *appmaintenance.SeekIndexService, *fakeReindexer) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "movie.mkv")
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))

	reidx := &fakeReindexer{hasIndex: map[string]bool{}}
	diag := fakeDiagnoser{byPath: map[string]*mkv.Diagnosis{
		p: {Findings: []mkv.DiagnosisFinding{{Kind: "no-index"}}},
	}}
	reidx.onReindexed = func(path string) { diag.byPath[path] = &mkv.Diagnosis{Healthy: true} }

	svc := appmaintenance.NewSeekIndexService(reidx,
		fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "Movie", Path: p}}}, nil).
		WithDiagnoser(diag).
		WithAudibleTracksProber(prober)
	require.NoError(t, svc.StartScan(context.Background(), ""))
	waitIdle(t, svc)
	return p, svc, reidx
}

// The incident, in one test: the rewrite succeeds, the sound is gone, and the
// operator must end up with the file they had - not with a silent one.
func TestReindex_SilencedTrackIsRolledBackAndReported(t *testing.T) {
	prober := &fakeAudibleProber{counts: []int{2, 0}} // two audible tracks, then none
	p, svc, reidx := guardLibrary(t, prober)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	assert.Equal(t, []string{p}, reidx.restored, "the backup must go back in place, unasked")
	st := svc.Status()
	require.Len(t, st.Last.Failures, 1, "and the file must be reported, not silently counted as repaired")
	assert.Equal(t, p, st.Last.Failures[0].Path)
	assert.Contains(t, st.Last.Failures[0].Error, "audible")
	assert.Equal(t, []string{p, p}, prober.calls, "measured before and after, on the same file")
}

// The ordinary case must stay ordinary: sound before, sound after, no rollback
// and no scary failure line.
func TestReindex_SoundSurvives_NoRollback(t *testing.T) {
	prober := &fakeAudibleProber{counts: []int{2, 2}}
	_, svc, reidx := guardLibrary(t, prober)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	assert.Empty(t, reidx.restored)
	assert.Empty(t, svc.Status().Last.Failures)
}

// A file that had no audible track to begin with (a silent short, a probe that
// found nothing) must not be rolled back for still having none: the guard
// exists to catch a LOSS, never to demand sound the file never had.
func TestReindex_NoAudioToLose_NoRollback(t *testing.T) {
	prober := &fakeAudibleProber{counts: []int{0, 0}}
	_, svc, reidx := guardLibrary(t, prober)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	assert.Empty(t, reidx.restored)
	assert.Empty(t, svc.Status().Last.Failures)
}

// The prober is not the judge of the repair: if it cannot measure the file
// BEFORE, there is no baseline and the repair proceeds unguarded rather than
// being blocked by an ffmpeg hiccup.
func TestReindex_UnmeasurableBefore_RepairProceeds(t *testing.T) {
	prober := &fakeAudibleProber{counts: []int{0, 0}, errs: []error{errProbeFailed, nil}}
	_, svc, reidx := guardLibrary(t, prober)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	assert.Empty(t, reidx.restored)
	assert.Empty(t, svc.Status().Last.Failures)
	assert.Len(t, prober.calls, 1, "no baseline, no second measurement to compare it against")
}

// ...and if it cannot measure the file AFTER, the loss is UNPROVEN: the file
// keeps its backup and the operator is told, but nothing is undone on a guess.
func TestReindex_UnmeasurableAfter_KeepsTheRepairAndWarns(t *testing.T) {
	prober := &fakeAudibleProber{counts: []int{2, 0}, errs: []error{nil, errProbeFailed}}
	_, svc, reidx := guardLibrary(t, prober)

	require.NoError(t, svc.StartReindexLibrary(context.Background(), "", appmaintenance.ReindexRunOptions{}))
	waitIdle(t, svc)

	assert.Empty(t, reidx.restored, "an unreadable measurement is not evidence of a loss")
	assert.Empty(t, svc.Status().Last.Failures)
}
