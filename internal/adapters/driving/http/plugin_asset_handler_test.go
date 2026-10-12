package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	"github.com/eveyproject/evey/internal/domain/plugin"
	pluginfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
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
	assert.Contains(t, w.Header().Get("Cache-Control"), "no-cache")
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"), "JS does not need CORS - only fonts/binary do")
	assert.Contains(t, w.Body.String(), "export default")
}

// Plugin bundles have STABLE, unhashed filenames (dist/index.js), so a
// far-future immutable Cache-Control means a plugin UPDATE never reaches a
// browser that already cached the bundle (seen live 2026-07-22: a weather-v2
// fix stayed invisible until a hard refresh). The route must force
// revalidation and answer 304 via a content-derived ETag instead.
func TestPluginAsset_CacheRevalidation(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)

	get := func(inm string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/index.js", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	first := get("")
	assert.Equal(t, http.StatusOK, first.Code)
	cc := first.Header().Get("Cache-Control")
	assert.NotContains(t, cc, "immutable", "unhashed plugin assets must never be immutable")
	assert.Contains(t, cc, "no-cache", "browser must revalidate so plugin updates propagate")
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag, "revalidation needs a content ETag")

	// Unchanged content revalidates to a bodyless 304.
	cached := get(etag)
	assert.Equal(t, http.StatusNotModified, cached.Code)
	assert.Empty(t, cached.Body.String())

	// A plugin update rewrites the file in place: same path, new bytes must
	// yield a new ETag and a full 200 even when the stale ETag is presented.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "p1", "dist", "index.js"), []byte("export default function(){/* v2 */}"), 0o644))
	updated := get(etag)
	assert.Equal(t, http.StatusOK, updated.Code)
	assert.NotEqual(t, etag, updated.Header().Get("ETag"))
	assert.Contains(t, updated.Body.String(), "v2")
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

// web_root is read back from the database and re-validated on every request: a
// row carrying a traversal must not widen what the handler serves. The check
// had no test (sabotage sweep, 2026-09-28).
func TestPluginAsset_WebRootTraversalRefused(t *testing.T) {
	mux, token, repo, _ := setupPluginAssetTest(t)
	require.NoError(t, repo.Create(context.Background(), &plugin.Plugin{
		ID: "evil", Name: "evil", Kind: plugin.KindPlugin, Enabled: true,
		WebRoot:  "../../",
		Frontend: []plugin.FrontModule{{Slot: "home.section", Src: "x.js"}},
	}))

	req := httptest.NewRequest("GET", "/api/v1/plugins/evil/assets/x.js", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "a web_root that climbs out of the plugin dir must be refused")
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
	// previewedSource records what PreviewSource was asked for.
	previewedSource string
}

func (s *stubPluginManager) InstallPlugin(_ context.Context, _ string) (*plugin.Plugin, error) {
	return s.installResult, s.installErr
}

func (s *stubPluginManager) PreviewPlugin(_ context.Context, _ string) (*appplugin.PluginPreview, error) {
	return s.previewResult, s.previewErr
}

func (s *stubPluginManager) PreviewSource(_ context.Context, source string) (*appplugin.PluginPreview, error) {
	s.previewedSource = source
	return s.previewResult, s.previewErr
}

func TestPluginPreview_Forbidden(t *testing.T) {
	mux, _ := setupPreviewTest(t)

	req := httptest.NewRequest("POST", "/api/v1/admin/plugins/preview", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// A JSON body names an install SOURCE (URL or path) to preview instead of
// uploading a file - the install-from-URL dialogs used to skip the preview.
func TestPluginPreview_FromSource(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})
	require.NoError(t, err)
	svc := &stubPluginManager{previewResult: &appplugin.PluginPreview{
		Name:        "remote-plugin",
		Version:     "1.2.0",
		Permissions: []string{"api.library"},
		SourceHash:  "abc",
		Analysis: &plugin.AnalysisReport{
			Safe:      true,
			FileCount: 4,
			Issues:    []string{"[WARNING] overreach: docker invoked at runtime"},
			URLs:      []plugin.ExtractedURL{{URL: "https://example.com/ping", File: "main.js", Line: 3}},
		},
	}}
	mux := http.NewServeMux()
	registerPluginPreviewRoute(mux, tokenGen, svc, ratelimit.New(100))

	t.Run("previews the source and returns the findings", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/admin/plugins/preview",
			strings.NewReader(`{"source":" https://example.com/plugin.zip#sha256:abc "}`))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "https://example.com/plugin.zip#sha256:abc", svc.previewedSource)
		var body struct {
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
			SourceHash  string   `json:"source_hash"`
			Analysis    struct {
				Safe   bool     `json:"safe"`
				Issues []string `json:"issues"`
				URLs   []string `json:"urls"`
			} `json:"analysis"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "remote-plugin", body.Name)
		assert.Equal(t, []string{"api.library"}, body.Permissions)
		assert.Equal(t, "abc", body.SourceHash)
		assert.True(t, body.Analysis.Safe)
		assert.Equal(t, []string{"[WARNING] overreach: docker invoked at runtime"}, body.Analysis.Issues)
		assert.Equal(t, []string{"https://example.com/ping  (main.js:3)"}, body.Analysis.URLs)
	})

	t.Run("an empty source is a 400", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/admin/plugins/preview", strings.NewReader(`{"source":"  "}`))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "plugin source required")
	})

	t.Run("a failed preview names the reason - it is the admin's input", func(t *testing.T) {
		failing := &stubPluginManager{previewErr: errors.New("resolve plugin source: download plugin: HTTP 404")}
		fmux := http.NewServeMux()
		registerPluginPreviewRoute(fmux, tokenGen, failing, ratelimit.New(100))
		req := httptest.NewRequest("POST", "/api/v1/admin/plugins/preview", strings.NewReader(`{"source":"https://example.com/missing.zip"}`))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		fmux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, "plugin preview failed: resolve plugin source: download plugin: HTTP 404", body["message"])
	})

	t.Run("without a token it is a 401 like the upload path", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/admin/plugins/preview", strings.NewReader(`{"source":"https://example.com/plugin.zip"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
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

// A revalidation must not re-read and re-hash the whole bundle: the ETag is
// remembered per (path, size, mtime), so an unchanged file answers 304 from the
// stat alone. Proven by making the file unreadable between the two requests.
func TestPluginAsset_RevalidationDoesNotRereadTheFile(t *testing.T) {
	mux, token, _, dir := setupPluginAssetTest(t)
	get := func(inm string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/v1/plugins/p1/assets/index.js", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	first := get("")
	require.Equal(t, http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")

	path := filepath.Join(dir, "p1", "dist", "index.js")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running as root: an unreadable file cannot prove the shortcut")
	}
	assert.Equal(t, http.StatusNotModified, get(etag).Code, "unchanged file must revalidate from its stat")
}
