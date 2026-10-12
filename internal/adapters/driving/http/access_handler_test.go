package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
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

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
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

// TestRemoteAccessPut_BodyTooLarge_400 proves that MaxBytesReader caps the
// request body: a body larger than 1 MiB must be rejected with 400.
func TestRemoteAccessPut_BodyTooLarge_400(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	// Build a JSON object whose total size exceeds 1 MiB.
	// The value is a string of 1 MiB+1 bytes embedded in a valid JSON object.
	giant := `{"enabled":true,"externalUrl":"` + strings.Repeat("a", 1<<20+1) + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader([]byte(giant)))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for oversized body, got %d: %s", w.Code, w.Body.String())
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

// The hardened validator (https-only, no loopback / RFC1918 / link-local) was
// only ever called by the gRPC handler, while THIS hand-written route is the
// one real traffic reaches: Go 1.22+ ServeMux precedence puts it ahead of the
// grpc-gateway route bound to the same method and path. So the inline check
// here - "any http or https URL with a host" - was the real policy, and it
// accepted every address the validator exists to reject.
func TestRemoteAccessPut_ExternalURL_RejectsUnsafeTargets(t *testing.T) {
	for _, target := range []string{
		"http://example.com",      // plain http
		"https://localhost",       // loopback by name
		"https://127.0.0.1",       // loopback by address
		"https://192.168.1.10",    // RFC1918
		"https://169.254.169.254", // link-local / cloud metadata
		"https://metadata.google.internal",
	} {
		t.Run(target, func(t *testing.T) {
			mux, adminTok, _ := setupRemoteAccessMux(t)
			b, _ := json.Marshal(map[string]any{"externalUrl": target})
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
			req.Header.Set("Authorization", "Bearer "+adminTok)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d: %s", target, w.Code, w.Body.String())
			}
		})
	}
}

func TestRemoteAccessPut_ExternalURL_AcceptsAPublicHTTPSURL(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	b, _ := json.Marshal(map[string]any{"externalUrl": "https://evey.example.org"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// Clearing the URL must stay possible - it is how an operator turns a published
// instance back into a LAN-only one, and an empty string is not a target.
func TestRemoteAccessPut_ExternalURL_EmptyStillClears(t *testing.T) {
	mux, adminTok, _ := setupRemoteAccessMux(t)
	b, _ := json.Marshal(map[string]any{"externalUrl": ""})
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
	if body["externalUrl"] != "" {
		t.Errorf("expected the external URL to be cleared, got %v", body["externalUrl"])
	}
}

// Same contract as the playback-settings and generic settings PUTs: a value
// the store refused is applied in memory but reported as not saved, never 200.
func TestRemoteAccessPut_PersistFailureIs500(t *testing.T) {
	gen, adminTok, _ := genTokens(t)
	cfg := &config.Config{RemoteAccess: config.RemoteAccessConfig{Enabled: false}}
	store := &fakeRemoteAccessStore{data: map[string]string{}, setErr: errors.New("db down")}
	mux := http.NewServeMux()
	registerRemoteAccessRoutes(mux, cfg, gen, store, ratelimit.New(600))

	b, _ := json.Marshal(map[string]any{"enabled": true})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if !cfg.RemoteAccess.Enabled {
		t.Error("live config should keep the applied value")
	}
}
