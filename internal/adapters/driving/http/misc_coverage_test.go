package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/plugin"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
)

// --- shared helpers ---

func TestClearRefreshCookie(t *testing.T) {
	w := httptest.NewRecorder()
	clearRefreshCookie(w, false)
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName && c.MaxAge < 0 {
			return
		}
	}
	t.Error("expected refresh cookie to be cleared")
}

func TestContainsJSON(t *testing.T) {
	if !containsJSON("application/json") {
		t.Error("should match application/json")
	}
	if !containsJSON("application/json; charset=utf-8") {
		t.Error("should match with charset")
	}
	if containsJSON("text/plain") {
		t.Error("should not match text/plain")
	}
}

func TestNetworkInfo_Admin_200(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/network-info", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// =====================================
// helpers.go - middleware tests
// =====================================

func TestApiCacheMiddleware_SetsNoStore(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiCacheMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/something", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("expected no-store, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestApiCacheMiddleware_LibrariesCacheHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := apiCacheMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !strings.Contains(w.Header().Get("Cache-Control"), "max-age=300") {
		t.Errorf("expected max-age=300, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestJsonEncodeResponse_OK(t *testing.T) {
	w := httptest.NewRecorder()
	jsonEncodeResponse(w, map[string]string{"key": "val"})
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json content type")
	}
	if !strings.Contains(w.Body.String(), "key") {
		t.Error("expected key in response")
	}
}

func TestSetCORSHeaders_SameOrigin(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	setCORSHeaders(w, req, "http://localhost:3000")
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Error("expected CORS header for same origin")
	}
}

func TestSetCORSHeaders_DifferentOrigin(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	setCORSHeaders(w, req, "http://localhost:3000")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("should not set CORS for different origin")
	}
}

func TestSetCORSHeaders_NoOrigin(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	setCORSHeaders(w, req, "http://localhost:3000")
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("should not set CORS without origin")
	}
}

func TestServeFileWithCache_NotFound(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	serveFileWithCache(w, req, "/nonexistent_file_xyz")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDiscardResponseWriter(t *testing.T) {
	d := &discardResponseWriter{header: make(http.Header)}
	d.WriteHeader(http.StatusOK)
	n, err := d.Write([]byte("test"))
	if err != nil || n != 4 {
		t.Errorf("expected 4 bytes written, got %d err=%v", n, err)
	}
	if d.status != http.StatusOK {
		t.Errorf("expected 200, got %d", d.status)
	}
}

func TestNewServerConfigStore(t *testing.T) {
	repo := sharedfakes.NewFakeServerConfigRepository()
	store := NewServerConfigStore(repo)
	ctx := context.Background()

	// Set
	if err := store.Set(ctx, "key1", "val1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Get
	val, err := store.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if val != "val1" {
		t.Errorf("expected val1, got %q", val)
	}
	// Get missing
	_, err = store.Get(ctx, "missing")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

// ============================================================
// Available runtimes route test
// ============================================================

func TestAvailableRuntimes_OK(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})

	mux := http.NewServeMux()
	registerAvailableRuntimesRoute(mux, gen)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugin-runtimes/available", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "runtimes") {
		t.Error("expected runtimes key in response")
	}
}

// ============================================================
// discardResponseWriter test (helpers.go line 253)
// ============================================================

func TestDiscardResponseWriter_AllMethods(t *testing.T) {
	d := &discardResponseWriter{header: make(http.Header)}
	h := d.Header()
	if h == nil {
		t.Fatal("expected non-nil header")
	}
	h.Set("X-Test", "value")
	if d.Header().Get("X-Test") != "value" {
		t.Error("expected header to be set")
	}
	n, err := d.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("expected 5 nil, got %d %v", n, err)
	}
	d.WriteHeader(http.StatusCreated)
	if d.status != http.StatusCreated {
		t.Errorf("expected 201, got %d", d.status)
	}
}

// ============================================================
// extractBlockedIssues test (plugin_asset_handler.go)
// ============================================================

func TestExtractBlockedIssues_WithBrackets(t *testing.T) {
	msg := "plugin blocked: [uses eval()] [accesses localStorage]"
	issues := extractBlockedIssues(msg)
	if len(issues) < 2 {
		t.Fatalf("expected at least 2 issues, got %d: %v", len(issues), issues)
	}
}

func TestExtractBlockedIssues_NoBrackets(t *testing.T) {
	msg := "some error"
	issues := extractBlockedIssues(msg)
	if len(issues) != 1 || issues[0] != msg {
		t.Errorf("expected [%q], got %v", msg, issues)
	}
}

// ============================================================
// transcode resolveSourceDimensions tests
// ============================================================

// ============================================================
// transcode resolveSubtitle tests
// ============================================================

// ============================================================
// parseClientProfile tests
// ============================================================

func TestParseClientProfile_Default(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := h.parseClientProfile(req)
	if p.MaxWidth == 0 || p.MaxHeight == 0 {
		t.Error("expected non-zero default dimensions")
	}
}

