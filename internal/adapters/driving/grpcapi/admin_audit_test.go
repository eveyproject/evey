package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"google.golang.org/grpc/codes"
)

func TestAdminGetAuditLog(t *testing.T) {
	d := newTestSystemHandler()
	audit := &adminSessionHandler{activitySvc: d.activitySvc}
	resp, err := audit.GetAuditLog(context.Background(), &eveyv1.GetAuditLogRequest{Limit: 10})
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 audit entries, got %d", resp.Total)
	}
}

func TestSendSessionCommand_NonAdminDenied(t *testing.T) {
	audit := &adminSessionHandler{}
	ctx := userCtx("usr-1", "prf-1")
	_, err := audit.SendSessionCommand(ctx, &eveyv1.SendSessionCommandRequest{SessionId: "s-1", Command: "pause"})
	assertCode(t, err, codes.PermissionDenied)
}

func TestSendSessionCommand_Unauthenticated(t *testing.T) {
	audit := &adminSessionHandler{}
	_, err := audit.SendSessionCommand(context.Background(), &eveyv1.SendSessionCommandRequest{SessionId: "s-1", Command: "pause"})
	assertCode(t, err, codes.Unauthenticated)
}

func TestAdminGetAuditLog_WithFilters(t *testing.T) {
	d := newTestSystemHandler()
	audit := &adminSessionHandler{activitySvc: d.activitySvc}
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
