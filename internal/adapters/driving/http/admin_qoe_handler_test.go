package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appqoe "github.com/eveyproject/evey/internal/app/qoe"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// fakeQoEDashboardSvc implements qoeDashboardProvider for tests.
// It records the last window it received and returns a canned dashboard or error.
type fakeQoEDashboardSvc struct {
	dashboard  *appqoe.Dashboard
	err        error
	lastWindow time.Duration
}

func (f *fakeQoEDashboardSvc) GetDashboard(_ context.Context, window time.Duration) (*appqoe.Dashboard, error) {
	f.lastWindow = window
	return f.dashboard, f.err
}

// newQoETestMux builds a fresh mux with the QoE route registered.
// Auth setup mirrors admin_plugin_security_handler_test.go (RSA key, NewJWTGenerator).
func newQoETestMux(t *testing.T, svc qoeDashboardProvider) (mux *http.ServeMux, adminToken, userToken string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	userToken, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})
	mux = http.NewServeMux()
	registerQoEDashboardRoute(mux, gen, svc)
	return mux, adminToken, userToken
}

// cannedDashboard returns a minimal non-nil Dashboard for happy-path tests.
func cannedDashboard() *appqoe.Dashboard {
	return &appqoe.Dashboard{
		Summary: &appqoe.Summary{
			WindowLabel:   "24h",
			TotalEvents:   100,
			StarvedEvents: 5,
			StarvedRatio:  0.05,
		},
		TopStarved: []appqoe.StarvedTitle{
			{MediaItemID: "mi-1", Title: "A Film", StarvedEvents: 3},
		},
		ByDevice: []appqoe.DeviceBreakdown{
			{DeviceClass: "desktop", TotalEvents: 80},
		},
	}
}

// --- 1. parseQoEWindow table test ---

func TestParseQoEWindow(t *testing.T) {
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"7d", 7 * 24 * time.Hour},
		{"7days", 7 * 24 * time.Hour},
		{"week", 7 * 24 * time.Hour},
		{"24h", 24 * time.Hour},
		{"", 24 * time.Hour},
		{"garbage", 24 * time.Hour},
		{"1h", 24 * time.Hour},
		{"30d", 24 * time.Hour},
	}
	for _, c := range cases {
		got := parseQoEWindow(c.input)
		if got != c.want {
			t.Errorf("parseQoEWindow(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

// --- 2. Route behaviour ---

// 2a. Admin GET → 200; fake received default window (24h).
func TestQoEDashboard_Admin_200_DefaultWindow(t *testing.T) {
	svc := &fakeQoEDashboardSvc{dashboard: cannedDashboard()}
	mux, adminToken, _ := newQoETestMux(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/qoe/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got appqoe.Dashboard
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Summary == nil {
		t.Error("expected non-nil summary")
	}
	if svc.lastWindow != 24*time.Hour {
		t.Errorf("expected window=24h, got %v", svc.lastWindow)
	}
}

// 2b. ?window=7d → fake received 168h.
func TestQoEDashboard_Admin_7dWindow(t *testing.T) {
	svc := &fakeQoEDashboardSvc{dashboard: cannedDashboard()}
	mux, adminToken, _ := newQoETestMux(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/qoe/dashboard?window=7d", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if svc.lastWindow != 7*24*time.Hour {
		t.Errorf("expected window=168h, got %v", svc.lastWindow)
	}
}

// 2c. Svc returns error → 500.
func TestQoEDashboard_ServiceError_500(t *testing.T) {
	svc := &fakeQoEDashboardSvc{err: errors.New("db down")}
	mux, adminToken, _ := newQoETestMux(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/qoe/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// 2d. Nil TopStarved + nil ByDevice → JSON has [] not null.
func TestQoEDashboard_NilSlices_EmptyArrayInJSON(t *testing.T) {
	svc := &fakeQoEDashboardSvc{
		dashboard: &appqoe.Dashboard{
			Summary:    &appqoe.Summary{WindowLabel: "24h"},
			TopStarved: nil,
			ByDevice:   nil,
		},
	}
	mux, adminToken, _ := newQoETestMux(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/qoe/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()
	// Verify raw JSON uses [] not null for both slices.
	if !strings.Contains(body, `"top_starved_titles":[]`) {
		t.Errorf("expected top_starved_titles:[] in JSON, got: %s", body)
	}
	if !strings.Contains(body, `"by_device":[]`) {
		t.Errorf("expected by_device:[] in JSON, got: %s", body)
	}

	// Also assert via decode that slices are non-nil.
	var got appqoe.Dashboard
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TopStarved == nil {
		t.Error("decoded TopStarved is nil, want empty slice")
	}
	if got.ByDevice == nil {
		t.Error("decoded ByDevice is nil, want empty slice")
	}
}

// 2e. svc==nil → route not registered → 404.
func TestQoEDashboard_NilSvc_RouteNotRegistered(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	mux := http.NewServeMux()
	registerQoEDashboardRoute(mux, gen, nil)

	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/qoe/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when svc==nil, got %d", w.Code)
	}
}

// --- 3. Auth gating ---
// Auth setup mirrors admin_handler_test.go / admin_plugin_security_handler_test.go:
// RSA key + auth.NewJWTGenerator, Claims{IsAdmin:true/false}.

// Unauthenticated request → 401.
func TestQoEDashboard_NoAuth_401(t *testing.T) {
	svc := &fakeQoEDashboardSvc{dashboard: cannedDashboard()}
	mux, _, _ := newQoETestMux(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/qoe/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// Non-admin authenticated request → 403.
func TestQoEDashboard_NonAdmin_403(t *testing.T) {
	svc := &fakeQoEDashboardSvc{dashboard: cannedDashboard()}
	mux, _, userToken := newQoETestMux(t, svc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/qoe/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
