package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/config"
)

type fakeDBPinger struct{ err error }

func (f *fakeDBPinger) Ping(_ context.Context) error { return f.err }

type fakeSearchHealth struct{ err error }

func (f *fakeSearchHealth) HealthCheck(_ context.Context) error { return f.err }

func TestHealthzHandler_OK(t *testing.T) {
	srv := &HTTPServer{
		Version:      "1.2.3",
		StartTime:    time.Now().Add(-30 * time.Second),
		DB:           &fakeDBPinger{},
		SearchHealth: &fakeSearchHealth{},
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.healthzHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status: want ok, got %q", resp.Status)
	}
	if resp.DB != "ok" {
		t.Errorf("db: want ok, got %q", resp.DB)
	}
	if resp.Meilisearch != "ok" {
		t.Errorf("meilisearch: want ok, got %q", resp.Meilisearch)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("version: want 1.2.3, got %q", resp.Version)
	}
	if resp.UptimeSeconds < 1 {
		t.Errorf("uptime_seconds: want >= 1, got %d", resp.UptimeSeconds)
	}
}

// /healthz is the mobile/TV discovery probe: the admin's server name rides on
// it so the server list can show "Salon" instead of a bare URL. A blank name
// reports the default, never "".
func TestHealthzHandler_AdvertisesServerName(t *testing.T) {
	for name, want := range map[string]string{"Salon": "Salon", "": config.DefaultServerName} {
		cfg := &config.Config{}
		cfg.Server.Name = name
		srv := &HTTPServer{Cfg: cfg, StartTime: time.Now(), DB: &fakeDBPinger{}}
		w := httptest.NewRecorder()
		srv.healthzHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		var resp healthResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Name != want {
			t.Errorf("cfg name %q: healthz name = %q, want %q", name, resp.Name, want)
		}
	}
}

// The TV app compares its own versionCode to the one released with the
// server: the Play Store auto-update rarely runs on a television (27-09, a TV
// stuck a week on 0.1.4). Unstamped, the field is absent rather than zero.
func TestHealthzHandler_AdvertisesTVRelease(t *testing.T) {
	decode := func(srv *HTTPServer) map[string]any {
		w := httptest.NewRecorder()
		srv.healthzHandler(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		var body map[string]any
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body
	}

	body := decode(&HTTPServer{StartTime: time.Now(), DB: &fakeDBPinger{}, TVRelease: ClientRelease{Version: "0.1.11", VersionCode: 56}})
	clients, _ := body["clients"].(map[string]any)
	tv, _ := clients["tv"].(map[string]any)
	if tv["version"] != "0.1.11" || tv["version_code"] != float64(56) {
		t.Fatalf("clients.tv = %v, want version 0.1.11 and version_code 56", body["clients"])
	}

	if body := decode(&HTTPServer{StartTime: time.Now(), DB: &fakeDBPinger{}}); body["clients"] != nil {
		t.Fatalf("unstamped build: clients = %v, want absent", body["clients"])
	}
}

// The build stamps are what makes a deployment verifiable over HTTP alone
// (scripts/verify-deploy.sh). A stamped server must report the SHA verbatim -
// not a truncation, not the version string.
func TestHealthzHandler_ReportsBuildStamps(t *testing.T) {
	srv := &HTTPServer{
		Version:      "beta-0.0.1-full",
		Commit:       "0123456789abcdef0123456789abcdef01234567",
		BuildDate:    "2026-08-22T09:00:00Z",
		StartTime:    time.Now(),
		DB:           &fakeDBPinger{},
		SearchHealth: &fakeSearchHealth{},
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.healthzHandler(w, req)

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Commit != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("commit: got %q", resp.Commit)
	}
	if resp.BuildDate != "2026-08-22T09:00:00Z" {
		t.Errorf("build_date: got %q", resp.BuildDate)
	}
}

// An unstamped build must SAY "unknown" rather than send "". An empty string is
// indistinguishable from an older server that lacks the field, and a caller
// that cannot tell those apart ends up assuming a match - the exact failure
// mode this whole mechanism exists to prevent.
func TestHealthzHandler_UnstampedBuildSaysUnknown(t *testing.T) {
	srv := &HTTPServer{
		Version:      "dev",
		StartTime:    time.Now(),
		DB:           &fakeDBPinger{},
		SearchHealth: &fakeSearchHealth{},
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.healthzHandler(w, req)

	raw := w.Body.String()
	if !strings.Contains(raw, `"commit":"unknown"`) {
		t.Errorf("commit must be serialised as \"unknown\", got body %s", raw)
	}
	if !strings.Contains(raw, `"build_date":"unknown"`) {
		t.Errorf("build_date must be serialised as \"unknown\", got body %s", raw)
	}
}

func TestHealthzHandler_DBError(t *testing.T) {
	srv := &HTTPServer{
		Version:   "dev",
		StartTime: time.Now(),
		DB:        &fakeDBPinger{err: context.DeadlineExceeded},
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.healthzHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("status: want error, got %q", resp.Status)
	}
	if resp.DB != "error" {
		t.Errorf("db: want error, got %q", resp.DB)
	}
}

func TestHealthzHandler_NilDB(t *testing.T) {
	srv := &HTTPServer{StartTime: time.Now()}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.healthzHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.DB != "ok" {
		t.Errorf("db with nil pinger: want ok, got %q", resp.DB)
	}
	if resp.Meilisearch != "not_configured" {
		t.Errorf("meilisearch: want not_configured, got %q", resp.Meilisearch)
	}
	if resp.Version != "dev" {
		t.Errorf("version fallback: want dev, got %q", resp.Version)
	}
}

func TestHealthzHandler_MeilisearchDegraded(t *testing.T) {
	srv := &HTTPServer{
		Version:      "dev",
		StartTime:    time.Now(),
		DB:           &fakeDBPinger{},
		SearchHealth: &fakeSearchHealth{err: errors.New("connection refused")},
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.healthzHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (degraded still serves), got %d", w.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("status: want degraded, got %q", resp.Status)
	}
	if resp.Meilisearch != "degraded" {
		t.Errorf("meilisearch: want degraded, got %q", resp.Meilisearch)
	}
	if resp.DB != "ok" {
		t.Errorf("db: want ok, got %q", resp.DB)
	}
}

func TestHealthzHandler_DBError_Overrides_MeilisearchDegraded(t *testing.T) {
	srv := &HTTPServer{
		Version:      "dev",
		StartTime:    time.Now(),
		DB:           &fakeDBPinger{err: errors.New("db down")},
		SearchHealth: &fakeSearchHealth{err: errors.New("meili down")},
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.healthzHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "error" {
		t.Errorf("status: want error, got %q", resp.Status)
	}
}
