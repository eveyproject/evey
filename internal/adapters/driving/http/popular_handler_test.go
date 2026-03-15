package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	applibrary "github.com/gravity-zero/evey/internal/app/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

func newPopularTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	tokenGen := newTestJWTGen(t)

	activitySvc := newTestActivityQuerySvc()
	libQuery := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
	)

	tok, err := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	mux := http.NewServeMux()
	registerPopularRoute(mux, activitySvc, libQuery, tokenGen)
	return mux, tok
}

func TestPopularRequiresAuth(t *testing.T) {
	// Without an Authorization header, route should return 401.
	// We can verify this via the requireAdmin helper (which enforces auth+admin).
	// For non-admin auth we simulate via the handler's inline check.
	tv := invalidValidator()
	handler := requireAdmin(tv, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/v1/popular", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPopular_NoAuth_401(t *testing.T) {
	mux, _ := newPopularTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPopular_ValidAuth_200(t *testing.T) {
	mux, tok := newPopularTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/popular", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
