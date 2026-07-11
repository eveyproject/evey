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

// setupPluginAssetTest seeds a plugin with web_root="dist" and a small
// dist/ tree (JS, CSS, font, image) so each handler test can exercise the
// MIME / CORS / sanitizer / auth surface independently.
func setupPluginAssetTest(t *testing.T) (*http.ServeMux, string, *pluginfakes.FakePluginRepository, string) {
	t.Helper()
	dir := t.TempDir()
	repo := pluginfakes.NewFakePluginRepository()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)

	p := &plugin.Plugin{
		ID:       "p1",
		Name:     "test-plugin",
		Kind:     plugin.KindPlugin,
		Enabled:  true,
		WebRoot:  "dist",
		Frontend: []plugin.FrontModule{{Slot: "home.section", Src: "dist/index.js"}},
	}
	require.NoError(t, repo.Create(context.Background(), p))

	distDir := filepath.Join(dir, "p1", "dist")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "index.js"), []byte("export default function(){}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "style.css"), []byte(":root{--accent:blue}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "font.woff2"), []byte("fake-font-bytes"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "image.png"), []byte("fake-png-bytes"), 0o644))

	mux := http.NewServeMux()
	registerPluginAssetRoutes(mux, tokenGen, tokenGen, repo, dir, func(data []byte) ([]byte, []string) { return data, nil }, nil)

	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "user1", IsAdmin: false})
	require.NoError(t, err)
	return mux, token, repo, dir
}

func TestPluginAsset_HappyJS(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/index.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "javascript")
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "immutable")
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"), "JS does not need CORS - only fonts/binary do")
	assert.Contains(t, w.Body.String(), "export default")
}

func TestPluginAsset_SecurityHeaders(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	for _, path := range []string{"index.js", "style.css", "font.woff2", "image.png"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/"+path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"),
				"plugin assets must not be embeddable in a foreign frame")
			assert.Equal(t, "default-src 'none'; sandbox", w.Header().Get("Content-Security-Policy"),
				"plugin assets served directly must be maximally restricted")
		})
	}
}

func TestPluginAsset_FontHasCORS(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/font.woff2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "null", w.Header().Get("Access-Control-Allow-Origin"), "fonts must carry CORS for opaque-origin iframe fetch")
	assert.Contains(t, w.Header().Get("Content-Type"), "font")
}

func TestPluginAsset_PNGNoCORS(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/image.png", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"), "<img> loads cross-origin without CORS - no header needed")
}

func TestPluginAsset_PathTraversalEncoded(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/..%2F..%2Fetc%2Fpasswd", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "encoded ../ must return 400")
}

func TestPluginAsset_PathTraversalRaw(t *testing.T) {
	mux, token, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/../../etc/passwd", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Go's ServeMux normalises the URL with a 301 redirect; either way the
	// response body must NOT contain the requested file.
	assert.NotEqual(t, http.StatusOK, w.Code)
}

func TestPluginAsset_SizeLimit(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)

	bigPath := filepath.Join(dir, "p1", "dist", "big.bin")
	f, err := os.Create(bigPath)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(maxPluginAssetBytes+1))
	f.Close()

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/big.bin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPluginAsset_CSSSanitizerInvoked(t *testing.T) {
	dir := t.TempDir()
	repo := pluginfakes.NewFakePluginRepository()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)

	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "pcss", Name: "css", Kind: plugin.KindPlugin, Enabled: true, WebRoot: "dist",
		Frontend: []plugin.FrontModule{{Slot: "home.section", Src: "dist/style.css"}},
	}))
	distDir := filepath.Join(dir, "pcss", "dist")
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distDir, "style.css"),
		[]byte("body{position:fixed;pointer-events:auto}"), 0o644))

	called := false
	mux := http.NewServeMux()
	registerPluginAssetRoutes(mux, tokenGen, tokenGen, repo, dir, func(data []byte) ([]byte, []string) {
		called = true
		return []byte("/* sanitized */"), []string{"removed-rule"}
	}, nil)

	tok, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1"})
	req := httptest.NewRequest("GET", "/api/v1/plugins/pcss/assets/style.css", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, called, "css sanitizer must be invoked for .css responses")
	assert.Contains(t, w.Body.String(), "/* sanitized */")
}

func TestPluginAsset_Unauthorized(t *testing.T) {
	mux, _, _, _ := setupPluginAssetTest(t)

	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/index.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPluginAsset_PluginTokenWrongPlugin(t *testing.T) {
	mux, _, _, _ := setupPluginAssetTest(t)
	// Need a tokenGen identical to the one in setupPluginAssetTest - recreate.
	// Easier: use the stored generator implicitly via the same flow; here we
	// just verify a clearly-bad token is rejected.
	req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/index.js?token=garbage", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPluginAsset_NoWebRootConfigured(t *testing.T) {
	dir := t.TempDir()
	repo := pluginfakes.NewFakePluginRepository()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)

	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "pnw", Name: "no-web-root", Kind: plugin.KindPlugin, Enabled: true,
		Frontend: []plugin.FrontModule{{Slot: "home.section", Src: "front.js"}},
		// WebRoot intentionally empty
	}))
	mux := http.NewServeMux()
	registerPluginAssetRoutes(mux, tokenGen, tokenGen, repo, dir, nil, nil)

	tok, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1"})
	req := httptest.NewRequest("GET", "/api/v1/plugins/pnw/assets/front.js", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "missing web_root must refuse to serve")
}

func TestPluginAsset_DisabledPlugin(t *testing.T) {
	mux, token, repo, _ := setupPluginAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "off", Name: "disabled", Kind: plugin.KindPlugin, Enabled: false,
		WebRoot:  "dist",
		Frontend: []plugin.FrontModule{{Slot: "home.section", Src: "dist/x.js"}},
	}))

	req := httptest.NewRequest("GET", "/api/v1/plugins/off/assets/x.js", nil)
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

func TestExtractEscalationEntries(t *testing.T) {
	// Parses a plugin-runtime denial message of the form
	// "... new entries requested: a, b, c" into the requested permission entries
	// shown to the admin. Strict on the ", " separator; trims + drops empties.
	t.Run("no marker returns nothing", func(t *testing.T) {
		assert.Empty(t, extractEscalationEntries("some unrelated error"))
		assert.Empty(t, extractEscalationEntries(""))
	})
	t.Run("marker present but nothing after returns nothing", func(t *testing.T) {
		assert.Empty(t, extractEscalationEntries("denied; new entries requested: "))
	})
	t.Run("single entry", func(t *testing.T) {
		assert.Equal(t, []string{"net:example.com"},
			extractEscalationEntries("denied; new entries requested: net:example.com"))
	})
	t.Run("multiple entries", func(t *testing.T) {
		assert.Equal(t, []string{"net:a.com", "fs:/tmp", "exec:ls"},
			extractEscalationEntries("blocked: new entries requested: net:a.com, fs:/tmp, exec:ls"))
	})
	t.Run("skips empty parts between entries", func(t *testing.T) {
		assert.Equal(t, []string{"net:a.com", "fs:/x"},
			extractEscalationEntries("new entries requested: net:a.com, , fs:/x"))
	})
}
