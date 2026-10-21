package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

func TestTailFile_ReturnsLastN(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "evey.log")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	lines, err := tailFile(p, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	if !strings.HasSuffix(lines[len(lines)-1], "line5") {
		t.Errorf("last line should be line5, got %q", lines[len(lines)-1])
	}
}

func TestTailFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(p, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	lines, err := tailFile(p, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lines != nil {
		t.Errorf("expected nil for empty file, got %v", lines)
	}
}

func newAdminTestMux(t *testing.T) (mux *http.ServeMux, adminToken, userToken string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	userToken, _ = gen.GenerateAccessToken(domainuser.Claims{UserID: "user-1", IsAdmin: false})

	mux = http.NewServeMux()
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "evey-2026-05-16.log"), []byte("log line 1\nlog line 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	registerLogsRoute(mux, logDir, gen, nil)
	return mux, adminToken, userToken
}

// --- GET /api/v1/admin/logs ---

func TestAdminLogs_NoAuth_Forbidden(t *testing.T) {
	mux, _, _ := newAdminTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminLogs_NonAdmin_Forbidden(t *testing.T) {
	mux, _, userToken := newAdminTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAdminLogs_Admin_OK(t *testing.T) {
	mux, adminToken, _ := newAdminTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "lines") {
		t.Error("expected 'lines' key in response")
	}
}

func TestAdminLogs_Admin_ViaQueryToken(t *testing.T) {
	mux, adminToken, _ := newAdminTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?token="+adminToken, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with token query param, got %d", w.Code)
	}
}

// --- GET /api/v1/admin/logs/stream ---

func TestAdminLogsStream_NoAuth_Forbidden(t *testing.T) {
	mux, _, _ := newAdminTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminLogsStream_NonAdmin_Forbidden(t *testing.T) {
	mux, _, userToken := newAdminTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs/stream", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}
