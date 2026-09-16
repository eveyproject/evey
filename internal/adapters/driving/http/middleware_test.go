package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenRedactMiddleware_MovesQueryToHeader(t *testing.T) {
	var gotHeader string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		if r.URL.Query().Get("token") != "" {
			t.Error("token query param should be removed")
		}
	})
	h := tokenRedactMiddleware(nil, false, next)
	req := httptest.NewRequest(http.MethodGet, "/foo?token=mytoken", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotHeader != "Bearer mytoken" {
		t.Errorf("expected 'Bearer mytoken', got %q", gotHeader)
	}
}

func TestTokenRedactMiddleware_DoesNotOverwriteHeader(t *testing.T) {
	var gotHeader string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
	})
	h := tokenRedactMiddleware(nil, false, next)
	req := httptest.NewRequest(http.MethodGet, "/foo?token=querytoken", nil)
	req.Header.Set("Authorization", "Bearer headertoken")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if gotHeader != "Bearer headertoken" {
		t.Errorf("expected header token to win, got %q", gotHeader)
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := securityHeadersMiddleware(false, next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options")
	}
	if w.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Error("missing Referrer-Policy")
	}
}

// The CSP must allow BOTH the ac3go wasm decoder ('wasm-unsafe-eval') and the
// Google Cast sender SDK (script-src https://www.gstatic.com). Tightening the
// policy for the wasm path once silently broke Chromecast for every user:
// cast_sender.js was refused, castSdkReady stayed false, and no cast button
// ever appeared. This test pins the two directives together so neither can
// regress the other.
func TestSecurityHeadersMiddleware_CSPAllowsWasmAndCastSDK(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := securityHeadersMiddleware(false, next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	csp := w.Header().Get("Content-Security-Policy")
	scriptSrc := ""
	for directive := range strings.SplitSeq(csp, ";") {
		if d := strings.TrimSpace(directive); strings.HasPrefix(d, "script-src ") {
			scriptSrc = d
		}
	}
	if scriptSrc == "" {
		t.Fatalf("no script-src directive in CSP %q", csp)
	}
	if !strings.Contains(scriptSrc, "'wasm-unsafe-eval'") {
		t.Errorf("script-src must keep 'wasm-unsafe-eval' (ac3go wasm decoder), got %q", scriptSrc)
	}
	if !strings.Contains(scriptSrc, "https://www.gstatic.com") {
		t.Errorf("script-src must allow https://www.gstatic.com (Cast sender SDK), got %q", scriptSrc)
	}
	// Plugin frontends are inlined into a srcdoc iframe, which inherits THIS
	// policy - without the token every plugin UI silently renders blank.
	if !strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Errorf("script-src must keep 'unsafe-inline' (plugin srcdoc bundles), got %q", scriptSrc)
	}
}

// base-uri and object-src have no fallback to default-src, so a policy that
// omits them leaves <base href> injection and <object>/<embed> plugin embedding
// unrestricted (flagged by the ZAP passive/active scan, 2026-09-06). Both the
// app and the cast-receiver page must carry the locked-down values. NOTE: we do
// NOT add frame-ancestors here - plugin frontends render in srcdoc iframes that
// inherit this policy, and 'none' would blank them (see the unsafe-inline note
// above); framing is covered by X-Frame-Options: DENY on the top-level response.
func TestSecurityHeadersMiddleware_CSPBaseAndObjectLockedDown(t *testing.T) {
	for _, path := range []string{"/", "/cast-receiver.html"} {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		h := securityHeadersMiddleware(false, next)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		csp := w.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "base-uri 'self'") {
			t.Errorf("%s: CSP must set base-uri 'self', got %q", path, csp)
		}
		if !strings.Contains(csp, "object-src 'none'") {
			t.Errorf("%s: CSP must set object-src 'none', got %q", path, csp)
		}
	}
}

// hls.js attaches the MediaSource through a blob: URL and runs its demuxer in a
// blob: worker. Neither directive was present, so both fell back to
// `default-src 'self'`, which does not match blob: - MSE playback dies.
//
// It looked fine only because the shipped Caddy strips this header and emits a
// complete policy of its own (docker/Caddyfile). Anyone behind Nginx Proxy
// Manager or Traefik passing the app header through - which docs/reverse-proxy.md
// explicitly tells them is enough - got a silently broken player. The app must
// emit a policy that is correct on its own.
func TestSecurityHeadersMiddleware_CSPAllowsBlobMediaAndWorker(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := securityHeadersMiddleware(false, next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	csp := w.Header().Get("Content-Security-Policy")

	directive := func(name string) string {
		for d := range strings.SplitSeq(csp, ";") {
			if t := strings.TrimSpace(d); strings.HasPrefix(t, name+" ") {
				return t
			}
		}
		return ""
	}
	for _, name := range []string{"media-src", "worker-src"} {
		got := directive(name)
		if got == "" {
			t.Errorf("CSP has no %s directive, so it falls back to default-src 'self' and blocks blob: - full CSP %q", name, csp)
			continue
		}
		if !strings.Contains(got, "blob:") {
			t.Errorf("%s must allow blob:, got %q", name, got)
		}
	}
}

func TestApiCacheMiddleware_DefaultNoStore(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := apiCacheMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("expected no-store, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestApiCacheMiddleware_LibrariesCacheable(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := apiCacheMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/libraries", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Cache-Control") != "private, max-age=300" {
		t.Errorf("expected cacheable header, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestExtractBearerToken_FromHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer mytoken")
	if got := extractBearerToken(req); got != "mytoken" {
		t.Errorf("expected 'mytoken', got %q", got)
	}
}

func TestExtractBearerToken_FromQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?token=querytoken", nil)
	if got := extractBearerToken(req); got != "querytoken" {
		t.Errorf("expected 'querytoken', got %q", got)
	}
}

func TestSafeJoin_Traversal(t *testing.T) {
	_, err := safeJoin("/tmp/safe", "../../../etc/passwd")
	if err == nil {
		t.Error("expected path traversal error")
	}
}

func TestSafeJoin_OK(t *testing.T) {
	got, err := safeJoin("/tmp/safe", "file.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/safe/file.jpg" {
		t.Errorf("expected /tmp/safe/file.jpg, got %s", got)
	}
}

// TokenFromQuery is how the on-demand manifest decides whether to bake the
// token back into sub-resource URIs: only a ?token= arrival (cookie-less
// Chromecast) says yes; cookie and Bearer clients keep bare URIs.
func TestTokenRedactMiddleware_RemembersQueryArrivalOnly(t *testing.T) {
	var fromQuery bool
	var raw string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fromQuery = TokenFromQuery(r.Context())
		raw = RawTokenFromContext(r.Context())
	})
	h := tokenRedactMiddleware(nil, false, next)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?token=opaque1", nil))
	if !fromQuery || raw != "opaque1" {
		t.Errorf("query arrival: fromQuery=%v raw=%q", fromQuery, raw)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8", nil)
	req.AddCookie(&http.Cookie{Name: hlsCookieName, Value: "opaque2"})
	h.ServeHTTP(httptest.NewRecorder(), req)
	if fromQuery {
		t.Error("a cookie client must not be told to bake the token into URIs")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8", nil)
	req.Header.Set("Authorization", "Bearer opaque3")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if fromQuery {
		t.Error("a Bearer client must not be told to bake the token into URIs")
	}
	if TokenFromQuery(httptest.NewRequest(http.MethodGet, "/", nil).Context()) {
		t.Error("an untouched context reads false")
	}
}
