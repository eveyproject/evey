package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	appmedia "github.com/evey-project/evey/internal/app/media"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// Compile-time check: fakeMediaItemGetter must satisfy MediaArchiveSource so
// it can be passed to the widened shareDownloadHandler parameter.
var _ MediaArchiveSource = (*fakeMediaItemGetter)(nil)

func TestShareDownloadHandler_ServesFileForValidToken(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(fpath, []byte("VIDEODATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := sharedfakes.NewFakeShareLinkRepo()
	repo.Seed(&shared.ShareLink{ID: "s1", Token: "tok-valid", MediaItemID: "m1", ExpiresAt: time.Now().Add(time.Hour)})
	getter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{
		ID: "m1", LibraryID: "lib1", Title: "Movie",
		File: &domainlibrary.MediaFile{Path: fpath},
	}}
	h := shareDownloadHandler(appmedia.NewShareService(repo), getter, "http://x")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/tok-valid/download", nil)
	req.SetPathValue("token", "tok-valid")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "VIDEODATA" {
		t.Errorf("body = %q, want VIDEODATA", rec.Body.String())
	}
}

func TestShareDownloadHandler_UnknownToken404(t *testing.T) {
	repo := sharedfakes.NewFakeShareLinkRepo()
	getter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	h := shareDownloadHandler(appmedia.NewShareService(repo), getter, "http://x")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/nope/download", nil)
	req.SetPathValue("token", "nope")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestShareDownloadHandler_PathOutsideRoots_404(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(fpath, []byte("VIDEODATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := sharedfakes.NewFakeShareLinkRepo()
	repo.Seed(&shared.ShareLink{ID: "s1", Token: "tok-conf", MediaItemID: "m1", ExpiresAt: time.Now().Add(time.Hour)})
	// roots does NOT include dir - confinement should block serving
	otherDir := t.TempDir()
	getter := &fakeMediaItemGetter{
		item: &domainlibrary.MediaItem{
			ID: "m1", LibraryID: "lib1", Title: "Movie",
			File: &domainlibrary.MediaFile{Path: fpath},
		},
		libraries: []*domainlibrary.Library{
			{ID: "lib1", Paths: []string{otherDir}},
		},
	}
	h := shareDownloadHandler(appmedia.NewShareService(repo), getter, "http://x")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/tok-conf/download", nil)
	req.SetPathValue("token", "tok-conf")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404 (path outside roots)", rec.Code)
	}
}

func TestShareDownloadHandler_ExpiredToken410(t *testing.T) {
	repo := sharedfakes.NewFakeShareLinkRepo()
	repo.Seed(&shared.ShareLink{ID: "s2", Token: "tok-exp", MediaItemID: "m1", ExpiresAt: time.Now().Add(-time.Hour)})
	getter := &fakeMediaItemGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1"}}
	h := shareDownloadHandler(appmedia.NewShareService(repo), getter, "http://x")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/share/tok-exp/download", nil)
	req.SetPathValue("token", "tok-exp")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("code = %d, want 410", rec.Code)
	}
}

// An admin shares a whole series or one season: the link serves the same ZIP
// as an authenticated download (it answered 404, having no file of its own).
func TestShareDownloadHandler_SeriesAndSeasonServeZIP(t *testing.T) {
	dir := t.TempDir()
	ep1 := &domainlibrary.MediaItem{ID: "ep1", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Pilot", IndexNumber: new(1), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e1.mkv", "one")}}
	ep2 := &domainlibrary.MediaItem{ID: "ep2", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Finale", IndexNumber: new(2), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e2.mkv", "two")}}
	s1 := &domainlibrary.MediaItem{ID: "s1", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 1", IndexNumber: new(1)}
	s2 := &domainlibrary.MediaItem{ID: "s2", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 2", IndexNumber: new(2)}
	series := &domainlibrary.MediaItem{ID: "sr1", LibraryID: "lib1", Type: domainlibrary.Series, Title: "My Show"}
	children := map[string][]*domainlibrary.MediaItem{"sr1": {s1, s2}, "s1": {ep1}, "s2": {ep2}}

	for _, tc := range []struct {
		item *domainlibrary.MediaItem
		want []string
	}{
		{series, []string{"Season 01/E01 - Pilot.mkv", "Season 02/E02 - Finale.mkv"}},
		{s2, []string{"E02 - Finale.mkv"}},
	} {
		repo := sharedfakes.NewFakeShareLinkRepo()
		repo.Seed(&shared.ShareLink{ID: "l1", Token: "tok", MediaItemID: tc.item.ID, ExpiresAt: time.Now().Add(time.Hour)})
		h := shareDownloadHandler(appmedia.NewShareService(repo), &fakeMediaItemGetter{item: tc.item, children: children}, "http://x")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/share/tok/download", nil)
		req.SetPathValue("token", "tok")
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: code = %d, want 200 (%s)", tc.item.Title, rec.Code, rec.Body.String())
		}
		if got := zipEntryNames(t, rec.Body.Bytes()); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: zip entries = %v, want %v", tc.item.Title, got, tc.want)
		}
	}
}

// A view limit is a hard limit: a resumed (ranged) request spends a view like
// any other, so "bytes=1-" cannot re-download a link whose views are gone.
func TestShareDownloadHandler_RangeRequestSpendsAView(t *testing.T) {
	dir := t.TempDir()
	ep := &domainlibrary.MediaItem{ID: "ep1", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Pilot", IndexNumber: new(1), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e1.mkv", "one")}}
	season := &domainlibrary.MediaItem{ID: "s1", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 1", IndexNumber: new(1)}
	repo := sharedfakes.NewFakeShareLinkRepo()
	repo.Seed(&shared.ShareLink{ID: "l1", Token: "tok", MediaItemID: "s1", ExpiresAt: time.Now().Add(time.Hour), MaxViews: new(1)})
	h := shareDownloadHandler(appmedia.NewShareService(repo), &fakeMediaItemGetter{item: season, children: map[string][]*domainlibrary.MediaItem{"s1": {ep}}}, "http://x")

	codes := []int{}
	for _, rng := range []string{"bytes=10-", "bytes=0-9"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/share/tok/download", nil)
		req.SetPathValue("token", "tok")
		req.Header.Set("Range", rng)
		rec := httptest.NewRecorder()
		h(rec, req)
		codes = append(codes, rec.Code)
	}
	if !reflect.DeepEqual(codes, []int{http.StatusPartialContent, http.StatusGone}) {
		t.Fatalf("codes = %v, want [206 410]: the resume must not outlive the view limit", codes)
	}
}