func TestParseClientProfile_AllParams(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?maxWidth=1280&maxHeight=720&maxBitrate=5000000&videoCodecs=h264,hevc&audioCodecs=aac,opus&containers=mp4,mkv&hdr=true", nil)
	p := h.parseClientProfile(req)
	if p.MaxWidth != 1280 {
		t.Errorf("expected 1280, got %d", p.MaxWidth)
	}
	if p.MaxHeight != 720 {
		t.Errorf("expected 720, got %d", p.MaxHeight)
	}
	if p.MaxBitrate != 5000000 {
		t.Errorf("expected 5000000, got %d", p.MaxBitrate)
	}
	if len(p.VideoCodecs) != 2 || p.VideoCodecs[0] != "h264" {
		t.Errorf("unexpected video codecs: %v", p.VideoCodecs)
	}
	if len(p.AudioCodecs) != 2 {
		t.Errorf("unexpected audio codecs: %v", p.AudioCodecs)
	}
	if len(p.Containers) != 2 {
		t.Errorf("unexpected containers: %v", p.Containers)
	}
	if !p.SupportsHDR {
		t.Error("expected SupportsHDR=true")
	}
}

// The web and mobile clients send hdr=1 (the exact form the VOD path parses);
// this endpoint only accepted hdr=true, so every real client was treated as
// SDR here - a remux/playback-info decision then forced a tonemap for clients
// that decode HDR natively, contradicting the GetStreamInfo decision that had
// picked direct-stream for them.
func TestParseClientProfile_HDRNumericForm(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?hdr=1", nil)
	if p := h.parseClientProfile(req); !p.SupportsHDR {
		t.Error("hdr=1 (the form web/mobile actually send) must set SupportsHDR")
	}
}

func TestParseClientProfile_InvalidValues(t *testing.T) {
	h := &playbackServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?maxWidth=abc&maxHeight=-1&maxBitrate=bad", nil)
	p := h.parseClientProfile(req)
	assert.NotZero(t, p.MaxWidth, "invalid should keep default, which is non-zero")
	if p.SupportsHDR {
		t.Error("expected SupportsHDR=false without hdr=true")
	}
}

// ============================================================
// Plugin handleListThemes test
// ============================================================

func TestListThemes_OK(t *testing.T) {
	mux, token, repo, _ := setupPluginAssetTest(t)
	_ = repo.Create(context.Background(), &plugin.Plugin{
		ID: "theme1", Name: "Dark Theme", Kind: plugin.KindTheme, Enabled: true,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/themes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Dark Theme") {
		t.Error("expected theme name in response")
	}
}

func TestParseRootCSSVars(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "basic",
			in:   ":root { --a: red; --b: #fff; }",
			want: map[string]string{"--a": "red", "--b": "#fff"},
		},
		{
			name: "with comments",
			in:   "/* skip */ :root { /* inner */ --x: blue; }",
			want: map[string]string{"--x": "blue"},
		},
		{
			name: "ignores non-vars",
			in:   ":root { color: red; --keep: yellow; }",
			want: map[string]string{"--keep": "yellow"},
		},
		{
			name: "no root block",
			in:   "body { color: red; }",
			want: nil,
		},
		{
			name: "rgba value preserved",
			in:   ":root { --shadow: rgba(0, 0, 0, 0.4); }",
			want: map[string]string{"--shadow": "rgba(0, 0, 0, 0.4)"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseRootCSSVars([]byte(c.in))
			if len(got) != len(c.want) {
				t.Fatalf("got %d entries, want %d: %v", len(got), len(c.want), got)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("key %s: got %q want %q", k, got[k], v)
				}
			}
		})
	}
}

// ============================================================
// HLS master handler auth tests
// ============================================================

// ============================================================
// Photo handler additional tests
// ============================================================

func TestServePhoto_NotFound(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-nonexistent/serve", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass, got %d", w.Code)
	}
}

// ============================================================
// Backup handler additional tests
// ============================================================

func TestCreateBackup_Admin_201(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetBackup_NotFound_404(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteBackup_NotFound_404(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRestoreBackup_NoFile_400(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/restore", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDeleteBackup_AfterCreate(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	// create
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	backupID := created["id"].(string)

	// delete
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/"+backupID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestGetBackup_AfterCreate(t *testing.T) {
	mux, token := newBackupTestMux(t, true)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups", nil)
	createReq.Header.Set("Authorization", "Bearer "+token)
	cw := httptest.NewRecorder()
	mux.ServeHTTP(cw, createReq)
	if cw.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", cw.Code)
	}
	var created map[string]any
	json.NewDecoder(cw.Body).Decode(&created)
	backupID := created["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/"+backupID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "application/gzip" {
		t.Errorf("expected gzip content type, got %s", w.Header().Get("Content-Type"))
	}
}

// ============================================================
// buildFFmpegArgs test (transcode_handler.go - pure logic, no I/O)
// ============================================================
