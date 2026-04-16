package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupSPADir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>SPA-INDEX</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('hi')"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSPAHandler_UnknownRoute_ServesIndex(t *testing.T) {
	dir := setupSPADir(t)
	h := spaHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/foo/bar", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SPA-INDEX") {
		t.Errorf("expected index.html content, got %q", w.Body.String())
	}
}

func TestSPAHandler_RootServesIndex(t *testing.T) {
	dir := setupSPADir(t)
	h := spaHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SPA-INDEX") {
		t.Errorf("expected index.html content, got %q", w.Body.String())
	}
}

func TestSPAHandler_ExistingAsset_ServesFile(t *testing.T) {
	dir := setupSPADir(t)
	h := spaHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "console.log") {
		t.Errorf("expected app.js content, got %q", w.Body.String())
	}
}

func TestSPAHandler_APIPath_NotFound(t *testing.T) {
	dir := setupSPADir(t)
	h := spaHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for /api/*, got %d", w.Code)
	}
}

func TestSPAHandler_Metrics_NotFound(t *testing.T) {
	dir := setupSPADir(t)
	h := spaHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for /metrics, got %d", w.Code)
	}
}

func TestSPAHandler_Healthz_NotFound(t *testing.T) {
	dir := setupSPADir(t)
	h := spaHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for /healthz, got %d", w.Code)
	}
}
