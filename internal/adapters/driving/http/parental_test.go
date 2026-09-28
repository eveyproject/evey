package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

// fakeProfileGetter keys on the requested ID rather than returning the same
// profile whatever it is asked for.
//
// The permissive version hid an entire bug class: evaluating one profile's
// request against ANOTHER profile's parental settings was invisible to the
// suite. Mutation-proved - pointing the checker at a hardcoded foreign profile
// ID left the whole http package green. Now an unknown ID returns nil, and the
// checker fails closed on it, so the mutation trips a test.
type fakeProfileGetter struct {
	profile *domainuser.Profile
	err     error
}

func (f *fakeProfileGetter) GetProfileByID(_ context.Context, id string) (*domainuser.Profile, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.profile == nil || string(f.profile.ID) != id {
		return nil, nil
	}
	return f.profile, nil
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
		{"profile lookup error fails closed", nil, errors.New("db blip"), ratedItem("R"), true},
		{"nil profile without error fails closed", nil, nil, ratedItem("R"), true},
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
	// An all-ages certification, so the kids age rule lets the title through
	// and only the tag can block it: an unrated item failed closed on its own
	// and hid a tag check that did nothing (sabotage sweep, 2026-09-28).
	allAges := &domainlibrary.MediaClassification{CertificationsByCountry: map[string]string{"US": "G"}}
	item := &domainlibrary.MediaItem{ID: "m1", Classification: allAges, Catalog: &domainlibrary.MediaCatalog{Tags: []string{"Horror"}}}
	if !pc.BlockedByParental(context.Background(), "p1", item) {
		t.Error("expected item with a blocked tag to be blocked")
	}
	clean := &domainlibrary.MediaItem{ID: "m2", Classification: allAges, Catalog: &domainlibrary.MediaCatalog{Tags: []string{"Comedy"}}}
	if pc.BlockedByParental(context.Background(), "p1", clean) {
		t.Error("control: the same all-ages title without the tag must pass")
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
	t.Run("blocked kid gets 404 (existence hidden)", func(t *testing.T) {
		w := httptest.NewRecorder()
		if checkParental(context.Background(), w, &domainuser.Claims{ProfileID: "p1"}, item, blocker) {
			t.Error("blocked content should not pass")
		}
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
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

// allowAllParental never blocks - used when a test exercises handler logic past
// the parental gate without an admin claim doing the bypassing.
type allowAllParental struct{}

func (allowAllParental) BlockedByParental(context.Context, string, *domainlibrary.MediaItem) bool {
	return false
}

// TestMediaStream_ParentalBlocked_NotFound guards the parental gate on the main
// streaming endpoint (handleStream / withParental). Production wires
// withParental(parentalChk) in server.go, but no test exercised a blocking
// checker through it - a regression dropping the gate from the primary flux
// would have passed unnoticed. The temp file makes the negative control real:
// with the gate open the bytes stream (200), so the 404 below is the gate.
func TestMediaStream_ParentalBlocked_NotFound(t *testing.T) {
	tmp := t.TempDir()
	mediaPath := filepath.Join(tmp, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("fake-media-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: shared.LibraryID("lib1"),
		File:      &domainlibrary.MediaFile{Path: mediaPath},
	}
	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1", nil)
		req.SetPathValue("id", "m1")
		return withHTTPClaims(req, testClaims(false))
	}
	build := func(pc ParentalChecker) http.HandlerFunc {
		return mediaHandler(
			&fakeMediaItemGetter{item: item},
			nil, // artworkRepo: unused on the stream path
			"http://localhost",
			&fakeLibraryAccessChecker{allowed: true}, // library access granted…
			withParental(pc),
		)
	}

	// Control: gate open → the existing file streams.
	wOK := httptest.NewRecorder()
	build(allowAllParental{}).ServeHTTP(wOK, newReq())
	if wOK.Code != http.StatusOK {
		t.Fatalf("control: open parental gate should stream the file, got %d: %s", wOK.Code, wOK.Body.String())
	}

	// …but parental blocks → 404, hiding the item's existence.
	wBlocked := httptest.NewRecorder()
	build(alwaysBlockChecker{}).ServeHTTP(wBlocked, newReq())
	if wBlocked.Code != http.StatusNotFound {
		t.Errorf("parental-blocked stream must return 404 (existence hidden), got %d", wBlocked.Code)
	}
}

// The 404 must come from the PARENTAL GATE, not from anywhere else that also
// 404s. The previous version of this test pointed at "/tmp/fake.mkv", a file
// that does not exist: serveDownload's os.Stat failed and emitted the very same
// 404, so deleting the gate from downloadHandler left the whole package green.
// Verified by mutation - the gate could be removed undetected.
//
// A real file plus the open-gate control makes the negative real, exactly as
// TestMediaStream_ParentalBlocked_NotFound above already did.
func TestDownloadHandler_ParentalBlocked_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(path, []byte("mkv-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(pc ParentalChecker) http.HandlerFunc {
		return downloadHandler(
			&fakeMediaItemGetter{
				item:      &domainlibrary.MediaItem{ID: "m1", LibraryID: shared.LibraryID("lib1"), File: &domainlibrary.MediaFile{Path: path}},
				libraries: []*domainlibrary.Library{{ID: "lib1", Paths: []string{dir}}},
			},
			"http://localhost",
			&fakeLibraryAccessChecker{allowed: true}, // library access granted…
			pc,
		)
	}
	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/download", nil)
		req.SetPathValue("id", "m1")
		return withHTTPClaims(req, testClaims(false))
	}

	// Control: with the gate open the bytes are served. Without this, a 404
	// below proves nothing - it could come from a missing file or a bad root.
	wOK := httptest.NewRecorder()
	build(allowAllParental{}).ServeHTTP(wOK, newReq())
	if wOK.Code != http.StatusOK {
		t.Fatalf("control: open parental gate should serve the download, got %d: %s", wOK.Code, wOK.Body.String())
	}

	// …and with it closed, 404 - hiding the item's existence.
	wBlocked := httptest.NewRecorder()
	build(alwaysBlockChecker{}).ServeHTTP(wBlocked, newReq())
	if wBlocked.Code != http.StatusNotFound {
		t.Errorf("parental-blocked download must return 404 (existence hidden), got %d", wBlocked.Code)
	}
}

// blockAllParental blocks every item for a non-empty profile.
type blockAllParental struct{ calls int }

func (b *blockAllParental) BlockedByParental(_ context.Context, profileID string, item *domainlibrary.MediaItem) bool {
	if profileID == "" || item == nil {
		return false
	}
	b.calls++
	return true
}

// The ZIP paths (collection / playlist / season / series) used to filter only
// on library access, so a parentally-restricted profile could download exactly
// the media the streaming route answers 404 for. The archive builder must apply
// the same gate, per item AND per episode inside a season/series container.
func TestBuildMediaArchiveEntries_AppliesParentalPerItem(t *testing.T) {
	pc := &blockAllParental{}
	allow := parentalAllowFunc(context.Background(), &domainuser.Claims{UserID: "usr-1", ProfileID: "prf-kid"}, pc)

	items := []*domainlibrary.MediaItem{
		{ID: "mi-1", LibraryID: "lib-1", Title: "Adult Movie", Type: domainlibrary.MovieItem,
			File: &domainlibrary.MediaFile{Path: "/movies/adult.mkv"}},
	}
	entries := buildMediaArchiveEntries(context.Background(), nil, items,
		func(shared.LibraryID) bool { return true }, allow, []string{"/movies"})

	if len(entries) != 0 {
		t.Fatalf("a parentally-blocked item must not land in the archive, got %d entries", len(entries))
	}
	if pc.calls == 0 {
		t.Error("the parental checker was never consulted")
	}
}

func TestBuildMediaArchiveEntries_AdminKeepsEverything(t *testing.T) {
	pc := &blockAllParental{}
	// Admins bypass the gate upstream, exactly like checkParental does.
	allow := parentalAllowFunc(context.Background(), &domainuser.Claims{UserID: "usr-1", ProfileID: "prf-admin", IsAdmin: true}, pc)

	items := []*domainlibrary.MediaItem{
		{ID: "mi-1", LibraryID: "lib-1", Title: "Adult Movie", Type: domainlibrary.MovieItem,
			File: &domainlibrary.MediaFile{Path: "/movies/adult.mkv"}},
	}
	entries := buildMediaArchiveEntries(context.Background(), nil, items,
		func(shared.LibraryID) bool { return true }, allow, []string{"/movies"})

	if len(entries) != 1 {
		t.Fatalf("an admin must still get the file, got %d entries", len(entries))
	}
}
