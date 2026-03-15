package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	favfakes "github.com/gravity-zero/evey/internal/domain/favorite/fakes"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newFavoriteTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	favRepo := favfakes.NewFakeRepository()
	favSvc := appfavorite.NewService(favRepo)

	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaItemRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQuery := applibrary.NewQueryService(libQueryRepo, mediaItemRepo, streamRepo, searchIdx)

	mux := http.NewServeMux()
	registerFavoriteRoutes(mux, favSvc, libQuery, gen)
	return mux, token
}

func TestFavoriteRequiresAuth(t *testing.T) {
	tv := invalidValidator()
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/v1/favorites", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListFavorites_NoAuth_401(t *testing.T) {
	mux, _ := newFavoriteTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListFavorites_Auth_200(t *testing.T) {
	mux, token := newFavoriteTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAddFavorite_NoAuth_401(t *testing.T) {
	mux, _ := newFavoriteTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/favorites/media-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAddFavorite2_NoAuth_401(t *testing.T) {
	mux, _ := newFavoriteTestMux(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/favorites/media-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
