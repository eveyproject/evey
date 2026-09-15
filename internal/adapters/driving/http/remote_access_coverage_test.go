package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
)

// --- shared helpers ---

type fakeRemoteAccessStore struct {
	data   map[string]string
	setErr error
}

func (f *fakeRemoteAccessStore) Get(_ context.Context, key string) (string, error) {
	v, ok := f.data[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (f *fakeRemoteAccessStore) Set(_ context.Context, key, value string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.data[key] = value
	return nil
}

func (f *fakeRemoteAccessStore) Delete(_ context.Context, key string) error {
	delete(f.data, key)
	return nil
}

// --- remote access routes ---

func newRemoteAccessMux(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	gen, adminTok, userTok := genTokens(t)
	cfg := &config.Config{}
	store := &fakeRemoteAccessStore{data: make(map[string]string)}
	mux := http.NewServeMux()
	registerRemoteAccessRoutes(mux, cfg, gen, store, ratelimit.New(100))
	return mux, adminTok, userTok
}

func TestRemoteAccess_Get_NoAuth_401(t *testing.T) {
	mux, _, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/remote-access", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRemoteAccess_Get_Admin_200(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/remote-access", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRemoteAccess_Put_Admin_OK(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	body := `{"enabled":true,"externalUrl":"https://example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRemoteAccess_Put_NonAdmin_403(t *testing.T) {
	mux, _, userTok := newRemoteAccessMux(t)
	body := `{"enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRemoteAccess_Put_BadBody_400(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", strings.NewReader("bad"))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRemoteAccess_Put_InvalidExternalURL_400(t *testing.T) {
	mux, adminTok, _ := newRemoteAccessMux(t)
	body := `{"externalUrl":"not-a-url"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/remote-access", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
