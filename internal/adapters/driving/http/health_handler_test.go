package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
