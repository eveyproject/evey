package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// fakeSecurityDashboardSvc implements pluginSecurityDashboardProvider for tests.
type fakeSecurityDashboardSvc struct {
	snapshots []domainplugin.PluginSecuritySnapshot
	sandbox   appplugin.SandboxStatus
	err       error
}

func (f *fakeSecurityDashboardSvc) GetSecurityDashboard(_ context.Context) ([]domainplugin.PluginSecuritySnapshot, error) {
	return f.snapshots, f.err
}

func (f *fakeSecurityDashboardSvc) SandboxStatus() appplugin.SandboxStatus {
	return f.sandbox
}

func newSecurityDashboardTestMux(t *testing.T, svc pluginSecurityDashboardProvider) (mux *http.ServeMux, adminToken, userToken string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	userToken, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})
	mux = http.NewServeMux()
	registerPluginSecurityDashboardRoute(mux, gen, svc)
	return mux, adminToken, userToken
}

func TestPluginSecurityDashboard_NoAuth_401(t *testing.T) {
	svc := &fakeSecurityDashboardSvc{}
	mux, _, _ := newSecurityDashboardTestMux(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins/security-dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPluginSecurityDashboard_NonAdmin_403(t *testing.T) {
	svc := &fakeSecurityDashboardSvc{}
	mux, _, userToken := newSecurityDashboardTestMux(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins/security-dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPluginSecurityDashboard_Admin_200(t *testing.T) {
	installed := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	svc := &fakeSecurityDashboardSvc{
		sandbox: appplugin.SandboxStatus{
			Configured:      "auto",
			Effective:       "landlock",
			LandlockKernel:  true,
			LauncherPresent: true,
		},
		snapshots: []domainplugin.PluginSecuritySnapshot{
			{
				ID:           "plugin-1",
				Name:         "My Plugin",
				Version:      "1.2.3",
				SandboxMode:  "basic",
				Permissions:  []string{"network"},
				Findings:     nil,
				LastError:    nil,
				SourceSHA256: "deadbeef",
				InstalledAt:  installed,
				SizeBytes:    4096,
			},
		},
	}
	mux, adminToken, _ := newSecurityDashboardTestMux(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins/security-dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var body struct {
		Plugins []domainplugin.PluginSecuritySnapshot `json:"plugins"`
		Sandbox appplugin.SandboxStatus               `json:"sandbox"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Sandbox.Effective != "landlock" || body.Sandbox.Configured != "auto" {
		t.Errorf("unexpected sandbox status: %+v", body.Sandbox)
	}
	if !body.Sandbox.LandlockKernel {
		t.Error("expected landlockKernel=true in sandbox status")
	}
	if len(body.Plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(body.Plugins))
	}
	p := body.Plugins[0]
	if p.ID != "plugin-1" {
		t.Errorf("unexpected plugin ID: %q", p.ID)
	}
	if p.SandboxMode != "basic" {
		t.Errorf("unexpected sandboxMode: %q", p.SandboxMode)
	}
	if p.SourceSHA256 != "deadbeef" {
		t.Errorf("unexpected sourceSHA256: %q", p.SourceSHA256)
	}
	if p.SizeBytes != 4096 {
		t.Errorf("unexpected sizeBytes: %d", p.SizeBytes)
	}
}

func TestPluginSecurityDashboard_Admin_EmptyList(t *testing.T) {
	svc := &fakeSecurityDashboardSvc{snapshots: nil}
	mux, adminToken, _ := newSecurityDashboardTestMux(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins/security-dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var body struct {
		Plugins []domainplugin.PluginSecuritySnapshot `json:"plugins"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// Must return an array, not null.
	if body.Plugins == nil {
		t.Error("expected non-null plugins array")
	}
}

func TestPluginSecurityDashboard_ServiceError_500(t *testing.T) {
	svc := &fakeSecurityDashboardSvc{err: errors.New("db unavailable")}
	mux, adminToken, _ := newSecurityDashboardTestMux(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugins/security-dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
