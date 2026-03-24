package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	photofakes "github.com/gravity-zero/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func TestCheckLibraryAccess_AdminBypass(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()
	claims := &domainuser.Claims{UserID: "admin", IsAdmin: true}
	ok := checkLibraryAccess(ctx, w, claims, "lib-restricted", denyAll())
	if !ok {
		t.Error("admin should bypass library access check")
	}
}

func TestCheckLibraryAccess_UserDenied(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()
	claims := &domainuser.Claims{UserID: "u1", IsAdmin: false}
	ok := checkLibraryAccess(ctx, w, claims, "lib-restricted", denyAll())
	if ok {
		t.Error("expected access denied")
	}
	if w.Code != 403 {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCheckLibraryAccess_UserAllowed(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()
	claims := &domainuser.Claims{UserID: "u1", IsAdmin: false}
	ok := checkLibraryAccess(ctx, w, claims, "lib-allowed", allowAll())
	if !ok {
		t.Error("expected access allowed")
	}
}

func TestCheckLibraryAccess_NilChecker(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()
	claims := &domainuser.Claims{UserID: "u1", IsAdmin: false}
	ok := checkLibraryAccess(ctx, w, claims, "lib-x", nil)
	if !ok {
		t.Error("nil checker should allow all")
	}
}

func newPhotoTestMux(t *testing.T, checker LibraryAccessChecker) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})

	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, nil)

	mux := http.NewServeMux()
	registerPhotoRoutes(mux, svc, tokenGen, checker, nil, context.Background())
	return mux, token
}

func TestListPhotos_NoAuth_401(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos?library_id=lib-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestListPhotos_IDOR_DeniedLibrary(t *testing.T) {
	mux, token := newPhotoTestMux(t, denyAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos?library_id=lib-other", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for denied library, got %d", w.Code)
	}
}

func TestDeletePhoto_NoAuth_401(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/photos/photo-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func newPhotoAdminTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	adminToken, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "admin-1", IsAdmin: true})

	repo := photofakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := appphoto.NewService(repo, bus, nil, nil)

	mux := http.NewServeMux()
	registerPhotoRoutes(mux, svc, tokenGen, allowAll(), nil, context.Background())
	return mux, adminToken
}

func TestListPhotos_Auth_200(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos?library_id=lib-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetPhoto_NoAuth_401(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPhotoAlbums_NoAuth_401(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/albums?library_id=lib-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPhotoAlbums_Auth_200(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/albums?library_id=lib-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestServePhoto_NoAuth_401(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-1/serve", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPhotoSearch_NoAuth_401(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/search?library_id=lib-1&q=test", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPhotoSearch_Auth_200(t *testing.T) {
	mux, token := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/search?library_id=lib-1&q=test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPhotoScan_RelativePath_400(t *testing.T) {
	mux, token := newPhotoAdminTestMux(t)
	body := `{"library_id":"lib-1","dir":"../../etc"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/photos/scan", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for relative path, got %d", w.Code)
	}
}
