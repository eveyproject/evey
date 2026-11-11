package maintenance_test

// The preview exists because no measurement on this server can say whether the
// sound matches the PICTURE. These tests pin the two things the code decides on
// its own: what a request is clamped to, and that a shift applied by ear is
// recorded as a person's judgement rather than dressed up as a measurement.

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

	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/domain/mkv"
)

type fakePreview struct {
	mu   sync.Mutex
	got  []appmaintenance.SyncPreviewRequest
	fail error
}

func (f *fakePreview) RenderSyncPreview(_ context.Context, req appmaintenance.SyncPreviewRequest) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, req)
	if f.fail != nil {
		return nil, f.fail
	}
	return []byte("mp4"), nil
}

func previewService(t *testing.T, p appmaintenance.SyncPreviewRenderer, r appmaintenance.TrackRetimer, j appmaintenance.RepairJournal) (string, *appmaintenance.SeekIndexService) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "film.mkv")
	require.NoError(t, os.WriteFile(path, []byte("bytes"), 0o644))
	svc := appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{}},
		fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "Film", Path: path}}}, nil)
	if p != nil {
		svc = svc.WithSyncPreviewRenderer(p)
	}
	if r != nil {
		svc = svc.WithTrackRetimer(r)
	}
	if j != nil {
		svc = svc.WithRepairJournal(j)
	}
	return path, svc
}

// A clip is bounded: an operator dragging a control must not be able to ask for
// the whole film, nor for a shift that stopped being a shift.
func TestSyncPreview_ClampsWhatItWillRender(t *testing.T) {
	p := &fakePreview{}
	path, svc := previewService(t, p, nil, nil)

	_, err := svc.SyncPreviewOfMedia(context.Background(), "m1", appmaintenance.SyncPreviewRequest{
		AtSec: 600, LenSec: 9999, Track: -3, ShiftSec: 500,
	})
	require.NoError(t, err)

	require.Len(t, p.got, 1)
	got := p.got[0]
	assert.Equal(t, path, got.Path)
	assert.InDelta(t, appmaintenance.SyncPreviewMaxSec, got.LenSec, 0.001)
	assert.Equal(t, 0, got.Track)
	assert.InDelta(t, appmaintenance.SyncPreviewMaxShiftSec, got.ShiftSec, 0.001)
}

