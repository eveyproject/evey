package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// A direct play arrives as a run of Range requests; the item is read once per
// TTL window, not once per request. The access checks still run on each.
func TestStream_ItemLookupIsCachedAcrossRangeRequests(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(media, []byte("mkv-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: media}}
	getter := newCountingMediaItemGetter(&fakeMediaItemGetter{item: item})
	h := mediaHandler(getter, &fakeArtworkRepo{}, "http://localhost", allowAll())

	for range 6 {
		if w := streamRequest(h); w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
		}
	}
	if got := getter.count("m1"); got != 1 {
		t.Fatalf("six range requests read the item %d times, want 1", got)
	}
}

func TestStream_ItemLookupDoesNotCacheAMiss(t *testing.T) {
	getter := newCountingMediaItemGetter(&fakeMediaItemGetter{err: os.ErrNotExist})
	h := mediaHandler(getter, &fakeArtworkRepo{}, "http://localhost", allowAll())
	for range 2 {
		if w := streamRequest(h); w.Code != http.StatusNotFound {
			t.Fatalf("want 404, got %d", w.Code)
		}
	}
	if got := getter.count("m1"); got != 2 {
		t.Fatalf("a miss must not be cached: asked %d times, want 2", got)
	}
}
