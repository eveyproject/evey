package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apprating "github.com/gravity-zero/evey/internal/app/rating"
	libraryfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	ratingfakes "github.com/gravity-zero/evey/internal/domain/rating/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/auth"
)

func newRatingTestMux(t *testing.T, checker LibraryAccessChecker, mediaItems map[string]*domainlibrary.MediaItem) (*http.ServeMux, string) {
	t.Helper()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, _ := tokenGen.GenerateAccessToken(domainuser.Claims{UserID: "u1", IsAdmin: false, ProfileID: "prof-1"})

	ratingRepo := libraryfakes.NewFakeUserRatingRepository()
	starRepo := ratingfakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	starSvc := apprating.NewService(starRepo, bus)

	getter := newFakeMediaGetter()
	maps.Copy(getter.items, mediaItems)

	mux := http.NewServeMux()
	registerRatingRoutes(mux, ratingRepo, tokenGen, getter, checker)
	registerStarRatingRoutes(mux, starSvc, tokenGen, getter, checker)
	return mux, token
}

func TestRateMedia_NoAuth_401(t *testing.T) {
	mux, _ := newRatingTestMux(t, allowAll(), nil)
	body := `{"value":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/media-1/rate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetMyRating_NoAuth_401(t *testing.T) {
	mux, _ := newRatingTestMux(t, allowAll(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/my-rating", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPostStarRating_NoAuth_401(t *testing.T) {
	mux, _ := newRatingTestMux(t, allowAll(), nil)
	body := `{"value":8}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/media-1/rating", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetStarRating_NoAuth_401(t *testing.T) {
	mux, _ := newRatingTestMux(t, allowAll(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDeleteStarRating_NoAuth_401(t *testing.T) {
	mux, _ := newRatingTestMux(t, allowAll(), nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/media/media-1/rating", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRateMedia_IDOR_403(t *testing.T) {
	items := map[string]*domainlibrary.MediaItem{
		"media-1": {ID: "media-1", LibraryID: "lib-restricted"},
	}
	mux, token := newRatingTestMux(t, denyAll(), items)
	body := `{"value":1}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/media/media-1/rate", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestGetStarRatingAggregate_NoAuth_401(t *testing.T) {
	mux, _ := newRatingTestMux(t, allowAll(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/media-1/rating/aggregate", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRatingIDOR_DeniedWhenNoLibraryAccess(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()

	claims := &domainuser.Claims{UserID: "attacker", IsAdmin: false}
	getter := newFakeMediaGetter()
	getter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-restricted"}

	item, err := getter.GetMediaItem(ctx, "media-1")
	if err != nil || item == nil {
		t.Fatal("item not found in fake getter")
	}

	ok := checkLibraryAccess(ctx, w, claims, item.LibraryID, denyAll())
	if ok {
		t.Error("expected IDOR check to block access")
	}
	if w.Code != 403 {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRatingIDOR_AllowedWhenLibraryAccess(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()

	claims := &domainuser.Claims{UserID: "user-1", IsAdmin: false}
	getter := newFakeMediaGetter()
	getter.items["media-1"] = &domainlibrary.MediaItem{ID: "media-1", LibraryID: "lib-allowed"}

	item, err := getter.GetMediaItem(ctx, "media-1")
	if err != nil || item == nil {
		t.Fatal("item not found in fake getter")
	}

	ok := checkLibraryAccess(ctx, w, claims, item.LibraryID, allowAll())
	if !ok {
		t.Error("expected access to be allowed")
	}
}

// TestRatingAggregateIDOR verifies that the aggregate endpoint enforces library access.
// The checkMediaAccess closure used by the aggregate handler fetches the media item
// and calls checkLibraryAccess — this test exercises that same code path directly.
func TestRatingAggregateIDOR_Denied(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()

	claims := &domainuser.Claims{UserID: "attacker", IsAdmin: false}
	getter := newFakeMediaGetter()
	getter.items["media-agg"] = &domainlibrary.MediaItem{ID: "media-agg", LibraryID: "lib-secret"}

	item, err := getter.GetMediaItem(ctx, "media-agg")
	if err != nil || item == nil {
		t.Fatal("item not found")
	}
	ok := checkLibraryAccess(ctx, w, claims, item.LibraryID, denyAll())
	if ok {
		t.Error("aggregate IDOR: expected 403 for restricted library")
	}
	if w.Code != 403 {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRatingAggregateIDOR_Allowed(t *testing.T) {
	ctx := context.Background()
	w := httptest.NewRecorder()

	claims := &domainuser.Claims{UserID: "user-1", IsAdmin: false}
	getter := newFakeMediaGetter()
	getter.items["media-agg"] = &domainlibrary.MediaItem{ID: "media-agg", LibraryID: "lib-ok"}

	item, err := getter.GetMediaItem(ctx, "media-agg")
	if err != nil || item == nil {
		t.Fatal("item not found")
	}
	ok := checkLibraryAccess(ctx, w, claims, item.LibraryID, allowAll())
	if !ok {
		t.Error("aggregate IDOR: expected access for authorized user")
	}
}
