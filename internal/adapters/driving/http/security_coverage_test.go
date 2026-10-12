package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	shared "github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
)

// --- shared helpers ---

// =====================================
// cookie_middleware.go
// =====================================

func TestAuthCookieMiddleware_SetsCookieAndKeepsBody(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"at","refreshToken":"rt-value"}`))
	})
	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "refreshToken") {
		t.Error("refreshToken should remain in body for native clients")
	}
	var foundCookie bool
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName && c.Value == "rt-value" {
			foundCookie = true
		}
	}
	if !foundCookie {
		t.Error("expected refresh cookie to be set")
	}
}

func TestAuthCookieMiddleware_NoRefreshToken_PassThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accessToken":"at"}`))
	})
	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "accessToken") {
		t.Error("body should be passed through")
	}
}

func TestAuthCookieMiddleware_NonJSON_PassThrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	})
	handler := authCookieMiddleware(inner, false)
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Body.String() != "not json" {
		t.Error("non-json should pass through")
	}
}

func TestInjectRefreshTokenFromCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie-rt"})
	r2 := injectRefreshTokenFromCookie(req)
	var body map[string]any
	if err := json.NewDecoder(r2.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["refreshToken"] != "cookie-rt" {
		t.Errorf("expected refreshToken=cookie-rt, got %v", body["refreshToken"])
	}
}

