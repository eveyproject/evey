package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/pkg/metrics"
)

func TestHandlerForbidsNonLoopback(t *testing.T) {
	h := metrics.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "203.0.113.1:1234" // external IP
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestHandlerAllowsLoopback(t *testing.T) {
	h := metrics.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:9090"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "evey_active_hls_sessions") {
		t.Fatal("response missing evey_active_hls_sessions metric")
	}
}

func TestInstrumentHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := metrics.InstrumentHandler(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestSanitizePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/", "/"},
		{"", ""},
		{"/api/v1/media/med-abc123/stream", "/api/v1/media/{id}/stream"},
		{"/api/v1/users/12345", "/api/v1/users/{id}"},
		{"/api/v1/users/usr-deadbeef/profile", "/api/v1/users/{id}/profile"},
		{"/api/v1/items/550e8400-e29b-41d4-a716-446655440000", "/api/v1/items/{id}"},
		{"/api/v1/static/about", "/api/v1/static/about"},
		{"/api/v1/stream/" + strings.Repeat("a", 40), "/api/v1/stream/{token}"},
		{"/api/v1/files/abcdef0123456789", "/api/v1/files/{id}"},
		{"/healthz", "/healthz"},
	}
	for _, c := range cases {
		got := metrics.SanitizePath(c.in)
		if got != c.want {
			t.Errorf("SanitizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHLSSessionsGauge(t *testing.T) {
	metrics.HLSSessionsGauge.Set(5)
	// verify it can be set without panic
	metrics.HLSSessionsGauge.Set(0)
}

func TestPluginMetrics(t *testing.T) {
	metrics.SetPluginUp("plugin-test", true)
	metrics.IncPluginError("plugin-test")
	metrics.SetPluginUp("plugin-test", false)

	h := metrics.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:9090"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "evey_plugin_up") {
		t.Fatal("response missing evey_plugin_up metric")
	}
	if !strings.Contains(body, "evey_plugin_errors_total") {
		t.Fatal("response missing evey_plugin_errors_total metric")
	}
}
