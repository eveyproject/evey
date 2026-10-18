package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// recordingJournal answers history and list reads with canned records and
// remembers what it was asked, so the RPCs' argument plumbing is observable.
type recordingJournal struct {
	records     []appmaintenance.RepairRecord
	err         error
	historyPath string
	historyLim  int
	listQuery   appmaintenance.RepairQuery
}

func (j *recordingJournal) RecordRepair(context.Context, appmaintenance.RepairRecord) error {
	return nil
}
func (j *recordingJournal) RepairHistory(_ context.Context, path string, limit int) ([]appmaintenance.RepairRecord, error) {
	j.historyPath, j.historyLim = path, limit
	return j.records, j.err
}
func (j *recordingJournal) ListRepairs(_ context.Context, q appmaintenance.RepairQuery) ([]appmaintenance.RepairRecord, error) {
	j.listQuery = q
	return j.records, j.err
}

type erroringCatalog struct {
	fakeSeekCatalog
	err error
}

func (c erroringCatalog) GetMediaFile(context.Context, string) (appmaintenance.MediaRef, error) {
	return appmaintenance.MediaRef{}, c.err
}

func newJournalHandler(j *recordingJournal, catalog appmaintenance.MediaCatalog) *adminMediaHandler {
	svc := appmaintenance.NewSeekIndexService(fakeReindexer{}, catalog, nil).WithRepairJournal(j)
	return &adminMediaHandler{seekIndex: svc}
}

