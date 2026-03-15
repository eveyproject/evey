package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newCollectionTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	cmdRepo := libfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	queryRepo := libfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)

	mux := http.NewServeMux()
	registerCollectionRoutes(mux, svc, gen)
	return mux, token
}

func TestCollectionRoutes_Unauthenticated(t *testing.T) {
	tv := invalidValidator()
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/v1/collections", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
}

func TestListCollections_NoAuth_401(t *testing.T) {
	mux, _ := newCollectionTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListCollections_Auth_200(t *testing.T) {
	mux, token := newCollectionTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetCollectionItems_NoAuth_401(t *testing.T) {
	mux, _ := newCollectionTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/col-1/items", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
