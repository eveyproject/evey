package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	appactivity "github.com/eveyproject/evey/internal/app/activity"
	"github.com/eveyproject/evey/internal/domain/activity"
	activityfakes "github.com/eveyproject/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// A wrong PIN on a kids profile left no trace: the entry says who tried, on
// which profile, the field names and the refusal, never the PIN itself.
func TestAuditedRoute_RefusedPINNamesFieldNotValue(t *testing.T) {
	repo := activityfakes.NewFakeAuditLogRepository()
	svc := appactivity.NewCommandService(activityfakes.NewFakeLoginHistoryRepository(), activityfakes.NewFakeServerMetricRepository(), repo, sharedfakes.NewFakeEventBus())
	var seen string
	h := auditedRoute(svc, "verify_profile_pin", "profile", "id", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = string(b)
		w.WriteHeader(http.StatusForbidden)
	})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/profiles/{id}/parental/verify-pin", h)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles/prf-kid/parental/verify-pin", strings.NewReader(`{"pin":"9876"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "usr-1", ProfileID: "prf-1"}))
	mux.ServeHTTP(httptest.NewRecorder(), req)

	if seen != `{"pin":"9876"}` {
		t.Fatalf("handler read %q, want the whole body", seen)
	}
	action := "verify_profile_pin"
	got, err := repo.List(context.Background(), activity.AuditLogFilter{Action: &action, Limit: 10})
	if err != nil || len(got) != 1 {
		t.Fatalf("entries = %v (err %v), want 1", got, err)
	}
	e := got[0]
	if e.UserID != "usr-1" || e.ResourceID == nil || *e.ResourceID != "prf-kid" {
		t.Errorf("entry = %+v, want the caller and the kids profile", e)
	}
	if e.Details["outcome"] != "refused" || e.Details["status"] != http.StatusForbidden {
		t.Errorf("details = %v, want refused / 403", e.Details)
	}
	if fields, _ := e.Details["fields"].([]string); !slices.Equal(fields, []string{"pin"}) {
		t.Errorf("fields = %v, want [pin]", e.Details["fields"])
	}
	if strings.Contains(fmt.Sprint(e.Details), "9876") {
		t.Errorf("details %v carry the PIN", e.Details)
	}
}
