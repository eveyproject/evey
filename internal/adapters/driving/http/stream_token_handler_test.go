package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appmedia "github.com/gravity-zero/evey/internal/app/media"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newStreamTokenTestMux(t *testing.T) (*http.ServeMux, string, *appmedia.StreamTokenStore) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "p1", IsAdmin: false})

	store := appmedia.NewStreamTokenStore(time.Minute)
	t.Cleanup(store.Stop)

	mg := &fakeMediaGetter{items: map[string]*domainlibrary.MediaItem{
		"m1": {ID: "m1", LibraryID: "lib1", FilePath: "/tmp/fake.mkv"},
	}}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/stream-token", streamTokenHandler(gen, store, allowAll(), mg))
	return mux, token, store
}

func TestStreamToken_NoAuth_401(t *testing.T) {
	mux, _, _ := newStreamTokenTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamToken_InvalidToken_401(t *testing.T) {
	mux, _, _ := newStreamTokenTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.Header.Set("Authorization", "Bearer invalid-garbage")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStreamToken_MediaNotFound_404(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "p1"})
	store := appmedia.NewStreamTokenStore(time.Minute)
	defer store.Stop()

	mg := &fakeMediaGetter{items: map[string]*domainlibrary.MediaItem{}} // empty

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/stream-token", streamTokenHandler(gen, store, allowAll(), mg))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestStreamToken_IDOR_403(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", ProfileID: "p1"})
	store := appmedia.NewStreamTokenStore(time.Minute)
	defer store.Stop()

	mg := &fakeMediaGetter{items: map[string]*domainlibrary.MediaItem{
		"m1": {ID: "m1", LibraryID: "lib-restricted", FilePath: "/tmp/fake.mkv"},
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/media/{id}/stream-token", streamTokenHandler(gen, store, denyAll(), mg))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestStreamToken_Success_200(t *testing.T) {
	mux, token, _ := newStreamTokenTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/stream-token", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["token"] == "" {
		t.Error("expected non-empty stream token")
	}
	if resp["mediaId"] != "m1" {
		t.Errorf("expected mediaId 'm1', got %q", resp["mediaId"])
	}
}
