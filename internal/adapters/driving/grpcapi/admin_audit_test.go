package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
)

func TestAdminGetAuditLog(t *testing.T) {
	d := newTestSystemDelegate()
	audit := &adminAuditDelegate{activitySvc: d.activitySvc}
	resp, err := audit.GetAuditLog(context.Background(), &eveyv1.GetAuditLogRequest{Limit: 10})
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 audit entries, got %d", resp.Total)
	}
}

func TestAdminGetAuditLog_WithFilters(t *testing.T) {
	d := newTestSystemDelegate()
	audit := &adminAuditDelegate{activitySvc: d.activitySvc}
	resp, err := audit.GetAuditLog(context.Background(), &eveyv1.GetAuditLogRequest{
		Limit:        10,
		UserId:       "u-1",
		Action:       "login",
		ResourceType: "user",
	})
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	_ = resp
}
