package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	appactivity "github.com/evey-project/evey/internal/app/activity"
	"github.com/evey-project/evey/internal/domain/activity"
	actfakes "github.com/evey-project/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

// Every hand-written admin route audits through this helper. The row must
// carry who (user AND profile - a profile can be renamed or deleted, the
// trace survives it), from where, and what; and neither a missing service
// nor an audit-store failure may fail the admin action itself.
func TestAdminHandler_Audit(t *testing.T) {
	auditRepo := actfakes.NewFakeAuditLogRepository()
	cmd := appactivity.NewCommandService(actfakes.NewFakeLoginHistoryRepository(),
		actfakes.NewFakeServerMetricRepository(), auditRepo, sharedfakes.NewFakeEventBus())
	h := &adminHandler{activityCmdSvc: cmd}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/x", nil)
	r.RemoteAddr = "10.0.0.7:5555"
	r.Header.Set("User-Agent", "evey-test/1")
	r = withClaims(r, &domainuser.Claims{UserID: "u1", ProfileID: "p1", IsAdmin: true})
	id := "m42"
	h.audit(r, "media.delete", "media_item", &id, map[string]any{"path": "/x"})

	rows, err := auditRepo.List(context.Background(), activity.AuditLogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	e := rows[0]
	if e.UserID != "u1" || e.ProfileID != "p1" || e.Action != "media.delete" || e.ResourceType != "media_item" {
		t.Errorf("row = %+v", e)
	}
	if e.ResourceID == nil || *e.ResourceID != "m42" || e.ClientIP == nil || *e.ClientIP != "10.0.0.7:5555" || e.UserAgent == nil || *e.UserAgent != "evey-test/1" {
		t.Errorf("who/where not recorded: %+v", e)
	}
	if e.ID == "" || e.CreatedAt.IsZero() {
		t.Error("id and timestamp are stamped by the service")
	}
	if e.Details["path"] != "/x" {
		t.Errorf("details = %v", e.Details)
	}

	// No claims (a route the middleware let through without a principal) writes nothing rather than a row with no author.
	h.audit(httptest.NewRequest(http.MethodPost, "/api/v1/admin/x", nil), "media.delete", "media_item", nil, nil)
	rows, _ = auditRepo.List(context.Background(), activity.AuditLogFilter{})
	if len(rows) != 1 {
		t.Errorf("anonymous audit must be dropped, rows = %d", len(rows))
	}

	// A failing audit store is logged, never propagated.
	auditRepo.Err = errors.New("db down")
	h.audit(r, "media.delete", "media_item", nil, nil)

	// No service wired: a no-op.
	(&adminHandler{}).audit(r, "media.delete", "media_item", nil, nil)
}
