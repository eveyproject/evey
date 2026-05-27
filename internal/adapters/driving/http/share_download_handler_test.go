package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	// roots does NOT include dir — confinement should block serving
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
