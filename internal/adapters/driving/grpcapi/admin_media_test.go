package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/domain/shared"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAdminAcquireReleaseHeavyOp(t *testing.T) {
	d := &adminMediaHandler{heavyOpSemaphore: make(chan struct{}, 1)}

	if !d.acquireHeavyOp() {
		t.Error("first acquire should succeed")
	}
	if d.acquireHeavyOp() {
		t.Error("second acquire should fail (semaphore full)")
	}
	d.releaseHeavyOp()
	if !d.acquireHeavyOp() {
		t.Error("acquire after release should succeed")
	}
	d.releaseHeavyOp()
}

func TestAdminAcquireHeavyOp_NilSemaphore(t *testing.T) {
	d := &adminMediaHandler{}
	if !d.acquireHeavyOp() {
		t.Error("nil semaphore should always succeed")
	}
	d.releaseHeavyOp() // should not panic
}

func TestAdminRefreshAllMetadata_NilService(t *testing.T) {
	d := &adminMediaHandler{}
	_, err := d.RefreshAllMetadata(context.Background(), &eveyv1.RefreshAllMetadataRequest{})
	if err == nil {
		t.Error("expected error for nil metadataCmd")
	}
}

func TestAdminGenerateAllTrickplays_NilService(t *testing.T) {
	d := &adminMediaHandler{}
	_, err := d.GenerateAllTrickplays(context.Background(), &eveyv1.GenerateAllTrickplaysRequest{})
	if err == nil {
		t.Error("expected error for nil libCmd")
	}
}

func TestAdminDeleteMediaItem_NilService(t *testing.T) {
	d := &adminMediaHandler{}
	_, err := d.DeleteMediaItem(context.Background(), &eveyv1.DeleteMediaItemRequest{Id: "m-1"})
	if err == nil {
		t.Error("expected error for nil libCmd")
	}
}

func TestSeekIndexError_Mapping(t *testing.T) {
	d := &adminMediaHandler{}
	ctx := context.Background()

	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"erofs wrapped", &os.PathError{Op: "rename", Path: "/media/m.mkv", Err: syscall.EROFS}, codes.FailedPrecondition},
		{"erofs by message", errors.New("mkvgo reindex: rebuild: read-only file system"), codes.FailedPrecondition},
		{"job running", fmt.Errorf("seek-index reindex: %w", appmaintenance.ErrJobRunning), codes.ResourceExhausted},
		{"backup overcommit", &appmaintenance.BackupSpaceError{Volumes: []appmaintenance.SeekVolume{
			{Root: "/media", FreeBytes: 1 << 30, MissingBytes: 40 << 30, MissingFiles: 3},
		}}, codes.FailedPrecondition},
		{"unknown media", fmt.Errorf("seek-index reindex: lookup media x: %w", shared.ErrNotFound), codes.NotFound},
		{"nothing to undo", appmaintenance.ErrNoRepairToUndo, codes.FailedPrecondition},
		{"not reversible", fmt.Errorf("undo repair: %w", appmaintenance.ErrNotReversible), codes.FailedPrecondition},
		{"newer journal schema", appmaintenance.ErrJournalSchema, codes.FailedPrecondition},
		{"anything else sanitized", errors.New("boom"), codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := status.Code(d.seekIndexError(ctx, "op", tc.err))
			if got != tc.want {
				t.Errorf("code: want %v, got %v", tc.want, got)
			}
		})
	}
}

// fakes for driving the real SeekIndexService through the handler.
type fakeSeekCatalog struct{ ref appmaintenance.MediaRef }

func (f fakeSeekCatalog) ListMediaFiles(context.Context, string) ([]appmaintenance.MediaRef, error) {
	return []appmaintenance.MediaRef{f.ref}, nil
}
func (f fakeSeekCatalog) GetMediaFile(context.Context, string) (appmaintenance.MediaRef, error) {
	return f.ref, nil
}

type fakeReindexer struct {
	reindexErr error
	restoreErr error
	gate       chan struct{} // when non-nil, ReindexInPlace blocks until closed
}

func (f fakeReindexer) HasSeekIndex(context.Context, string) (bool, error) { return false, nil }
func (f fakeReindexer) HasBackup(context.Context, string) (bool, error)    { return false, nil }
func (f fakeReindexer) ReindexInPlace(context.Context, string, mkv.ReindexOptions) (mkv.ReindexResult, error) {
	if f.gate != nil {
		<-f.gate
	}
	return mkv.ReindexResult{}, f.reindexErr
}
func (f fakeReindexer) RestoreBackup(context.Context, string) error { return f.restoreErr }
func (f fakeReindexer) DeleteBackup(context.Context, string) error  { return nil }
func (f fakeReindexer) SupportsInPlace() bool                       { return false }

