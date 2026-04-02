package httpapi

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	applibrary "github.com/gravity-zero/evey/internal/app/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/ratelimit"
)

func newAccessTestMux(t *testing.T, isAdmin bool) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	gen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := gen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: isAdmin})

	accessRepo := userfakes.NewFakeLibraryAccessRepository()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaItemRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQuery := applibrary.NewQueryService(libQueryRepo, mediaItemRepo, streamRepo, searchIdx)

	mux := http.NewServeMux()
	registerLibraryAccessRoutes(mux, accessRepo, libQuery, gen, ratelimit.New(100))
	return mux, token
}

func TestAccessHandlerRequiresAdmin(t *testing.T) {
	tv := validUserValidator(false)
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/v1/admin/users/u1/library-access", nil)
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", w.Code)
	}
}

func TestGetLibraryAccess_NoAuth_403(t *testing.T) {
	mux, _ := newAccessTestMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/library-access", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestGetLibraryAccess_NonAdmin_403(t *testing.T) {
	mux, token := newAccessTestMux(t, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/library-access", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestGetLibraryAccess_Admin_200(t *testing.T) {
	mux, token := newAccessTestMux(t, true)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/u1/library-access", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
