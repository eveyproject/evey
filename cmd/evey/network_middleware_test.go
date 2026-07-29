package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/pkg/network"
)

// The 2026-07-29 lockout: a WAN client was refused on EVERY route, the refusal
// was never logged, and the refusal of /api/v1/i18n left the clients rendering
// raw translation keys - so the operator saw nothing and the user saw gibberish.
// These tests pin all three fixes.

func newWANRequest(path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	// The proxy-trust middleware has already collapsed X-Forwarded-For by the
	// time netMw runs, so RemoteAddr IS the real client.
	r.RemoteAddr = "203.0.113.9:41234"
	return r
}

func gateHandler(enabled bool, logger *slog.Logger) http.Handler {
	detector := network.NewDetector([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	mw := networkMiddleware(detector, &enabled, logger)
	return mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	}))
}

func TestNetworkMiddleware_BootstrapRoutesStayReachableWhenRemoteDisabled(t *testing.T) {
	h := gateHandler(false, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	// A refused client must still be able to render a READABLE screen that
	// explains itself: the translation catalogue and the server identity are
	// what the login screen needs before anything else.
	for _, path := range []string{
		"/healthz",
		"/api/v1/i18n/en",
		"/api/v1/i18n/fr",
		"/api/v1/setup/status",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newWANRequest(path))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (bootstrap route must never be gated)", path, rec.Code)
		}
	}
}

func TestNetworkMiddleware_RefusesOtherRoutesWithAMachineReadableCode(t *testing.T) {
	h := gateHandler(false, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newWANRequest("/api/v1/auth/login"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON (%v): %q", err, rec.Body.String())
	}
	// The client cannot act on prose. It needs a stable token to map onto
	// "remote access is disabled on this server" instead of a generic failure.
	if body.Code != "remote_access_disabled" {
		t.Errorf("code = %q, want %q", body.Code, "remote_access_disabled")
	}
	if body.Message == "" {
		t.Error("message is empty: humans read the logs too")
	}
	if got := rec.Header().Get("X-Evey-Remote-Access"); got != "disabled" {
		t.Errorf("X-Evey-Remote-Access = %q, want \"disabled\"", got)
	}
}

func TestNetworkMiddleware_LogsTheRefusal(t *testing.T) {
	var buf bytes.Buffer
	h := gateHandler(false, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	h.ServeHTTP(httptest.NewRecorder(), newWANRequest("/api/v1/auth/login"))

	out := buf.String()
	if !strings.Contains(out, "remote access") {
		t.Fatalf("refusal not logged: %q", out)
	}
	// Without the IP and the path an operator cannot tell a locked-out family
	// member from a scanner - which is precisely what cost two days.
	for _, want := range []string{"203.0.113.9", "/api/v1/auth/login"} {
		if !strings.Contains(out, want) {
			t.Errorf("log line missing %q: %q", want, out)
		}
	}
}

func TestNetworkMiddleware_RefusalLogIsThrottledPerClient(t *testing.T) {
	var buf bytes.Buffer
	h := gateHandler(false, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	// A retrying app must not flood the log: one line per client per window.
	for range 25 {
		h.ServeHTTP(httptest.NewRecorder(), newWANRequest("/api/v1/auth/login"))
	}
	if n := strings.Count(buf.String(), "remote access"); n != 1 {
		t.Errorf("logged %d times for one client, want 1 (throttled)", n)
	}
}

func TestNetworkMiddleware_LANAndEnabledRemoteBothPassThrough(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	lan := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	lan.RemoteAddr = "192.168.1.50:5000"
	rec := httptest.NewRecorder()
	gateHandler(false, logger).ServeHTTP(rec, lan)
	if rec.Code != http.StatusOK {
		t.Errorf("LAN client: status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	gateHandler(true, logger).ServeHTTP(rec, newWANRequest("/api/v1/auth/login"))
	if rec.Code != http.StatusOK {
		t.Errorf("WAN client with remote access ON: status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("X-Evey-Network"); got != "wan" {
		t.Errorf("X-Evey-Network = %q, want \"wan\"", got)
	}
}