func TestRepairJournalRPCs_NotConfigured(t *testing.T) {
	d := &adminMediaHandler{}
	ctx := context.Background()
	_, err := d.GetMediaRepairHistory(ctx, &eveyv1.GetMediaRepairHistoryRequest{MediaId: "mi-1"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = d.ListMediaRepairs(ctx, &eveyv1.ListMediaRepairsRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = d.AckSeekIndexMedia(ctx, &eveyv1.AckSeekIndexMediaRequest{MediaId: "mi-1"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = d.AckSeekIndexGroup(ctx, &eveyv1.AckSeekIndexGroupRequest{LibraryId: "lib-1"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestGetMediaRepairHistory_ResolvesPathAndMapsEntries(t *testing.T) {
	started := time.Date(2026, 9, 16, 2, 0, 0, 0, time.UTC)
	j := &recordingJournal{records: []appmaintenance.RepairRecord{
		{MediaID: "mi-1", Path: "/media/m.mkv", Op: appmaintenance.RepairReindex, Job: "reindex", Started: started, Outcome: "applied"},
		{MediaID: "mi-1", Path: "/media/m.mkv", Op: appmaintenance.RepairAudioRetime, Started: started.Add(time.Minute), Outcome: "failed", Error: "erofs"},
	}}
	d := newJournalHandler(j, fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}})

	got, err := d.GetMediaRepairHistory(context.Background(), &eveyv1.GetMediaRepairHistoryRequest{MediaId: "mi-1", Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, "/media/m.mkv", j.historyPath, "the journal is keyed by path; the UI only holds the media id")
	assert.Equal(t, 5, j.historyLim)
	require.Len(t, got.GetEntries(), 2)
	assert.Equal(t, started.UnixMilli(), got.GetEntries()[0].GetAtMs())
	assert.Equal(t, "reindex", got.GetEntries()[0].GetJob())
	assert.Equal(t, "applied", got.GetEntries()[0].GetOutcome())
	assert.Equal(t, "erofs", got.GetEntries()[1].GetError())
	assert.Equal(t, "failed", got.GetEntries()[1].GetOutcome())
}

func TestGetMediaRepairHistory_EmptyWhenNoJournal(t *testing.T) {
	svc := appmaintenance.NewSeekIndexService(fakeReindexer{}, fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}}, nil)
	d := &adminMediaHandler{seekIndex: svc}
	got, err := d.GetMediaRepairHistory(context.Background(), &eveyv1.GetMediaRepairHistoryRequest{MediaId: "mi-1"})
	require.NoError(t, err)
	assert.NotNil(t, got.GetEntries())
	assert.Empty(t, got.GetEntries())
}

func TestGetMediaRepairHistory_UnknownMediaIsNotFound(t *testing.T) {
	d := newJournalHandler(&recordingJournal{}, erroringCatalog{err: shared.ErrNotFound})
	_, err := d.GetMediaRepairHistory(context.Background(), &eveyv1.GetMediaRepairHistoryRequest{MediaId: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestGetMediaRepairHistory_JournalFailureIsSanitized(t *testing.T) {
	d := newJournalHandler(&recordingJournal{err: errors.New("sqlite: database is locked")},
		fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}})
	_, err := d.GetMediaRepairHistory(context.Background(), &eveyv1.GetMediaRepairHistoryRequest{MediaId: "mi-1"})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.NotContains(t, err.Error(), "sqlite")
}

func TestListMediaRepairs_WindowAndJobReachTheJournal(t *testing.T) {
	j := &recordingJournal{records: []appmaintenance.RepairRecord{
		{MediaID: "mi-2", Path: "/media/b.mkv", Op: appmaintenance.RepairReindex, Job: "rollback", Started: time.UnixMilli(1_700_000_000_000)},
	}}
	d := newJournalHandler(j, fakeSeekCatalog{})

	got, err := d.ListMediaRepairs(context.Background(), &eveyv1.ListMediaRepairsRequest{
		Job: "rollback", Limit: 7, SinceMs: 1_600_000_000_000, UntilMs: 1_800_000_000_000,
	})
	require.NoError(t, err)
	assert.Equal(t, "rollback", j.listQuery.Job)
	assert.Equal(t, 7, j.listQuery.Limit)
	assert.Equal(t, time.UnixMilli(1_600_000_000_000), j.listQuery.Since)
	assert.Equal(t, time.UnixMilli(1_800_000_000_000), j.listQuery.Until)
	require.Len(t, got.GetEntries(), 1)
	assert.Equal(t, "mi-2", got.GetEntries()[0].GetMediaId())
	assert.Equal(t, "/media/b.mkv", got.GetEntries()[0].GetPath())

	_, err = d.ListMediaRepairs(context.Background(), &eveyv1.ListMediaRepairsRequest{})
	require.NoError(t, err)
	assert.True(t, j.listQuery.Since.IsZero(), "0 ms = open window, never the epoch")
	assert.True(t, j.listQuery.Until.IsZero())
	assert.Equal(t, 100, j.listQuery.Limit, "0 = service default")
}

func TestListMediaRepairs_JournalFailureIsSanitized(t *testing.T) {
	d := newJournalHandler(&recordingJournal{err: errors.New("disk gone")}, fakeSeekCatalog{})
	_, err := d.ListMediaRepairs(context.Background(), &eveyv1.ListMediaRepairsRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.NotContains(t, err.Error(), "disk gone")
}

func TestAckSeekIndex_OutsideAnyScan(t *testing.T) {
	d := newJournalHandler(&recordingJournal{}, fakeSeekCatalog{})
	ctx := context.Background()

	_, err := d.AckSeekIndexMedia(ctx, &eveyv1.AckSeekIndexMediaRequest{MediaId: "mi-1", Acked: true})
	assert.Equal(t, codes.NotFound, status.Code(err), "a file no scan listed cannot be acknowledged")

	_, err = d.AckSeekIndexGroup(ctx, &eveyv1.AckSeekIndexGroupRequest{LibraryId: "lib-1", Verdict: string(appmaintenance.VerdictInformational)})
	assert.Equal(t, codes.NotFound, status.Code(err), "no cached scan for the library")

	_, err = d.AckSeekIndexGroup(ctx, &eveyv1.AckSeekIndexGroupRequest{LibraryId: "lib-1", Verdict: "broken"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "only informational verdicts are acknowledgeable in bulk")
}

// A file nobody repaired, or whose last repair discarded bytes, is a verdict
// the operator can act on - not a server fault to page anyone about.
func TestUndoMediaRepair_RefusalsAreFailedPreconditionAndUnaudited(t *testing.T) {
	ctx := ctxWithClaims("adm-1", "prf-adm", true)
	catalog := fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}}

	d, repo := newAuditedMediaHandler(t)
	d.translator = realTranslator(t)
	d.seekIndex = appmaintenance.NewSeekIndexService(fakeReindexer{}, catalog, nil).WithRepairJournal(&recordingJournal{})
	_, err := d.UndoMediaRepair(ctx, &eveyv1.UndoMediaRepairRequest{MediaId: "mi-1"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, "No repair is recorded for this file", status.Convert(err).Message())

	d.seekIndex = appmaintenance.NewSeekIndexService(fakeReindexer{}, catalog, nil).WithRepairJournal(&recordingJournal{
		records: []appmaintenance.RepairRecord{{MediaID: "mi-1", Path: "/media/m.mkv", Op: appmaintenance.RepairReindex, Outcome: "applied"}},
	})
	_, err = d.UndoMediaRepair(ctx, &eveyv1.UndoMediaRepairRequest{MediaId: "mi-1"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, "The last repair on this file cannot be undone in software", status.Convert(err).Message())

	assert.Empty(t, auditEntries(t, repo, "undo_media_repair"), "nothing happened to the file, nothing to audit")
}

// An unknown collection or plugin answers a plain NotFound: the repository's
// own error text (SQL, driver wording) stays out of the wire.
func TestNotFoundAnswersCarryNoRepositoryText(t *testing.T) {
	ctx := ctxWithClaims("adm-1", "prf-adm", true)

	col, _ := newTestCollectionHandler()
	_, err := col.GetCollection(ctx, &eveyv1.GetCollectionRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.NotContains(t, status.Convert(err).Message(), ":")

	pl := newTestPluginHandler()
	_, err = pl.GetPluginConfig(ctx, &eveyv1.GetPluginConfigRequest{PluginId: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.NotContains(t, status.Convert(err).Message(), ":")
	_, err = pl.UpdatePluginConfig(ctx, &eveyv1.UpdatePluginConfigRequest{PluginId: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.NotContains(t, status.Convert(err).Message(), ":")
}

type recordingRetimer struct{ calls int }

func (r *recordingRetimer) RetimeTracks(context.Context, string, map[uint64]float64) error {
	r.calls++
	return nil
}

// An operator's shift beyond what the tool moves is the caller's mistake
// (InvalidArgument, the sentence says how far it goes), a server with no
// retimer says so as a precondition, and both leave the file untouched.
func TestApplyAudioShift_RefusalsAreNotServerFaults(t *testing.T) {
	ctx := context.Background()
	_, err := (&adminMediaHandler{}).ApplyAudioShift(ctx, &eveyv1.ApplyAudioShiftRequest{MediaId: "mi-1", Track: 2, ShiftSec: 1})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	catalog := fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}}
	noRetimer := &adminMediaHandler{seekIndex: appmaintenance.NewSeekIndexService(fakeReindexer{}, catalog, nil)}
	_, err = noRetimer.ApplyAudioShift(ctx, &eveyv1.ApplyAudioShiftRequest{MediaId: "mi-1", Track: 2, ShiftSec: 1})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	r := &recordingRetimer{}
	d := &adminMediaHandler{seekIndex: appmaintenance.NewSeekIndexService(fakeReindexer{}, catalog, nil).WithTrackRetimer(r), translator: realTranslator(t)}
	_, err = d.ApplyAudioShift(ctx, &eveyv1.ApplyAudioShiftRequest{MediaId: "mi-1", Track: 2, ShiftSec: 3600})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "60")
	assert.Zero(t, r.calls, "nothing may be written for a value out of range")
}
