package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

type fakeCollectionSource struct {
	col      *domainlibrary.Collection
	colErr   error
	items    []*domainlibrary.MediaItem
	itemsErr error
}

func (f *fakeCollectionSource) GetByID(_ context.Context, _ string) (*domainlibrary.Collection, error) {
	return f.col, f.colErr
}

func (f *fakeCollectionSource) GetItems(_ context.Context, _ string, _, _ int) ([]*domainlibrary.MediaItem, int, error) {
	return f.items, len(f.items), f.itemsErr
}

// fakeLibSetAccessChecker grants access only to the listed library IDs.
type fakeLibSetAccessChecker struct{ allowed map[string]bool }

func (f *fakeLibSetAccessChecker) HasAccess(_ context.Context, _ shared.UserID, lib shared.LibraryID) (bool, error) {
	return f.allowed[string(lib)], nil
}

func runCollectionDownload(t *testing.T, h *collectionHandler, claims *domainuser.Claims) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/c1/download", nil)
	req.SetPathValue("id", "c1")
	req = withHTTPClaims(req, claims)
	w := httptest.NewRecorder()
	h.downloadCollection(w, req)
	return w
}

func TestDownloadCollection_MixedMovieAndSeries(t *testing.T) {
	dir := t.TempDir()
	movie := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "Some Movie", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "some-movie.mkv", "movie")}}
	series := &domainlibrary.MediaItem{ID: "sr1", LibraryID: "lib1", Type: domainlibrary.Series, Title: "My Show"}
	season := &domainlibrary.MediaItem{ID: "s1", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 1", IndexNumber: new(1)}
	ep := &domainlibrary.MediaItem{ID: "ep1", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Pilot", IndexNumber: new(1), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "ep1.mkv", "ep")}}

	h := &collectionHandler{
		collSvc:    &fakeCollectionSource{col: &domainlibrary.Collection{Name: "Faves"}, items: []*domainlibrary.MediaItem{movie, series}},
		libSvc:     &fakeMediaItemGetter{children: map[string][]*domainlibrary.MediaItem{"sr1": {season}, "s1": {ep}}},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	w := runCollectionDownload(t, h, testClaims(false))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"Some Movie.mkv", "My Show/Season 01/E01 - Pilot.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("zip entries = %v, want %v", got, want)
	}
}

func TestDownloadCollection_CrossLibrary_FiltersToAccessible(t *testing.T) {
	dir := t.TempDir()
	mine := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "Mine", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "a.mkv", "a")}}
	foreign := &domainlibrary.MediaItem{ID: "m2", LibraryID: "lib2", Type: domainlibrary.MovieItem, Title: "Foreign", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "b.mkv", "b")}}

	h := &collectionHandler{
		collSvc:    &fakeCollectionSource{col: &domainlibrary.Collection{Name: "Mix"}, items: []*domainlibrary.MediaItem{mine, foreign}},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: &fakeLibSetAccessChecker{allowed: map[string]bool{"lib1": true}},
		baseURL:    "http://localhost",
	}
	w := runCollectionDownload(t, h, testClaims(false))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"Mine.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("non-admin should only get accessible-library items: got %v, want %v", got, want)
	}
}

func TestDownloadCollection_Admin_SeesAllLibraries(t *testing.T) {
	dir := t.TempDir()
	a := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "A", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "a.mkv", "a")}}
	b := &domainlibrary.MediaItem{ID: "m2", LibraryID: "lib2", Type: domainlibrary.MovieItem, Title: "B", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "b.mkv", "b")}}

	h := &collectionHandler{
		collSvc:    &fakeCollectionSource{col: &domainlibrary.Collection{Name: "All"}, items: []*domainlibrary.MediaItem{a, b}},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: denyAll(), // admin must bypass this
		baseURL:    "http://localhost",
	}
	w := runCollectionDownload(t, h, testClaims(true))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"A.mkv", "B.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("admin should get every library: got %v, want %v", got, want)
	}
}

func TestDownloadCollection_NoAccessibleItems_NotFound(t *testing.T) {
	dir := t.TempDir()
	foreign := &domainlibrary.MediaItem{ID: "m2", LibraryID: "lib2", Type: domainlibrary.MovieItem, Title: "Foreign", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "b.mkv", "b")}}
	h := &collectionHandler{
		collSvc:    &fakeCollectionSource{col: &domainlibrary.Collection{Name: "Mix"}, items: []*domainlibrary.MediaItem{foreign}},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: denyAll(),
		baseURL:    "http://localhost",
	}
	w := runCollectionDownload(t, h, testClaims(false))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when no items are accessible, got %d", w.Code)
	}
}

func TestDownloadCollection_NotFound(t *testing.T) {
	h := &collectionHandler{
		collSvc:    &fakeCollectionSource{col: nil, colErr: errTest},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	w := runCollectionDownload(t, h, testClaims(true))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing collection, got %d", w.Code)
	}
}

// A download token is bound to the one collection it was minted for; replayed
// on another it must be refused before the archive is built. Neutralising the
// check left the package green (sabotage sweep, 2026-09-28).
func TestDownloadCollection_TokenBoundElsewhere_Forbidden(t *testing.T) {
	movie := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "Some Movie", File: &domainlibrary.MediaFile{Path: archiveTempFile(t, t.TempDir(), "some-movie.mkv", "movie")}}
	h := &collectionHandler{
		collSvc:    &fakeCollectionSource{col: &domainlibrary.Collection{Name: "Faves"}, items: []*domainlibrary.MediaItem{movie}},
		libSvc:     &fakeMediaItemGetter{},
		libChecker: allowAll(),
		baseURL:    "http://localhost",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections/c1/download", nil)
	req.SetPathValue("id", "c1")
	req = withHTTPClaims(req, testClaims(false))
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{UserID: "user-1", MediaItemID: "c-other", Scope: shared.ScopeStream}))
	w := httptest.NewRecorder()
	h.downloadCollection(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("a token bound to another collection must be refused, got %d", w.Code)
	}
}
