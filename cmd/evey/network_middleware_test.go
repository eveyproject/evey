package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/pkg/network"
)

// The 2026-07-29 lockout: a WAN client was refused on EVERY route, the refusal
// was never logged, and the refusal of /api/v1/i18n left the clients rendering
// raw translation keys - so the operator saw nothing and the user saw gibberish.
// These tests pin all three fixes.

// fromBoundary is a request as the gate sees it in production: the proxy-trust
// middleware ran first and put the ONE validated client IP in the context. The
// gate reads only that - never a header - so a test that skips this step is
// testing an unvalidated request, which is WAN by contract.
func fromBoundary(path, ip string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.RemoteAddr = ip + ":41234"
	return r.WithContext(network.WithClientIP(r.Context(), net.ParseIP(ip)))
}

func newWANRequest(path string) *http.Request {
	return fromBoundary(path, "203.0.113.9")
}

func gateHandler(enabled bool, logger *slog.Logger) http.Handler {
	detector := network.NewDetector([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	mw := networkMiddleware(func() *network.Detector { return detector }, func() bool { return enabled }, logger)
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

	lan := fromBoundary("/api/v1/auth/login", "192.168.1.50")
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

// The gate reads RemoteAccess.Enabled on every request while an admin PUT can
// flip it at any moment. Until 2026-08-01 the middleware held a raw *bool into
// the config struct and read it with no synchronization, while the REST and
// gRPC handlers wrote it under two DIFFERENT private mutexes - so not even the
// two writers excluded each other.
//
// Run with -race, this fails on the old code and passes on the accessor.
func TestNetworkMiddleware_FlagFlipIsRaceFree(t *testing.T) {
	cfg := &config.Config{}
	cfg.RemoteAccess.LANNetworks = []string{"192.168.0.0/16"}
	cfg.RemoteAccess.Enabled = false

	detector := network.NewDetector(cfg.RemoteAccess.LANNetworks)
	h := networkMiddleware(func() *network.Detector { return detector }, cfg.EffectiveRemoteAccessEnabled,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	var wg sync.WaitGroup
	// Writers: both sides of the admin API mutate through the config's own lock.
	for w := range 2 {
		wg.Go(func() {
			for i := range 200 {
				cfg.ApplyServerSettings(func() {
					cfg.RemoteAccess.Enabled = (i+w)%2 == 0
					cfg.RemoteAccess.EnabledExplicit = true
				})
			}
		})
	}
	// Readers: the request-time gate.
	for range 4 {
		wg.Go(func() {
			for range 200 {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, newWANRequest("/api/v1/auth/login"))
				if rec.Code != http.StatusOK && rec.Code != http.StatusForbidden {
					t.Errorf("unexpected status %d", rec.Code)
					return
				}
			}
		})
	}
	wg.Wait()
}

// Admin -> Remote Access edits the LAN list as well as the switch. The gate used
// to build its detector once at boot, so a VPN range added in the panel kept
// being refused until a restart.
func TestNetworkMiddleware_LANListChangeAppliesWithoutRestart(t *testing.T) {
	cfg := &config.Config{}
	cfg.RemoteAccess.LANNetworks = []string{"192.168.0.0/16"}
	lan := network.NewLiveDetector(func() (cidrs []string) {
		cfg.WithReadLock(func() { cidrs = cfg.RemoteAccess.LANNetworks })
		return cidrs
	})
	h := networkMiddleware(lan.Detector, cfg.EffectiveRemoteAccessEnabled,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	vpnClient := func() int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, fromBoundary("/api/v1/auth/login", "100.64.3.4"))
		return rec.Code
	}
	if got := vpnClient(); got != http.StatusForbidden {
		t.Fatalf("before the change: status %d, want 403", got)
	}
	cfg.ApplyServerSettings(func() {
		cfg.RemoteAccess.LANNetworks = []string{"192.168.0.0/16", "100.64.0.0/10"}
	})
	if got := vpnClient(); got != http.StatusOK {
		t.Fatalf("after adding the VPN range: status %d, want 200", got)
	}
}

// The settings pair is read as ONE snapshot: a reader must never observe the
// forward value of one update next to the backward value of another.
func TestPlaybackSkipSeconds_SnapshotIsRaceFree(t *testing.T) {
	cfg := &config.Config{}
	cfg.Playback.SkipForwardSeconds = 30
	cfg.Playback.SkipBackwardSeconds = 10

	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 300 {
			cfg.ApplyServerSettings(func() {
				cfg.Playback.SkipForwardSeconds = 10 + i%50
				cfg.Playback.SkipBackwardSeconds = 5 + i%20
			})
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 300 {
				if fwd, bwd := cfg.EffectivePlaybackSkipSeconds(); fwd <= 0 || bwd <= 0 {
					t.Errorf("torn read: fwd=%d bwd=%d", fwd, bwd)
					return
				}
			}
		})
	}
	wg.Wait()
}
