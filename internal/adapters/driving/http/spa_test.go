package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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

// Cache policy. Three classes of static files, three behaviors:
//
//   - /wasm/*: mutable files at FIXED urls whose two halves (ac3go.wasm +
//     wasm_exec.js shim) are only compatible within the same build GENERATION.
//     Heuristic browser caching (no Cache-Control at all) let a redeploy pair a
//     cached old shim with a fresh new wasm - instant LinkError, silent fallback
//     to the AAC transcode (prod 2026-07-26, the go->tinygo switch). no-cache
//     forces revalidation on every use; unchanged files still ride 304s.
//   - /index.html (and the SPA fallback): same mutable-at-fixed-url class - it
//     names the hashed bundles, so a stale one resurrects dead assets.
//   - /assets/*: Vite content-hashed filenames - a changed file IS a changed
//     URL, so cache forever.
func TestSPAHandler_WasmAssets_MustRevalidate(t *testing.T) {
	dir := setupSPADir(t)
	wasmDir := filepath.Join(dir, "wasm")
	if err := os.MkdirAll(wasmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ac3go.wasm", "wasm_exec.js", "ac3Worker.js"} {
		if err := os.WriteFile(filepath.Join(wasmDir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := spaHandler(dir)

	for _, f := range []string{"/wasm/ac3go.wasm", "/wasm/wasm_exec.js", "/wasm/ac3Worker.js"} {
		req := httptest.NewRequest(http.MethodGet, f, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", f, w.Code)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want %q (a stale wasm/shim mix breaks decode)", f, got, "no-cache")
		}
	}
}

func TestSPAHandler_Index_MustRevalidate(t *testing.T) {
	dir := setupSPADir(t)
	h := spaHandler(dir)

	for _, path := range []string{"/", "/foo/bar", "/index.html"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want %q (a stale index names dead hashed bundles)", path, got, "no-cache")
		}
	}
}

func TestSPAHandler_HashedAssets_CacheForever(t *testing.T) {
	dir := setupSPADir(t)
	h := spaHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("/assets/app.js: Cache-Control = %q, want immutable (Vite content-hashes the filename)", got)
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

// A missing /assets/ file must be a 404, never the SPA fallback. Vite
// content-hashes every asset filename, so a request for an asset the server
// does not have is ALWAYS a stale tab asking for a chunk a redeploy replaced.
// Serving index.html (200 text/html) instead made the dynamic import fail as
// a module error, which aborts the navigation silently: on the 2026-08-25
// prod, every home-page title (Films, Séries, Collections) "could not be
// clicked" in any tab opened before the day's four redeploys. A 404 lets the
// client's preload-error handler detect staleness and reload.
func TestSPAHandler_MissingAssetIs404NotIndexHTML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := spaHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "/assets/main-STALE123.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing asset must 404, got %d (the SPA fallback disguises a stale chunk as HTML)", rec.Code)
	}
}

// The browser TV client is a second SPA under /tv: its deep paths must open
// ITS index, and its hashed assets must get the same policy and the same
// staleness 404 as the web app's.
func setupTVDir(t *testing.T) string {
	t.Helper()
	dir := setupSPADir(t)
	tv := filepath.Join(dir, "tv")
	if err := os.MkdirAll(filepath.Join(tv, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tv, "index.html"), []byte("<html>TV-INDEX</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tv, "assets", "tv.js"), []byte("console.log('tv')"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSPAHandler_SubdirectoryClient(t *testing.T) {
	dir := setupTVDir(t)
	h := spaHandler(dir)

	cases := []struct {
		path      string
		status    int
		body      string
		cacheCtrl string
	}{
		{path: "/tv/anything", status: http.StatusOK, body: "TV-INDEX", cacheCtrl: "no-cache"},
		{path: "/tv/assets/tv.js", status: http.StatusOK, body: "console.log('tv')", cacheCtrl: "public, max-age=31536000, immutable"},
		{path: "/tv/assets/gone.js", status: http.StatusNotFound},
		{path: "/foo/bar", status: http.StatusOK, body: "SPA-INDEX", cacheCtrl: "no-cache"},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.path, nil))
		if w.Code != c.status {
			t.Fatalf("%s: expected %d, got %d", c.path, c.status, w.Code)
		}
		if c.body != "" && !strings.Contains(w.Body.String(), c.body) {
			t.Errorf("%s: expected %q, got %q", c.path, c.body, w.Body.String())
		}
		if c.cacheCtrl != "" && w.Header().Get("Cache-Control") != c.cacheCtrl {
			t.Errorf("%s: expected Cache-Control %q, got %q", c.path, c.cacheCtrl, w.Header().Get("Cache-Control"))
		}
	}
}

// The two ways a viewer types the browser TV's address. /tv/ came back as a
// relative redirect to "tv/", so a browser landed on /tv/tv/: the TV index
// loaded, its router matched nothing under /tv/tv and the page stayed an empty
// dark screen. Following redirects the way a browser does, both must end on
// /tv/ with the TV index.
func TestSPAHandler_SubdirectoryClientEntryPoints(t *testing.T) {
	dir := setupTVDir(t)
	h := spaHandler(dir)

	for _, start := range []string{"/tv", "/tv/"} {
		u, _ := url.Parse("https://evey.test" + start)
		var w *httptest.ResponseRecorder
		for hop := 0; ; hop++ {
			if hop > 3 {
				t.Fatalf("%s: more than 3 redirects, last at %s", start, u.Path)
			}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, u.String(), nil))
			if w.Code < 300 || w.Code >= 400 {
				break
			}
			loc, err := url.Parse(w.Header().Get("Location"))
			if err != nil {
				t.Fatalf("%s: bad Location %q", start, w.Header().Get("Location"))
			}
			u = u.ResolveReference(loc)
		}
		if u.Path != "/tv/" {
			t.Errorf("%s: a browser ends on %s, want /tv/", start, u.Path)
		}
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "TV-INDEX") {
			t.Errorf("%s: expected 200 with the TV index, got %d %q", start, w.Code, w.Body.String())
		}
	}
}
