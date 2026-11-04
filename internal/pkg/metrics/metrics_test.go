package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

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

func TestHandlerAllowsExternalWithScrapeToken(t *testing.T) {
	h := metrics.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "203.0.113.1:1234" // external IP
	w := httptest.NewRecorder()
	h.ServeHTTP(w, metrics.WithScrapeAuth(req))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
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
		// The files a playback session serves by the hundred are one label:
		// a segment index per series was a memory the process never gave back.
		{"/api/v1/media/med-abc/hls/stream_0_123.m4s", "/api/v1/media/{id}/hls/{file}"},
		{"/api/v1/media/med-abc/hls/stream_0_999.m4s", "/api/v1/media/{id}/hls/{file}"},
		{"/api/v1/media/med-abc/hls/audio_1_345.m4s", "/api/v1/media/{id}/hls/{file}"},
		{"/api/v1/media/med-abc/hls/sub0_12.vtt", "/api/v1/media/{id}/hls/{file}"},
		{"/api/v1/media/med-abc/trickplay/sheet_0007.jpg", "/api/v1/media/{id}/trickplay/{file}"},
		{"/api/v1/media/med-abc/hls/master.m3u8", "/api/v1/media/{id}/hls/master.m3u8"},
		{"/api/v1/media/med-abc/remux.mp4", "/api/v1/media/{id}/remux.mp4"},
	}
	for _, c := range cases {
		got := metrics.SanitizePath(c.in)
		if got != c.want {
			t.Errorf("SanitizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Every request no route answers shares one label, and a method the server
// never routes is one label too: an anonymous client probing paths or
// inventing verbs used to mint a series per spelling, for as long as the
// process lived.
func TestInstrumentHandler_LabelsStayBounded(t *testing.T) {
	notFound := metrics.InstrumentHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	before := testutil.CollectAndCount(metrics.HTTPRequestsTotal)
	for i := range 500 {
		req := httptest.NewRequest(http.MethodGet, "/probe-"+strconv.Itoa(i), nil)
		notFound.ServeHTTP(httptest.NewRecorder(), req)
	}
	after := testutil.CollectAndCount(metrics.HTTPRequestsTotal)
	if after-before > 1 {
		t.Fatalf("500 unmatched paths grew the series by %d, want at most 1", after-before)
	}

	ok := metrics.InstrumentHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	before = testutil.CollectAndCount(metrics.HTTPRequestsTotal)
	for _, verb := range []string{"BREW", "PURGE", "TRACK", "SEARCH"} {
		ok.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(verb, "/api/v1/test", nil))
	}
	after = testutil.CollectAndCount(metrics.HTTPRequestsTotal)
	if after-before > 1 {
		t.Fatalf("4 invented verbs grew the series by %d, want at most 1", after-before)
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

// Labelled by the route it reached: an anonymous probe through a wildcard
// route the guard refuses, a refusal under the catch-all "/" (the gateway),
// an unclean path the mux redirects, each is one series however it is spelt,
// and a route's own 404 keeps the route.
func TestInstrumentRoutes_LabelsByPattern(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/media/{id}/stream", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("GET /api/v1/items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	h := metrics.InstrumentRoutes(mux, mux)
	serve := func(path string) int {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}
	grown := func(name string, paths func(i int) string, wantStatus int) int {
		t.Helper()
		before := testutil.CollectAndCount(metrics.HTTPRequestsTotal)
		for i := range 300 {
			if got := serve(paths(i)); got != wantStatus {
				t.Fatalf("%s: status %d, want %d", name, got, wantStatus)
			}
		}
		return testutil.CollectAndCount(metrics.HTTPRequestsTotal) - before
	}
	cases := []struct {
		name   string
		path   func(i int) string
		status int
	}{
		{"wildcard route refusing", func(i int) string { return "/api/v1/media/probe" + strconv.Itoa(i) + "/stream" }, http.StatusUnauthorized},
		{"gateway refusing under /", func(i int) string { return "/api/v1/libraries/probe" + strconv.Itoa(i) }, http.StatusUnauthorized},
		{"unclean path redirected", func(i int) string { return "/probe" + strconv.Itoa(i) + "//" }, http.StatusTemporaryRedirect},
		{"route's own 404", func(i int) string { return "/api/v1/items/probe" + strconv.Itoa(i) }, http.StatusNotFound},
	}
	for _, c := range cases {
		if n := grown(c.name, c.path, c.status); n > 1 {
			t.Errorf("%s: 300 spellings grew the series by %d, want at most 1", c.name, n)
		}
	}

	serve("/api/v1/items/one")
	if got := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/api/v1/items/{id}", "404")); got < 1 {
		t.Errorf("a route's own 404 is not labelled by the route (count %v)", got)
	}
}
