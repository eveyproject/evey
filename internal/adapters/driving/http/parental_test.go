package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

type fakeProfileGetter struct {
	profile *domainuser.Profile
	err     error
}

func (f *fakeProfileGetter) GetProfileByID(_ context.Context, _ string) (*domainuser.Profile, error) {
	return f.profile, f.err
}

func kidProfile(maxAge int) *domainuser.Profile {
	return &domainuser.Profile{
		ID: "p1", UserID: "u1", IsKids: true,
		Parental: &domainuser.ParentalSettings{MaxAge: &maxAge, Enabled: true},
	}
}

func ratedItem(officialRating string) *domainlibrary.MediaItem {
	return &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1",
		Classification: &domainlibrary.MediaClassification{OfficialRating: officialRating},
	}
}

func TestParentalChecker_BlockedByParental(t *testing.T) {
	tests := []struct {
		name    string
		profile *domainuser.Profile
		err     error
		item    *domainlibrary.MediaItem
		blocked bool
	}{
		{"kid blocked by over-age rating", kidProfile(10), nil, ratedItem("R"), true},   // R=17 > 10
		{"kid allowed by suitable rating", kidProfile(13), nil, ratedItem("PG"), false}, // PG=10 <= 13
		{"kid fails closed on unresolvable rating", kidProfile(10), nil, ratedItem(""), true},
		{"kid fails closed on unknown rating", kidProfile(10), nil, ratedItem("ZZ-bogus"), true},
		{"non-kid profile never blocked", &domainuser.Profile{ID: "p1", IsKids: false}, nil, ratedItem("R"), false},
		{"unresolvable profile does not assert a block", nil, errors.New("not found"), ratedItem("R"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewParentalChecker(&fakeProfileGetter{profile: tt.profile, err: tt.err})
			if got := pc.BlockedByParental(context.Background(), "p1", tt.item); got != tt.blocked {
				t.Errorf("BlockedByParental = %v, want %v", got, tt.blocked)
			}
		})
	}
}

func TestParentalChecker_BlockedTag(t *testing.T) {
	p := &domainuser.Profile{ID: "p1", IsKids: true, Parental: &domainuser.ParentalSettings{Enabled: true, BlockedTags: []string{"horror"}}}
	pc := NewParentalChecker(&fakeProfileGetter{profile: p})
	item := &domainlibrary.MediaItem{ID: "m1", Catalog: &domainlibrary.MediaCatalog{Tags: []string{"Horror"}}}
	if !pc.BlockedByParental(context.Background(), "p1", item) {
		t.Error("expected item with a blocked tag to be blocked")
	}
}

func TestCheckParental(t *testing.T) {
	blocker := NewParentalChecker(&fakeProfileGetter{profile: kidProfile(10)}) // blocks R
	item := ratedItem("R")

	t.Run("nil checker is a no-op", func(t *testing.T) {
		w := httptest.NewRecorder()
		if !checkParental(context.Background(), w, &domainuser.Claims{ProfileID: "p1"}, item, nil) {
			t.Error("nil checker should pass")
		}
	})
	t.Run("admin bypasses parental", func(t *testing.T) {
		w := httptest.NewRecorder()
		if !checkParental(context.Background(), w, &domainuser.Claims{ProfileID: "p1", IsAdmin: true}, item, blocker) {
			t.Error("admin should bypass parental")
		}
	})
	t.Run("blocked kid gets 403", func(t *testing.T) {
		w := httptest.NewRecorder()
		if checkParental(context.Background(), w, &domainuser.Claims{ProfileID: "p1"}, item, blocker) {
			t.Error("blocked content should not pass")
		}
		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})
	t.Run("allowed content passes", func(t *testing.T) {
		w := httptest.NewRecorder()
		if !checkParental(context.Background(), w, &domainuser.Claims{ProfileID: "p1"}, ratedItem("PG"), NewParentalChecker(&fakeProfileGetter{profile: kidProfile(13)})) {
			t.Error("suitable content should pass")
		}
	})
}

// alwaysBlockChecker is an in-test ParentalChecker that blocks everything; it
// proves a handler actually consults the gate after the library-access check.
type alwaysBlockChecker struct{}

func (alwaysBlockChecker) BlockedByParental(context.Context, string, *domainlibrary.MediaItem) bool {
	return true
}

func TestDownloadHandler_ParentalBlocked_403(t *testing.T) {
	h := downloadHandler(
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: shared.LibraryID("lib1"), File: &domainlibrary.MediaFile{Path: "/tmp/fake.mkv"}}},
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true}, // library access granted…
		alwaysBlockChecker{},                     // …but parental blocks
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 from parental gate, got %d", w.Code)
	}
}
