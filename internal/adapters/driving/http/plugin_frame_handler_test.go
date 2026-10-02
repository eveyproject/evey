package httpapi

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The plugin frame carries its own policy, through the app's security
// middleware: sandboxed even top-level, only its own loader inline, network
// limited to the manifest's hosts, framed by the app only.
func TestPluginFrame_OwnPolicy(t *testing.T) {
	h := securityHeadersMiddleware(true, http.HandlerFunc(pluginFrameHandler))
	req := httptest.NewRequest(http.MethodGet, pluginFramePath+"?connect=api.example.org,https://img.example.net:8443,http://localhost:8080,evil.com/path,%2A.example.org,1.2.3.4,ftp://x.org,a.org%27%3B%20script-src%20*", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{
		"sandbox allow-scripts;",
		"default-src 'none'",
		"connect-src https://api.example.org https://img.example.net:8443 http://localhost:8080;",
		"frame-ancestors 'self'",
		"form-action 'none'",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("policy %q lacks %q", csp, want)
		}
	}
	if strings.Contains(csp, "unsafe-inline' blob") || strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "'unsafe-eval'") {
		t.Errorf("script-src too wide: %q", csp)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options %q, want SAMEORIGIN (the app frames it)", got)
	}

	// The hash in the policy is the hash of the loader the page actually holds.
	body := w.Body.String()
	start := strings.Index(body, "<script>") + len("<script>")
	end := strings.Index(body, "</script>")
	sum := sha256.Sum256([]byte(body[start:end]))
	if !strings.Contains(csp, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
		t.Errorf("policy does not allow the page's own inline loader")
	}
	if strings.Count(body, "<script") != 1 {
		t.Errorf("the loader must be the page's only script")
	}
}

func TestPluginFrame_NoHostsMeansNoNetwork(t *testing.T) {
	for _, q := range []string{"", "?connect=", "?connect=javascript:alert(1)", "?connect=10.0.0.1", "?connect=%5B::1%5D"} {
		w := httptest.NewRecorder()
		pluginFrameHandler(w, httptest.NewRequest(http.MethodGet, pluginFramePath+q, nil))
		if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "connect-src 'none';") {
			t.Errorf("%q: %q, want connect-src 'none'", q, csp)
		}
	}
}
