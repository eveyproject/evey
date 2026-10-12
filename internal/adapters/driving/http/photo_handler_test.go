package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	appphoto "github.com/eveyproject/evey/internal/app/photo"
	"github.com/eveyproject/evey/internal/domain/photo"
	photofakes "github.com/eveyproject/evey/internal/domain/photo/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
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
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
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
	registerPhotoRoutes(mux, svc, tokenGen, checker, nil, context.Background(), nil, "", nil)
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

// A JPEG is read whole and EXIF-stripped on every request: a revalidation of an
// unchanged file must answer 304 from its stat, without that work. Proven by
// making the file unreadable between the two requests.
func TestServePhoto_JPEGRevalidatesFromStat(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	path := filepath.Join(t.TempDir(), "shot.jpg")
	// Smallest JPEG that the EXIF stripper walks: SOI + EOI.
	if err := os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0o644); err != nil {
		t.Fatal(err)
	}
	repo := photofakes.NewFakeRepository()
	if err := repo.Upsert(context.Background(), photo.Photo{ID: "ph-1", LibraryID: "lib1", FilePath: path, MimeType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, nil)
	mux := http.NewServeMux()
	registerPhotoRoutes(mux, svc, tokenGen, allowAll(), nil, context.Background(), nil, "", nil)

	get := func(inm string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/photos/ph-1/serve", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	first := get("")
	if first.Code != http.StatusOK {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("a JPEG response must carry an ETag so the browser can revalidate")
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("running as root: an unreadable file cannot prove the shortcut")
	}
	if got := get(etag).Code; got != http.StatusNotModified {
		t.Fatalf("revalidation of an unchanged file: got %d, want 304", got)
	}
}
