package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appphoto "github.com/gravity-zero/evey/internal/app/photo"
	photofakes "github.com/gravity-zero/evey/internal/domain/photo/fakes"
	"github.com/gravity-zero/evey/internal/domain/shared"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func TestCheckLibraryAccess_AdminBypass(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()
	claims := &domainuser.Claims{UserID: "admin", IsAdmin: true}
	ok := checkLibraryAccess(ctx, w, claims, shared.LibraryID("lib-restricted"), denyAll())
	if !ok {
		t.Error("admin should bypass library access check")
	}
}

func TestCheckLibraryAccess_UserDenied(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()
	claims := &domainuser.Claims{UserID: "u1", IsAdmin: false}
	ok := checkLibraryAccess(ctx, w, claims, shared.LibraryID("lib-restricted"), denyAll())
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
	ok := checkLibraryAccess(ctx, w, claims, shared.LibraryID("lib-allowed"), allowAll())
	if !ok {
		t.Error("expected access allowed")
	}
}

func TestCheckLibraryAccess_NilChecker_DeniesNonAdmin(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()
	claims := &domainuser.Claims{UserID: "u1", IsAdmin: false}
	ok := checkLibraryAccess(ctx, w, claims, shared.LibraryID("lib-x"), nil)
	if ok {
		t.Error("nil checker should deny non-admin")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCheckLibraryAccess_NilChecker_AllowsAdmin(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()
	claims := &domainuser.Claims{UserID: "u1", IsAdmin: true}
	ok := checkLibraryAccess(ctx, w, claims, shared.LibraryID("lib-x"), nil)
	if !ok {
		t.Error("nil checker should allow admin")
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

func TestServePhoto_NoAuth_401(t *testing.T) {
	mux, _ := newPhotoTestMux(t, allowAll())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-1/serve", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