// waitJobDone polls the handler's job endpoint until the job finishes and
// returns the final response (every seek-index entry point is async now).
func waitJobDone(t *testing.T, d *adminMediaHandler) *eveyv1.GetSeekIndexJobResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := d.GetSeekIndexJob(context.Background(), &eveyv1.GetSeekIndexJobRequest{})
		if err != nil {
			t.Fatalf("GetSeekIndexJob: %v", err)
		}
		if !res.Running {
			return res
		}
		if time.Now().After(deadline) {
			t.Fatal("seek-index job did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A read-only media volume must surface in the job summary with the read_only
// flag so the UI can show the actionable remount message - the repair itself
// is async (a sync rewrite of a multi-GB file 504s behind a reverse proxy).
func TestReindexMediaSeekIndex_ReadOnlyVolume_FlagsJobSummary(t *testing.T) {
	svc := appmaintenance.NewSeekIndexService(
		fakeReindexer{reindexErr: &os.PathError{Op: "open", Path: "/media/m.mkv", Err: syscall.EROFS}},
		fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}},
		nil,
	)
	d := &adminMediaHandler{seekIndex: svc}
	res, err := d.ReindexMediaSeekIndex(context.Background(), &eveyv1.ReindexMediaSeekIndexRequest{MediaId: "mi-1"})
	if err != nil {
		t.Fatalf("start repair: %v", err)
	}
	if !res.Accepted {
		t.Fatal("repair must be accepted as a background job")
	}
	job := waitJobDone(t, d)
	if job.Last == nil {
		t.Fatal("finished job must publish a summary")
	}
	if !job.Last.ReadOnly {
		t.Error("EROFS failure must flag the summary read_only")
	}
	if len(job.Last.Failures) != 1 || !job.Last.Failures[0].ReadOnly {
		t.Errorf("want one read-only failure, got %+v", job.Last.Failures)
	}
}

// While a job runs, every other entry point is refused with ResourceExhausted
// (mapped from ErrJobRunning) instead of a 500.
func TestSeekIndexJob_ConcurrentStartRefused(t *testing.T) {
	gate := make(chan struct{})
	svc := appmaintenance.NewSeekIndexService(
		fakeReindexer{gate: gate},
		fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}},
		nil,
	)
	d := &adminMediaHandler{seekIndex: svc}
	if _, err := d.ReindexMediaSeekIndex(context.Background(), &eveyv1.ReindexMediaSeekIndexRequest{MediaId: "mi-1"}); err != nil {
		t.Fatalf("start repair: %v", err)
	}
	_, err := d.StartSeekIndexScan(context.Background(), &eveyv1.StartSeekIndexScanRequest{})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("want ResourceExhausted while a job runs, got %v (%v)", status.Code(err), err)
	}
	close(gate)
	waitJobDone(t, d)
}

// ScanSeekIndex serves the cached scan: empty with scanned_at_ms == 0 before
// any scan, populated after a StartSeekIndexScan job completes.
func TestScanSeekIndex_ServesCachedResult(t *testing.T) {
	svc := appmaintenance.NewSeekIndexService(
		fakeReindexer{},
		fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}},
		nil,
	)
	d := &adminMediaHandler{seekIndex: svc}

	res, err := d.ScanSeekIndex(context.Background(), &eveyv1.ScanSeekIndexRequest{})
	if err != nil {
		t.Fatalf("cached scan: %v", err)
	}
	if res.ScannedAtMs != 0 || len(res.Items) != 0 {
		t.Fatalf("want empty cache before first scan, got %+v", res)
	}

	if _, startErr := d.StartSeekIndexScan(context.Background(), &eveyv1.StartSeekIndexScanRequest{}); startErr != nil {
		t.Fatalf("start scan: %v", startErr)
	}
	waitJobDone(t, d)

	res, err = d.ScanSeekIndex(context.Background(), &eveyv1.ScanSeekIndexRequest{})
	if err != nil {
		t.Fatalf("cached scan: %v", err)
	}
	if res.ScannedAtMs == 0 {
		t.Error("scanned_at_ms must be set after a completed scan")
	}
	// The fake catalog's only file doesn't exist on disk → walked but skipped.
	if res.Total != 0 {
		t.Errorf("ghost file must not be counted, got total=%d", res.Total)
	}
}
