package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func setupRemoteAccessMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cfg := &config.Config{
		RemoteAccess: config.RemoteAccessConfig{
			Enabled:              false,
			ExternalURL:          "https://example.com",
			LANNetworks:          []string{"192.168.1.0/24"},
			MaxRemoteBitrateKbps: 8000,
		},
		Server: config.ServerConfig{BaseURL: "http://localhost:8080"},
	}
	store := &fakeRemoteAccessStore{data: map[string]string{}}
	rl := ratelimit.New(600)
	mux := http.NewServeMux()
	registerRemoteAccessRoutes(mux, cfg, gen, store, rl)
	return mux, adminTok, userTok
}

func TestRemoteAccessGet_Admin(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/remote-access", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != false {
		t.Error("expected enabled=false")
	}
	if body["externalUrl"] != "https://example.com" {
		t.Errorf("unexpected externalUrl: %v", body["externalUrl"])
	}
}

func TestRemoteAccessGet_NonAdmin_403(t *testing.T) {
	mux, _, userTok := setupRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/remote-access", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestRemoteAccessGet_NoAuth_403(t *testing.T) {
	mux, _, _ := setupRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/remote-access", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestRemoteAccessPut_Admin_UpdatesConfig(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	payload := map[string]any{
		"enabled":              true,
		"externalUrl":          "https://new.example.com",
		"lanNetworks":          []string{"10.0.0.0/8"},
		"maxRemoteBitrateKbps": 4000,
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != true {
		t.Error("expected enabled=true after update")
	}
	if body["externalUrl"] != "https://new.example.com" {
		t.Errorf("unexpected externalUrl: %v", body["externalUrl"])
	}
}

func TestRemoteAccessPut_NonAdmin_403(t *testing.T) {
	mux, _, userTok := setupRemoteAccessMux(t)
	b, _ := json.Marshal(map[string]any{"enabled": true})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestRemoteAccessPut_BadJSON_400(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader([]byte("not json")))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRemoteAccessPut_InvalidURL_400(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	b, _ := json.Marshal(map[string]any{"externalUrl": "ftp://bad"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid URL scheme, got %d", w.Code)
	}
}

func TestRemoteAccessPut_ClearURL(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	b, _ := json.Marshal(map[string]any{"externalUrl": ""})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["externalUrl"] != "" {
		t.Errorf("expected empty externalUrl, got %v", body["externalUrl"])
	}
}

func TestRemoteAccessPut_PartialUpdate(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	b, _ := json.Marshal(map[string]any{"maxRemoteBitrateKbps": 2000})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != false {
		t.Error("enabled should remain false on partial update")
	}
	if body["maxRemoteBitrateKbps"] != float64(2000) {
		t.Errorf("expected maxRemoteBitrateKbps=2000, got %v", body["maxRemoteBitrateKbps"])
	}
}

func TestRemoteAccessPut_PersistsToStore(t *testing.T) {
	gen, adminTok, _ := genTokens(t)
	cfg := &config.Config{
		RemoteAccess: config.RemoteAccessConfig{Enabled: false},
	}
	store := &fakeRemoteAccessStore{data: map[string]string{}}
	rl := ratelimit.New(600)
	mux := http.NewServeMux()
	registerRemoteAccessRoutes(mux, cfg, gen, store, rl)

	b, _ := json.Marshal(map[string]any{"enabled": true})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if _, ok := store.data[remoteAccessConfigKey]; !ok {
		t.Error("expected config to be persisted to store")
	}
}