func TestInjectRefreshTokenFromCookie_NoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{}`))
	r2 := injectRefreshTokenFromCookie(req)
	if r2 != req {
		t.Error("without cookie, should return same request")
	}
}

// =====================================
// helpers.go - safeJoin, sanitizeFilename, jsonEncodeResponse, setCORSHeaders, serveFileWithCache
// =====================================

func TestSafeJoin_PathTraversal(t *testing.T) {
	_, err := safeJoin("/tmp/safe", "../../etc/passwd")
	if err == nil {
		t.Error("expected path traversal error")
	}
}

func TestSafeJoin_Valid(t *testing.T) {
	path, err := safeJoin("/tmp/safe", "file.txt")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, "file.txt") {
		t.Errorf("expected path ending with file.txt, got %q", path)
	}
}

func TestSanitizeFilename_PathChars(t *testing.T) {
	got := sanitizeFilename("../../etc/passwd")
	if strings.Contains(got, "/") {
		t.Errorf("should sanitize / chars, got %q", got)
	}
	if got == "" {
		t.Error("should not produce empty string")
	}
}

func TestSanitizeFilename_Empty(t *testing.T) {
	got := sanitizeFilename("")
	if got != "download" {
		t.Errorf("expected 'download', got %q", got)
	}
}

// =====================================
// libraryStatsHandler - extra coverage
// =====================================

func TestLibraryStatsHandler_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(verifier)(libraryStatsHandler(
		&fakeLibraryLister{},
		nil,
	))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats", nil)
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLibraryStatsHandler_QueryToken(t *testing.T) {
	h := libraryStatsHandler(
		&fakeLibraryLister{},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/library-stats?token=valid", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// =====================================
// helpers.go - exported middleware functions, tokenRedactMiddleware, securityHeaders
// =====================================

func TestTokenRedactMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// token should be in Authorization header now
		if r.Header.Get("Authorization") != "Bearer mytoken" {
			t.Errorf("expected Bearer mytoken, got %q", r.Header.Get("Authorization"))
		}
		// query param should be stripped
		if r.URL.Query().Get("token") != "" {
			t.Error("token query param should be removed")
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := tokenRedactMiddleware(nil, false, inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media?token=mytoken", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTokenRedactMiddleware_ExistingAuthHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer existing" {
			t.Errorf("existing header should not be overwritten, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := tokenRedactMiddleware(nil, false, inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media?token=mytoken", nil)
	req.Header.Set("Authorization", "Bearer existing")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
}

func TestSecurityHeadersMiddleware_Coverage(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := securityHeadersMiddleware(false, inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected nosniff header")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected DENY frame options")
	}
}

// =====================================
// server.go coverage - RegisterRoutes not feasible without many deps
// but we can test the exported middleware wrappers
// =====================================

func TestTokenRedactMiddleware_Exported(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := TokenRedactMiddleware(inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSecurityHeadersMiddleware_Exported(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := SecurityHeadersMiddleware(false, inner)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected nosniff")
	}
}

func TestAvailableRuntimes_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)

	mux := http.NewServeMux()
	registerAvailableRuntimesRoute(mux, gen)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/plugin-runtimes/available", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ============================================================
// clearOIDCIDTokenCookie test
// ============================================================

func TestClearOIDCIDTokenCookie(t *testing.T) {
	w := httptest.NewRecorder()
	clearOIDCIDTokenCookie(w, true)
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == oidcIDTokenCookieName {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("expected MaxAge -1, got %d", c.MaxAge)
			}
			if !c.Secure {
				t.Error("expected Secure=true")
			}
		}
	}
	if !found {
		t.Error("expected OIDC cookie to be set")
	}
}

func TestClearOIDCIDTokenCookie_HTTP(t *testing.T) {
	w := httptest.NewRecorder()
	clearOIDCIDTokenCookie(w, false)
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == oidcIDTokenCookieName && c.Secure {
			t.Error("expected Secure=false for http")
		}
	}
}

func TestTokenRedactMiddlewareWithStore_OpaqueToken(t *testing.T) {
	st := &shared.StreamToken{UserID: "u1", MediaItemID: "m1"}
	store := &fakeStreamTokenValidator{token: st}

	var capturedCtx context.Context
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
	})
	handler := TokenRedactMiddlewareWithStore(store, false, inner)

	req := httptest.NewRequest(http.MethodGet, "/test?token=opaque-no-dots", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	got, ok := StreamTokenFromContext(capturedCtx)
	if !ok {
		t.Fatal("expected stream token in context")
	}
	if got.MediaItemID != "m1" {
		t.Errorf("expected m1, got %s", got.MediaItemID)
	}
}

func TestTokenRedactMiddlewareWithStore_JWTFallback(t *testing.T) {
	store := &fakeStreamTokenValidator{token: nil}

	var capturedAuth string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
	})
	handler := TokenRedactMiddlewareWithStore(store, false, inner)

	req := httptest.NewRequest(http.MethodGet, "/test?token=jwt.with.dots", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if capturedAuth != "Bearer jwt.with.dots" {
		t.Errorf("expected JWT in auth header, got %q", capturedAuth)
	}
}

// Plan B P1: the stream token also resolves from the evey_hls cookie (native
// <video>/hls.js) and from an opaque Bearer header (mobile expo-video), scoped
// to /api/v1/media/*, and an HLS request (re)issues the httpOnly cookie.
func TestTokenRedactMiddleware_CookieAndBearerTransports(t *testing.T) {
	st := &shared.StreamToken{UserID: "u1", MediaItemID: "m1", TTL: 5 * time.Minute}
	store := &fakeStreamTokenValidator{token: st}
	newHandler := func() (http.Handler, *context.Context) {
		var captured context.Context
		inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { captured = r.Context() })
		return TokenRedactMiddlewareWithStore(store, false, inner), &captured
	}
	hlsCookieOf := func(w *httptest.ResponseRecorder) *http.Cookie {
		for _, c := range w.Result().Cookies() {
			if c.Name == "evey_hls" {
				return c
			}
		}
		return nil
	}

	t.Run("cookie resolves token and master (re)issues it", func(t *testing.T) {
		h, ctx := newHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8", nil)
		req.AddCookie(&http.Cookie{Name: "evey_hls", Value: "opaque-cookie"})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if _, ok := StreamTokenFromContext(*ctx); !ok {
			t.Fatal("cookie transport: expected stream token in context")
		}
		c := hlsCookieOf(w)
		if c == nil {
			t.Fatal("expected evey_hls Set-Cookie on HLS response")
		}
		if c.Path != "/api/v1/media/m1/hls" {
			t.Errorf("cookie path = %q, want /api/v1/media/m1/hls", c.Path)
		}
		if !c.HttpOnly {
			t.Error("cookie must be HttpOnly")
		}
	})

	t.Run("opaque Bearer resolves token (mobile)", func(t *testing.T) {
		h, ctx := newHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/seg_00001.m4s", nil)
		req.Header.Set("Authorization", "Bearer opaque-bearer")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if _, ok := StreamTokenFromContext(*ctx); !ok {
			t.Error("bearer transport: expected stream token in context")
		}
	})

	t.Run("real JWT Bearer is not treated as a stream token", func(t *testing.T) {
		h, ctx := newHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8", nil)
		req.Header.Set("Authorization", "Bearer header.body.sig")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if _, ok := StreamTokenFromContext(*ctx); ok {
			t.Error("JWT bearer must not resolve as an opaque stream token")
		}
	})

	t.Run("cookie is ignored off the media paths", func(t *testing.T) {
		h, ctx := newHandler()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/me", nil)
		req.AddCookie(&http.Cookie{Name: "evey_hls", Value: "opaque-cookie"})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if _, ok := StreamTokenFromContext(*ctx); ok {
			t.Error("cookie must not authenticate non-media paths")
		}
	})
}

func TestListThemes_NoAuth_401(t *testing.T) {
	mux, _, _, _ := setupPluginAssetTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/themes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListThemes_IncludesParsedTokens(t *testing.T) {
	mux, token, repo, dir := setupPluginAssetTest(t)
	_ = repo.Create(context.Background(), &plugin.Plugin{
		ID: "themeWithCSS", Name: "Mid Theme", Kind: plugin.KindTheme, Enabled: true,
	})
	themeDir := filepath.Join(dir, "themeWithCSS")
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	css := []byte("/* hi */\n:root {\n  --bg: #112233;\n  --accent: #ff00ff;\n  --text: #eee;\n}\n")
	if err := os.WriteFile(filepath.Join(themeDir, "theme.css"), css, 0o644); err != nil {
		t.Fatalf("write theme.css: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/themes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"--bg":"#112233"`, `"--accent":"#ff00ff"`, `"--text":"#eee"`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
}

// ============================================================
// Plugin upload route test
// ============================================================

func TestPluginUpload_NoAuth_401(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	svc := &stubPluginManager{}
	mux := http.NewServeMux()
	registerPluginUploadRoute(mux, gen, svc, ratelimit.New(100))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plugins/upload", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetBackup_NoAuth_401(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backups/b1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDeleteBackup_NoAuth_401(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/backups/b1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRestoreBackup_NoAuth_401(t *testing.T) {
	mux, _ := newBackupTestMux(t, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/restore", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
