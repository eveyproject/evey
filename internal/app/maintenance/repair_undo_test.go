package maintenance_test

// Undoing a repair from the journal alone - no copy of the original kept.
//
// The bargain: duplicating multi-gigabyte files for every repair is not worth
// it, so instead every transformation is written down precisely enough to be
// replayed backwards. A retime moved tracks by a known amount; the opposite
// shift restores them exactly, and losslessly, whatever the codec.
//
// The one repair that stays outside this bargain is the resync: it discards
// bytes, and no record brings those back.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
)

type fakeRetimer struct {
	mu       sync.Mutex
	calls    []map[uint64]float64
	paths    []string
	fail     error
	failNth  int
	failWith error
}

func (f *fakeRetimer) RetimeTracks(_ context.Context, path string, shift map[uint64]float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, path)
	f.calls = append(f.calls, shift)
	// failNth lets a test fail ONE call and not the others - the case that
	// matters is the second one, where the shift went through and putting the
	// file back did not.
	if f.failNth > 0 && len(f.calls) == f.failNth {
		return f.failWith
	}
	return f.fail
}

// readableJournal serves a scripted history and records what gets written back.
type readableJournal struct {
	mu      sync.Mutex
	history []appmaintenance.RepairRecord
	written []appmaintenance.RepairRecord
}

func (j *readableJournal) RecordRepair(_ context.Context, rec appmaintenance.RepairRecord) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.written = append(j.written, rec)
	return nil
}

func (j *readableJournal) RepairHistory(_ context.Context, _ string, _ int) ([]appmaintenance.RepairRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]appmaintenance.RepairRecord(nil), j.history...), nil
}

func retimeEntry(path string, shift map[uint64]float64) appmaintenance.RepairRecord {
	return appmaintenance.RepairRecord{
		Schema: appmaintenance.RepairJournalSchema, Path: path, MediaID: "m1",
		Op: appmaintenance.RepairAudioRetime, Outcome: "applied",
		ShiftSec: shift, Started: time.Now(),
	}
}

func undoService(j appmaintenance.RepairJournal, r appmaintenance.TrackRetimer) *appmaintenance.SeekIndexService {
	svc := appmaintenance.NewSeekIndexService(&fakeReindexer{hasIndex: map[string]bool{}},
		fakeCatalog{}, nil).WithRepairJournal(j)
	if r != nil {
		svc = svc.WithTrackRetimer(r)
	}
	return svc
}

// The heart of it: a shift of +2.538s is undone by -2.538s, from the entry
// alone, with no backup anywhere.
func TestUndo_ReplaysTheOppositeShift(t *testing.T) {
	j := &readableJournal{history: []appmaintenance.RepairRecord{
		retimeEntry("/media/films/x.mkv", map[uint64]float64{2: 2.538}),
	}}
	r := &fakeRetimer{}
	svc := undoService(j, r)

	require.NoError(t, svc.UndoLastRepair(context.Background(), "/media/films/x.mkv"))

	require.Len(t, r.calls, 1)
	assert.Equal(t, "/media/films/x.mkv", r.paths[0])
	assert.InDelta(t, -2.538, r.calls[0][2], 1e-9, "the inverse of the recorded shift, exactly")
}

// Undoing is itself a repair: it leaves its own entry, so the history reads as
// a chain instead of a file mysteriously changing twice.
func TestUndo_IsItselfJournalled(t *testing.T) {
	j := &readableJournal{history: []appmaintenance.RepairRecord{
		retimeEntry("/media/films/x.mkv", map[uint64]float64{2: 2.538}),
	}}
	svc := undoService(j, &fakeRetimer{})

	require.NoError(t, svc.UndoLastRepair(context.Background(), "/media/films/x.mkv"))

	require.Len(t, j.written, 1)
	e := j.written[0]
	assert.Equal(t, appmaintenance.RepairAudioRetime, e.Op)
	assert.Equal(t, "applied", e.Outcome)
	assert.InDelta(t, -2.538, e.ShiftSec[2], 1e-9)
	assert.Equal(t, "operator-undo", e.Evidence.Method)
}

// A file whose last repair discarded bytes cannot be undone in software, and
// the refusal says so plainly instead of pretending to work.
func TestUndo_RefusesWhatCannotBeReplayed(t *testing.T) {
	j := &readableJournal{history: []appmaintenance.RepairRecord{{
		Schema: appmaintenance.RepairJournalSchema, Path: "/media/films/x.mkv",
		Op: appmaintenance.RepairResync, Outcome: "applied", Started: time.Now(),
	}}}
	svc := undoService(j, &fakeRetimer{})

	err := svc.UndoLastRepair(context.Background(), "/media/films/x.mkv")
	require.Error(t, err)
	assert.ErrorIs(t, err, appmaintenance.ErrNotReversible)
}

// An entry written by a FUTURE build, whose shape this code does not know, is
// refused rather than half-understood. The journal exists to be read years
// later; guessing at an unknown payload is how a rollback corrupts a file.
func TestUndo_RefusesAnUnknownSchema(t *testing.T) {
	rec := retimeEntry("/media/films/x.mkv", map[uint64]float64{2: 1})
	rec.Schema = appmaintenance.RepairJournalSchema + 1
	j := &readableJournal{history: []appmaintenance.RepairRecord{rec}}
	svc := undoService(j, &fakeRetimer{})

	err := svc.UndoLastRepair(context.Background(), "/media/films/x.mkv")
	require.Error(t, err)
	assert.ErrorIs(t, err, appmaintenance.ErrJournalSchema)
}

// Without the retiming primitive there is no software undo, and the caller is
// told so instead of getting a silent no-op.
func TestUndo_WithoutTheRetimerSaysSo(t *testing.T) {
	j := &readableJournal{history: []appmaintenance.RepairRecord{
		retimeEntry("/media/films/x.mkv", map[uint64]float64{2: 2.538}),
	}}
	svc := undoService(j, nil)

	err := svc.UndoLastRepair(context.Background(), "/media/films/x.mkv")
	require.Error(t, err)
	assert.ErrorIs(t, err, appmaintenance.ErrNotReversible)
}

// A failed undo is not recorded as a success: the history must never claim a
// file was restored when it was not.
func TestUndo_FailureIsNotRecordedAsApplied(t *testing.T) {
	j := &readableJournal{history: []appmaintenance.RepairRecord{
		retimeEntry("/media/films/x.mkv", map[uint64]float64{2: 2.538}),
	}}
	svc := undoService(j, &fakeRetimer{fail: errors.New("read-only volume")})

	require.Error(t, svc.UndoLastRepair(context.Background(), "/media/films/x.mkv"))
	require.Len(t, j.written, 1)
	assert.Equal(t, "failed", j.written[0].Outcome)
	assert.Contains(t, j.written[0].Error, "read-only volume")
}

// Nothing to undo is a clean answer, not an error the UI has to interpret.
func TestUndo_NothingToUndo(t *testing.T) {
	j := &readableJournal{}
	svc := undoService(j, &fakeRetimer{})

	err := svc.UndoLastRepair(context.Background(), "/media/films/x.mkv")
	assert.ErrorIs(t, err, appmaintenance.ErrNoRepairToUndo)
}
