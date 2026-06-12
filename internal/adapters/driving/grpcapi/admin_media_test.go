package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
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
}

func (f fakeReindexer) HasSeekIndex(context.Context, string) (bool, error) { return false, nil }
func (f fakeReindexer) HasBackup(context.Context, string) (bool, error)    { return false, nil }
func (f fakeReindexer) ReindexInPlace(context.Context, string, mkv.ReindexOptions) (mkv.ReindexResult, error) {
	return mkv.ReindexResult{}, f.reindexErr
}
func (f fakeReindexer) RestoreBackup(context.Context, string) error { return f.restoreErr }

// A read-only media volume must surface as FailedPrecondition with the
// actionable remount message, not as a sanitized 500.
func TestReindexMediaSeekIndex_ReadOnlyVolume(t *testing.T) {
	svc := appmaintenance.NewSeekIndexService(
		fakeReindexer{reindexErr: &os.PathError{Op: "open", Path: "/media/m.mkv", Err: syscall.EROFS}},
		fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}},
		nil,
	)
	d := &adminMediaHandler{seekIndex: svc}
	_, err := d.ReindexMediaSeekIndex(context.Background(), &eveyv1.ReindexMediaSeekIndexRequest{MediaId: "mi-1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v (%v)", status.Code(err), err)
	}
}

func TestRollbackMediaSeekIndex_ReadOnlyVolume(t *testing.T) {
	svc := appmaintenance.NewSeekIndexService(
		fakeReindexer{restoreErr: &os.PathError{Op: "rename", Path: "/media/m.mkv", Err: syscall.EROFS}},
		fakeSeekCatalog{ref: appmaintenance.MediaRef{MediaID: "mi-1", Path: "/media/m.mkv"}},
		nil,
	)
	d := &adminMediaHandler{seekIndex: svc}
	_, err := d.RollbackMediaSeekIndex(context.Background(), &eveyv1.RollbackMediaSeekIndexRequest{MediaId: "mi-1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v (%v)", status.Code(err), err)
	}
}
