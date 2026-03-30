package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/favorite"
	favfakes "github.com/gravity-zero/evey/internal/domain/favorite/fakes"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
)

type favoriteTestFixture struct {
	mux        *http.ServeMux
	token      string
	favRepo    *favfakes.FakeRepository
	mediaRepo  *libfakes.FakeMediaItemQueryRepo
	accessRepo *userfakes.FakeLibraryAccessRepository
}

func newFavoriteTestFixture(t *testing.T, claims domainuser.Claims) *favoriteTestFixture {
	t.Helper()
	gen := newTestJWTGen(t)
	token, _ := gen.GenerateAccessToken(claims)

	favRepo := favfakes.NewFakeRepository()
	favSvc := appfavorite.NewService(favRepo)

	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaItemRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	searchIdx := libfakes.NewFakeSearchIndex()
	libQuery := applibrary.NewQueryService(libQueryRepo, mediaItemRepo, streamRepo, searchIdx)

	accessRepo := userfakes.NewFakeLibraryAccessRepository()

	mux := http.NewServeMux()
	registerFavoriteRoutes(mux, favSvc, libQuery, gen, accessRepo)
	return &favoriteTestFixture{
		mux: mux, token: token, favRepo: favRepo,
		mediaRepo: mediaItemRepo, accessRepo: accessRepo,
	}
}

func newFavoriteTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	f := newFavoriteTestFixture(t, domainuser.Claims{UserID: "u1", IsAdmin: false})
	// seed a default media item so existing toggle/check tests pass
	f.mediaRepo.Seed(&library.MediaItem{ID: "media-1", LibraryID: "lib1", Title: "Test"})
	return f.mux, f.token
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

func TestToggleFavorite_IDOR_Denied(t *testing.T) {
	f := newFavoriteTestFixture(t, domainuser.Claims{UserID: "u1", IsAdmin: false})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m1", LibraryID: "lib-secret", Title: "Secret"})
	// grant access only to lib-other, not lib-secret
	_ = f.accessRepo.Grant(context.Background(), "u1", "lib-other")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/favorites/m1", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestToggleFavorite_Allowed(t *testing.T) {
	f := newFavoriteTestFixture(t, domainuser.Claims{UserID: "u1", IsAdmin: false})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m1", LibraryID: "lib1", Title: "Allowed"})
	// no restrictions = full access

	req := httptest.NewRequest(http.MethodPost, "/api/v1/favorites/m1", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestCheckFavorite_IDOR_Denied(t *testing.T) {
	f := newFavoriteTestFixture(t, domainuser.Claims{UserID: "u1", IsAdmin: false})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m1", LibraryID: "lib-secret", Title: "Secret"})
	_ = f.accessRepo.Grant(context.Background(), "u1", "lib-other")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites/m1", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestCheckFavorite_Allowed(t *testing.T) {
	f := newFavoriteTestFixture(t, domainuser.Claims{UserID: "u1", IsAdmin: false})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m1", LibraryID: "lib1", Title: "Allowed"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites/m1", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestListFavorites_FiltersInaccessible(t *testing.T) {
	f := newFavoriteTestFixture(t, domainuser.Claims{UserID: "u1", ProfileID: "p1", IsAdmin: false})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m1", LibraryID: "lib1", Title: "Visible"})
	f.mediaRepo.Seed(&library.MediaItem{ID: "m2", LibraryID: "lib-secret", Title: "Hidden"})
	// restrict user: only lib1
	_ = f.accessRepo.Grant(context.Background(), "u1", "lib1")
	// add both as favorites
	_ = f.favRepo.Add(context.Background(), &favorite.Favorite{ProfileID: "p1", MediaItemID: "m1", CreatedAt: time.Now()})
	_ = f.favRepo.Add(context.Background(), &favorite.Favorite{ProfileID: "p1", MediaItemID: "m2", CreatedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Items []struct {
			MediaItemID string `json:"mediaItemId"`
		} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].MediaItemID != "m1" {
		t.Errorf("expected m1, got %s", resp.Items[0].MediaItemID)
	}
}
