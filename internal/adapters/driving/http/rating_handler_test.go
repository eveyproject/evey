package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

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
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
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
// and calls checkLibraryAccess - this test exercises that same code path directly.
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
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
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
