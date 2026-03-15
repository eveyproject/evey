package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeDBPinger struct{ err error }

func (f *fakeDBPinger) Ping(_ context.Context) error { return f.err }

func TestHealthzHandler_OK(t *testing.T) {
	srv := &HTTPServer{
		Version:   "1.2.3",
		StartTime: time.Now().Add(-30 * time.Second),
		DB:        &fakeDBPinger{},
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
	if resp.Version != "1.2.3" {
		t.Errorf("version: want 1.2.3, got %q", resp.Version)
	}
	if resp.UptimeSeconds < 1 {
		t.Errorf("uptime_seconds: want >= 1, got %d", resp.UptimeSeconds)
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
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
	if resp.Version != "dev" {
		t.Errorf("version fallback: want dev, got %q", resp.Version)
	}
}
