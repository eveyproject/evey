package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func cspFor(t *testing.T, path string) string {
	t.Helper()
	h := securityHeadersMiddleware(false, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Header().Get("Content-Security-Policy")
}

// The Cast receiver page (/cast-receiver.html) runs on the TV: the CAF player
// loads not just scripts but also styles, fonts and images from gstatic - the
// app's CSP (style-src 'self') would silently block them on the Chromecast.
func TestSecurityHeaders_CastReceiverCSPAllowsCAF(t *testing.T) {
	t.Parallel()
	csp := cspFor(t, "/cast-receiver.html")
	if csp == "" {
		t.Fatal("receiver page must still carry a CSP")
	}
	for _, directive := range []string{"script-src", "style-src", "font-src", "img-src"} {
		seg := segmentOf(csp, directive)
		// The broad `https:` scheme covers gstatic just as well as the explicit host.
		if !strings.Contains(seg, "https://www.gstatic.com") && !strings.Contains(seg, "https:") {
			t.Fatalf("%s must allow gstatic for the CAF player, got %q", directive, seg)
		}
	}
	// The ac3go wasm + MSE are still needed on the TV.
	if !strings.Contains(csp, "wasm-unsafe-eval") {
		t.Fatalf("receiver CSP must keep wasm-unsafe-eval, got %q", csp)
	}
	if seg := segmentOf(csp, "media-src"); !strings.Contains(seg, "blob:") {
		t.Fatalf("media-src must allow blob:, got %q", seg)
	}
}

// The app's HARDENED policy must not widen: only the receiver gets the
// extended gstatic variant.
func TestSecurityHeaders_AppCSPUnchanged(t *testing.T) {
	t.Parallel()
	csp := cspFor(t, "/")
	if seg := segmentOf(csp, "style-src"); strings.Contains(seg, "gstatic") {
		t.Fatalf("app style-src must NOT allow gstatic, got %q", seg)
	}
	if seg := segmentOf(csp, "font-src"); strings.Contains(seg, "gstatic") {
		t.Fatalf("app font-src must NOT allow gstatic, got %q", seg)
	}
}

// segmentOf isolates one directive from a CSP policy ("style-src ...;").
func segmentOf(csp, directive string) string {
	for part := range strings.SplitSeq(csp, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, directive+" ") || part == directive {
			return part
		}
	}
	return ""
}