// Delaying the sound means reading it from earlier in the film. Asked near the
// opening, that window would start before the film does - so the clip moves
// forward rather than failing at the operator.
func TestSyncPreview_MovesTheWindowInsteadOfRefusingIt(t *testing.T) {
	p := &fakePreview{}
	_, svc := previewService(t, p, nil, nil)

	_, err := svc.SyncPreviewOfMedia(context.Background(), "m1", appmaintenance.SyncPreviewRequest{
		AtSec: 2, LenSec: 10, ShiftSec: 12,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, p.got[0].AtSec, p.got[0].ShiftSec)
}

// Without a renderer the caller is told, instead of receiving an empty video.
func TestSyncPreview_SaysWhenItCannotRender(t *testing.T) {
	_, svc := previewService(t, nil, nil, nil)
	_, err := svc.SyncPreviewOfMedia(context.Background(), "m1", appmaintenance.SyncPreviewRequest{AtSec: 10})
	assert.ErrorIs(t, err, appmaintenance.ErrPreviewUnavailable)
}

// The point of the whole feature: a number found by ear is applied, and the
// journal says it came from a person. That is stronger evidence than the
// declared offset which caused the incident, and the entry must not blur them.
func TestOperatorShift_IsAppliedAndRecordedAsAJudgement(t *testing.T) {
	r := &fakeRetimer{}
	j := &fakeJournal{}
	_, svc := previewService(t, nil, r, j)

	require.NoError(t, svc.ApplyOperatorShift(context.Background(), "m1", 2, 2.5))

	require.Len(t, r.calls, 1)
	assert.InDelta(t, 2.5, r.calls[0][2], 1e-9)
	entries := j.all()
	require.Len(t, entries, 1)
	assert.Equal(t, appmaintenance.RepairAudioRetime, entries[0].Op)
	assert.Equal(t, "applied", entries[0].Outcome)
	assert.Equal(t, "operator", entries[0].Evidence.Method)
	assert.True(t, entries[0].Reversible(), "an operator's shift must be undoable like any other")
}

// A shift nobody could have judged by ear is refused before it touches a file.
func TestOperatorShift_RefusesAnAbsurdAmount(t *testing.T) {
	r := &fakeRetimer{}
	_, svc := previewService(t, nil, r, &fakeJournal{})

	err := svc.ApplyOperatorShift(context.Background(), "m1", 2, 3600)
	require.ErrorIs(t, err, appmaintenance.ErrShiftOutOfRange, "the API maps this to the caller's fault, not a server fault")
	require.ErrorIs(t, svc.ApplyOperatorShift(context.Background(), "m1", 2, -61), appmaintenance.ErrShiftOutOfRange)
	assert.Empty(t, r.calls, "nothing may be written for a value out of range")
}

// A failed shift is recorded as failed. The history must never claim a file was
// moved when it was not.
func TestOperatorShift_FailureIsRecorded(t *testing.T) {
	r := &fakeRetimer{fail: errors.New("read-only volume")}
	j := &fakeJournal{}
	_, svc := previewService(t, nil, r, j)

	require.Error(t, svc.ApplyOperatorShift(context.Background(), "m1", 2, 1.0))
	require.Len(t, j.all(), 1)
	assert.Equal(t, "failed", j.all()[0].Outcome)
}

// The journal is the answer to 2026-07-13: seven weeks later, the only
// question that mattered was "what state is this file in". "failed" and
// "rolled_back" answer it differently - one says the file may be anything, the
// other says it is exactly what it was - and the operator path was writing the
// wrong one. It tested for ErrAudioLost, the SAMPLED guard's sentinel, while a
// retime is judged by the EXHAUSTIVE one and can only ever return
// ErrAudioIntegrityLost. So every restored retime was filed as a failure.
func TestOperatorShift_ARestoredFileIsRecordedAsRolledBack(t *testing.T) {
	r := &fakeRetimer{}
	j := &fakeJournal{}
	_, svc := previewService(t, nil, r, j)
	svc = svc.WithAudioIntegrityProber(&fakeIntegrityProber{answers: [][]mkv.TrackIntegrity{
		{sound(0, 6000, 2, 1)},
		{sound(0, 6000, 2, 2)}, // the frames now change format mid-track
	}})

	err := svc.ApplyOperatorShift(context.Background(), "m1", 2, 2.5)

	require.ErrorIs(t, err, appmaintenance.ErrAudioIntegrityLost)
	require.Len(t, r.calls, 2, "the shift, then its inverse to put the file back")
	require.Len(t, j.all(), 1)
	assert.Equal(t, "rolled_back", j.all()[0].Outcome)
}

// The worst outcome of all, and the one the ledger must never dress up: the
// sound was damaged AND putting the original back failed. The file on disk is
// the damaged one. Calling that "rolled_back" would send a reader looking for
// a file that no longer exists.
func TestOperatorShift_AFailedRestoreIsNotARollback(t *testing.T) {
	r := &fakeRetimer{failNth: 2, failWith: errors.New("read-only volume")}
	j := &fakeJournal{}
	_, svc := previewService(t, nil, r, j)
	svc = svc.WithAudioIntegrityProber(&fakeIntegrityProber{answers: [][]mkv.TrackIntegrity{
		{sound(0, 6000, 2, 1)},
		{sound(0, 6000, 2, 2)},
	}})

	err := svc.ApplyOperatorShift(context.Background(), "m1", 2, 2.5)

	require.ErrorIs(t, err, appmaintenance.ErrAudioIntegrityLost)
	require.Len(t, j.all(), 1)
	assert.Equal(t, "failed", j.all()[0].Outcome,
		"the restore failed, so the damaged file is still there")
}

// A shift applied by hand rewrites a file on the library disk like any batch
// repair does: it takes the single job slot, so it cannot run beside a batch
// that may be rewriting the same file.
func TestOperatorShift_RefusedWhileAJobRuns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "film.mkv")
	require.NoError(t, os.WriteFile(path, []byte("bytes"), 0o644))
	gate := make(chan struct{})
	reidx := &fakeReindexer{hasIndex: map[string]bool{}, gate: gate}
	r := &fakeRetimer{}
	j := &fakeJournal{}
	svc := appmaintenance.NewSeekIndexService(reidx,
		fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Title: "Film", Path: path}}}, nil).
		WithTrackRetimer(r).WithRepairJournal(j)
	require.NoError(t, svc.StartReindexLibrary(t.Context(), "lib1", appmaintenance.ReindexRunOptions{}))
	require.Eventually(t, func() bool {
		return reidx.reindexCount() >= 1 || svc.Status().Phase == appmaintenance.PhaseProcessing
	}, 5*time.Second, 10*time.Millisecond, "batch never reached the processing phase")

	err := svc.ApplyOperatorShift(context.Background(), "m1", 2, 2.5)

	require.ErrorIs(t, err, appmaintenance.ErrJobRunning)
	assert.Empty(t, r.calls, "nothing may be written beside a running job")
	assert.Empty(t, j.all(), "nothing happened, nothing is journalled")
	close(gate)
	waitIdle(t, svc)
}

