package grpcapi

import (
	"context"
	"strings"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appactivity "github.com/eveyproject/evey/internal/app/activity"
	"github.com/eveyproject/evey/internal/domain/activity"
	activityfakes "github.com/eveyproject/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// A page view is an audit row against the caller's own identity; junk paths
// are dropped silently (a client bug is not an error the user should see),
// and a handler without a command service is a no-op.
func TestRecordPageView(t *testing.T) {
	auditRepo := activityfakes.NewFakeAuditLogRepository()
	cmd := appactivity.NewCommandService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		auditRepo,
		sharedfakes.NewFakeEventBus(),
	)
	h := &activityHandler{cmd: cmd}
	ctx := ctxWithClaims("user-1", "profile-1", false)

	if _, err := h.RecordPageView(ctx, &eveyv1.RecordPageViewRequest{Path: " /library/lib-1 "}); err != nil {
		t.Fatal(err)
	}
	action := "page_view"
	rows, err := auditRepo.List(context.Background(), activity.AuditLogFilter{Action: &action, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].UserID != "user-1" || rows[0].ProfileID != "profile-1" || rows[0].Details["path"] != "/library/lib-1" {
		t.Errorf("row = %+v", rows[0])
	}

	for name, path := range map[string]string{
		"empty":       "",
		"relative":    "library",
		"too long":    "/" + strings.Repeat("a", 512),
		"only spaces": "   ",
	} {
		if _, err := h.RecordPageView(ctx, &eveyv1.RecordPageViewRequest{Path: path}); err != nil {
			t.Errorf("%s: junk must be ignored, not refused: %v", name, err)
		}
	}
	rows, _ = auditRepo.List(context.Background(), activity.AuditLogFilter{Action: &action, Limit: 10})
	if len(rows) != 1 {
		t.Errorf("junk paths recorded: %d rows", len(rows))
	}

	if _, err := h.RecordPageView(context.Background(), &eveyv1.RecordPageViewRequest{Path: "/x"}); err == nil {
		t.Error("no claims must be refused")
	}
	if _, err := (&activityHandler{}).RecordPageView(context.Background(), &eveyv1.RecordPageViewRequest{Path: "/x"}); err != nil {
		t.Errorf("no command service: got %v, want a silent no-op", err)
	}
}
