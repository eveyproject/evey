package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupThemeAssetTest(t *testing.T) (*http.ServeMux, string, *pluginfakes.FakePluginRepository, string) {
	t.Helper()
	dir := t.TempDir()
	repo := pluginfakes.NewFakePluginRepository()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)

	// dark-theme: a valid theme plugin
	theme := &plugin.Plugin{
		ID:      "dark",
		Name:    "dark-theme",
		Kind:    plugin.KindTheme,
		Enabled: true,
	}
	require.NoError(t, repo.Create(context.Background(), theme))

	themeDir := filepath.Join(dir, "dark")
	require.NoError(t, os.MkdirAll(themeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(themeDir, "theme.css"), []byte(":root { --accent: blue; }"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(themeDir, "theme.js"), []byte("console.log('theme')"), 0o644))

	mux := http.NewServeMux()
	registerThemeAssetRoutes(mux, tokenGen, repo, dir)

	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user1", IsAdmin: false})
	require.NoError(t, err)
	return mux, token, repo, dir
}

// --- theme.css tests ---

func TestThemeCSS_Public_Success(t *testing.T) {
	// theme.css is public: no auth token required
	mux, _, _, _ := setupThemeAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/themes/dark/theme.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/css; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, w.Body.String(), "--accent")
}

func TestThemeCSS_NotTheme(t *testing.T) {
	mux, _, repo, _ := setupThemeAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "ext1", Name: "extension", Kind: plugin.KindPlugin, Enabled: true,
	}))

	req := httptest.NewRequest("GET", "/api/v1/themes/ext1/theme.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestThemeCSS_NotFound(t *testing.T) {
	mux, _, _, _ := setupThemeAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/themes/nonexistent/theme.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestThemeCSS_Disabled(t *testing.T) {
	mux, _, repo, _ := setupThemeAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "off", Name: "off-theme", Kind: plugin.KindTheme, Enabled: false,
	}))

	req := httptest.NewRequest("GET", "/api/v1/themes/off/theme.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestThemeCSS_FileNotFound(t *testing.T) {
	mux, _, repo, dir := setupThemeAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "nocss", Name: "no-css-theme", Kind: plugin.KindTheme, Enabled: true,
	}))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nocss"), 0o755))
	// Intentionally do NOT write theme.css

	req := httptest.NewRequest("GET", "/api/v1/themes/nocss/theme.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- theme.js tests ---

func TestThemeJS_Success(t *testing.T) {
	mux, token, _, _ := setupThemeAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/themes/dark/theme.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/javascript", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, themeCSPScript, w.Header().Get("Content-Security-Policy"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=3600")
	assert.Contains(t, w.Body.String(), "console.log")
}

func TestThemeJS_Unauthorized(t *testing.T) {
	mux, _, _, _ := setupThemeAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/themes/dark/theme.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestThemeJS_NotTheme(t *testing.T) {
	mux, token, repo, _ := setupThemeAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "ext2", Name: "extension2", Kind: plugin.KindPlugin, Enabled: true,
	}))

	req := httptest.NewRequest("GET", "/api/v1/themes/ext2/theme.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestThemeJS_FileNotFound(t *testing.T) {
	mux, token, repo, dir := setupThemeAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "nojs", Name: "no-js-theme", Kind: plugin.KindTheme, Enabled: true,
	}))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nojs"), 0o755))
	// Intentionally do NOT write theme.js

	req := httptest.NewRequest("GET", "/api/v1/themes/nojs/theme.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
