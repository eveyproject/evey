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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
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

	require.Error(t, svc.ApplyOperatorShift(context.Background(), "m1", 2, 3600))
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
