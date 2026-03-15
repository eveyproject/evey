package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	activityfakes "github.com/gravity-zero/evey/internal/domain/activity/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

func newTestActivityQuerySvc() *appactivity.QueryService {
	return appactivity.NewQueryService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		activityfakes.NewFakeAuditLogRepository(),
		activityfakes.NewFakePlaySessionStatsReader(),
		activityfakes.NewFakePlayHistoryRepository(),
	).WithAnalyticsReader(activityfakes.NewFakeAnalyticsReader())
}

func TestAnalytics_NoAuth_401(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	svc := newTestActivityQuerySvc()
	mux := http.NewServeMux()
	registerAnalyticsRoutes(mux, svc, tokenGen)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAnalytics_NonAdmin_403(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	svc := newTestActivityQuerySvc()
	mux := http.NewServeMux()
	registerAnalyticsRoutes(mux, svc, tokenGen)

	tok, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAnalytics_Admin_200(t *testing.T) {
	tokenGen := newTestJWTGen(t)
	svc := newTestActivityQuerySvc()
	mux := http.NewServeMux()
	registerAnalyticsRoutes(mux, svc, tokenGen)

	tok, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: true})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}
