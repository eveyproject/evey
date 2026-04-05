package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
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
