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

	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	"github.com/gravity-zero/evey/internal/domain/plugin"
	pluginfakes "github.com/gravity-zero/evey/internal/domain/plugin/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPluginAssetTest(t *testing.T) (*http.ServeMux, string, *pluginfakes.FakePluginRepository, string) {
	t.Helper()
	dir := t.TempDir()
	repo := pluginfakes.NewFakePluginRepository()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)

	p := &plugin.Plugin{
		ID:           "p1",
		Name:         "test-widget",
		Enabled:      true,
		Capabilities: []string{"ui.extension"},
		UIWidgets:    []plugin.UIWidget{{Slot: "player.overlay", Src: "widget.jsx"}},
	}
	require.NoError(t, repo.Create(context.Background(), p))

	pluginDir := filepath.Join(dir, "p1")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "widget.bundle.js"), []byte("export default function(){}"), 0o644))

	mux := http.NewServeMux()
	registerPluginAssetRoutes(mux, tokenGen, repo, dir)

	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user1", IsAdmin: false})
	require.NoError(t, err)
	return mux, token, repo, dir
}

func TestWidgetJS_Success(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/widget.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/javascript", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=3600")
	assert.Contains(t, w.Body.String(), "export default")
}

func TestWidgetJS_Unauthorized(t *testing.T) {
	mux, _, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/widget.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWidgetJS_PluginNotFound(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/nonexistent/widget.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestWidgetJS_PluginDisabled(t *testing.T) {
	mux, token, repo, _ := setupPluginAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "p2", Name: "disabled", Enabled: false,
	}))

	req := httptest.NewRequest("GET", "/api/v1/plugins/p2/widget.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestThemeCSS_Success(t *testing.T) {
	mux, token, repo, dir := setupPluginAssetTest(t)

	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "theme1", Name: "dark-theme", Enabled: true, Capabilities: []string{"ui.theme"},
	}))
	themeDir := filepath.Join(dir, "theme1")
	require.NoError(t, os.MkdirAll(themeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(themeDir, "theme.css"), []byte(":root { --accent: red; }"), 0o644))

	req := httptest.NewRequest("GET", "/api/v1/plugins/theme1/theme.css", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/css", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "--accent")
}

func TestThemeCSS_NoThemeCapability(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	// p1 has ui.extension, not ui.theme
	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/theme.css", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- preview endpoint tests ---

func setupPreviewTest(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	require.NoError(t, err)

	svc := &stubPluginManager{}
	mux := http.NewServeMux()
	registerPluginPreviewRoute(mux, tokenGen, svc)
	return mux, adminToken
}

type stubPluginManager struct {
	installResult *plugin.Plugin
	installErr    error
	previewResult *appplugin.PluginPreview
	previewErr    error
}

func (s *stubPluginManager) InstallPlugin(_ context.Context, _ string) (*plugin.Plugin, error) {
	return s.installResult, s.installErr
}

func (s *stubPluginManager) PreviewPlugin(_ context.Context, _ string) (*appplugin.PluginPreview, error) {
	return s.previewResult, s.previewErr
}

func TestPluginPreview_Forbidden(t *testing.T) {
	mux, _ := setupPreviewTest(t)

	req := httptest.NewRequest("POST", "/api/v1/admin/plugins/preview", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPluginPreview_NoFile(t *testing.T) {
	mux, token := setupPreviewTest(t)

	req := httptest.NewRequest("POST", "/api/v1/admin/plugins/preview", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
