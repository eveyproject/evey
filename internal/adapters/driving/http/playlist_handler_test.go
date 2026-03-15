package httpapi

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appplaylist "github.com/gravity-zero/evey/internal/app/playlist"
	playlistfakes "github.com/gravity-zero/evey/internal/domain/playlist/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newPlaylistTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	plRepo := playlistfakes.NewFakePlaylistRepository()
	itemRepo := playlistfakes.NewFakePlaylistItemRepository()
	querier := playlistfakes.NewFakeSmartPlaylistQuerier()
	svc := appplaylist.NewService(plRepo, itemRepo, querier)

	mux := http.NewServeMux()
	registerPlaylistRoutes(mux, svc, gen)
	return mux, token
}

func TestPlaylistRequiresAuth(t *testing.T) {
	tv := invalidValidator()
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/v1/playlists", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListPlaylists_NoAuth_401(t *testing.T) {
	mux, _ := newPlaylistTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playlists", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListPlaylists_Auth_200(t *testing.T) {
	mux, token := newPlaylistTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/playlists", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCreatePlaylist_NoAuth_401(t *testing.T) {
	mux, _ := newPlaylistTestMux(t)
	body, _ := json.Marshal(map[string]string{"name": "My List", "type": "manual"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreatePlaylist_Auth_201(t *testing.T) {
	mux, token := newPlaylistTestMux(t)
	body, _ := json.Marshal(map[string]string{"name": "Favorites", "type": "manual"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playlists", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestDeletePlaylist_NoAuth_401(t *testing.T) {
	mux, _ := newPlaylistTestMux(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/playlists/pl-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
