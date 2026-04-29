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

	appplugin "github.com/evey-project/evey/internal/app/plugin"
	"github.com/evey-project/evey/internal/domain/plugin"
	pluginfakes "github.com/evey-project/evey/internal/domain/plugin/fakes"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
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
		Name:         "test-plugin",
		Enabled:      true,
		Capabilities: []string{"ui.extension"},
		Frontend:     []plugin.FrontModule{{Slot: "player.overlay", Src: "front.jsx"}},
	}
	require.NoError(t, repo.Create(context.Background(), p))

	pluginDir := filepath.Join(dir, "p1")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "front.js"), []byte("export default function(){}"), 0o644))

	mux := http.NewServeMux()
	registerPluginAssetRoutes(mux, tokenGen, repo, dir, func(data []byte) ([]byte, []string) { return data, nil })

	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user1", IsAdmin: false})
	require.NoError(t, err)
	return mux, token, repo, dir
}

func TestFrontJS_Success(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/front.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/javascript", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=3600")
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "script-src")
	assert.Contains(t, w.Body.String(), "export default")
}

func TestFrontJS_Unauthorized(t *testing.T) {
	mux, _, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/front.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestFrontJS_PluginNotFound(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/nonexistent/front.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFrontJS_PluginDisabled(t *testing.T) {
	mux, token, repo, _ := setupPluginAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "p2", Name: "disabled", Enabled: false,
		Frontend: []plugin.FrontModule{{Slot: "player.overlay", Src: "front.jsx"}},
	}))

	req := httptest.NewRequest("GET", "/api/v1/plugins/p2/front.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFrontJS_NoFrontend(t *testing.T) {
	mux, token, repo, _ := setupPluginAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "p3", Name: "no-ui", Enabled: true,
		Capabilities: []string{"backend"},
	}))

	req := httptest.NewRequest("GET", "/api/v1/plugins/p3/front.js", nil)
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
	registerPluginPreviewRoute(mux, tokenGen, svc, ratelimit.New(100))
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