// A rewrite by hand is refused while someone is watching: a batch would wait
// for an idle server between files, a request cannot. Once playback stops
// the same request goes through.
func TestOperatorShift_RefusedWhilePlaybackIsActive(t *testing.T) {
	r := &fakeRetimer{}
	j := &fakeJournal{}
	_, svc := previewService(t, nil, r, j)
	streams := &countingStreams{}
	streams.active.Store(1)
	svc = svc.WithActiveStreamChecker(streams)

	err := svc.ApplyOperatorShift(context.Background(), "m1", 2, 2.5)

	require.ErrorIs(t, err, appmaintenance.ErrPlaybackActive)
	assert.Empty(t, r.calls, "nothing may be written over a viewer")
	assert.Empty(t, j.all(), "nothing happened, nothing is journalled")

	streams.active.Store(0)
	require.NoError(t, svc.ApplyOperatorShift(context.Background(), "m1", 2, 2.5))
	require.Len(t, r.calls, 1)
}

// The shift runs as a job: it is accounted for like a batch, so the status
// page shows what was last done to the disk, and the slot is free afterwards.
func TestOperatorShift_RunsInTheJobSlot(t *testing.T) {
	r := &fakeRetimer{}
	_, svc := previewService(t, nil, r, &fakeJournal{})

	require.NoError(t, svc.ApplyOperatorShift(context.Background(), "m1", 2, 2.5))

	st := svc.Status()
	assert.False(t, st.Running, "a synchronous rewrite releases the slot before returning")
	require.NotNil(t, st.Last)
	assert.Equal(t, appmaintenance.JobAudioRetime, st.Last.Kind)
	assert.Equal(t, 1, st.Last.Done)
	assert.Equal(t, 0, st.Last.Failed)
}

type fakeAudioTracks struct{ numbers []uint64 }

func (f fakeAudioTracks) AudioTrackNumbers(context.Context, string) ([]uint64, error) {
	return f.numbers, nil
}

// The preview plays audio index N; the shift must move that same track. On a
// dub laid out audio, subtitles, picture, audio, the second audio track is
// Matroska track 5 - "index + 1" would have retimed a subtitle.
func TestOperatorShiftToAudio_MovesTheTrackThePreviewPlayed(t *testing.T) {
	r := &fakeRetimer{}
	_, svc := previewService(t, nil, r, &fakeJournal{})
	svc = svc.WithAudioTrackLister(fakeAudioTracks{numbers: []uint64{1, 5}})

	require.NoError(t, svc.ApplyOperatorShiftToAudio(context.Background(), "m1", 1, 2.5))

	require.Len(t, r.calls, 1)
	assert.Equal(t, map[uint64]float64{5: 2.5}, r.calls[0])
}

func TestOperatorShiftToAudio_RefusesATrackTheFileDoesNotHave(t *testing.T) {
	r := &fakeRetimer{}
	j := &fakeJournal{}
	_, svc := previewService(t, nil, r, j)
	svc = svc.WithAudioTrackLister(fakeAudioTracks{numbers: []uint64{1, 5}})

	require.ErrorIs(t, svc.ApplyOperatorShiftToAudio(context.Background(), "m1", 2, 2.5), appmaintenance.ErrAudioTrackMissing)
	require.ErrorIs(t, svc.ApplyOperatorShiftToAudio(context.Background(), "m1", -1, 2.5), appmaintenance.ErrAudioTrackMissing)
	assert.Empty(t, r.calls, "nothing may be written for a track that does not exist")
	assert.Empty(t, j.all())
}

func TestOperatorShiftToAudio_WithoutALookupSaysSo(t *testing.T) {
	r := &fakeRetimer{}
	_, svc := previewService(t, nil, r, &fakeJournal{})

	require.ErrorIs(t, svc.ApplyOperatorShiftToAudio(context.Background(), "m1", 0, 2.5), appmaintenance.ErrAudioTracksUnavailable)
	assert.Empty(t, r.calls)
}
