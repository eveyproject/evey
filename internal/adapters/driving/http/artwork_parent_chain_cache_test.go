package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainmeta "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// mapMediaItemGetter answers GetMediaItem per-id from a fixed map, unlike
// fakeMediaItemGetter (media_handler_test.go) which ignores the id argument
// and always returns the same item - this test needs episode and season
// lookups to resolve to DIFFERENT items.
type mapMediaItemGetter struct {
	items map[string]*domainlibrary.MediaItem
}

func (m *mapMediaItemGetter) GetMediaItem(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	it, ok := m.items[id]
	if !ok {
		return nil, errTest
	}
	return it, nil
}

// countingArtworkRepo wraps an ArtworkReader and counts GetByMediaItem calls
// per id, to prove the parent-chain walk memoises repeat lookups instead of
// re-querying per sibling request.
type countingArtworkRepo struct {
	inner ArtworkReader
	mu    sync.Mutex
	calls map[string]int
}

func newCountingArtworkRepo(inner ArtworkReader) *countingArtworkRepo {
	return &countingArtworkRepo{inner: inner, calls: map[string]int{}}
}

func (c *countingArtworkRepo) GetByMediaItem(ctx context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error) {
	c.mu.Lock()
	c.calls[mediaItemID]++
	c.mu.Unlock()
	return c.inner.GetByMediaItem(ctx, mediaItemID)
}

func (c *countingArtworkRepo) count(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[id]
}

// countingMediaItemGetter wraps a MediaItemGetter and counts GetMediaItem
// calls per id, mirroring countingArtworkRepo above.
type countingMediaItemGetter struct {
	inner MediaItemGetter
	mu    sync.Mutex
	calls map[string]int
}

func newCountingMediaItemGetter(inner MediaItemGetter) *countingMediaItemGetter {
	return &countingMediaItemGetter{inner: inner, calls: map[string]int{}}
}

func (c *countingMediaItemGetter) GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error) {
	c.mu.Lock()
	c.calls[id]++
	c.mu.Unlock()
	return c.inner.GetMediaItem(ctx, id)
}

func (c *countingMediaItemGetter) count(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[id]
}

// TestArtwork_ParentChainMemoized_AcrossSiblingRequests proves the N+1: a
// season with no episode-level artwork falls back to its own poster, and
// EVERY episode in the season repeats the identical
// GetByMediaItem(season)/GetMediaItem(season) pair (findArtworkRecords'
// parent-chain walk) to resolve it. A poster grid renders every sibling at
// once, so without memoisation this pair is re-queried once per episode
// instead of once per season.
func TestArtwork_ParentChainMemoized_AcrossSiblingRequests(t *testing.T) {
	// Test-only: guarantee a clean slate regardless of what a PRIOR test (or a
	// prior run of this same test) left behind - see resetArtworkMemoCaches.
	resetArtworkMemoCaches()

	const numEpisodes = 5
	seasonSourceURL := "https://img.example/season-poster.jpg"

	items := map[string]*domainlibrary.MediaItem{
		"season1": {ID: "season1", LibraryID: "lib1", ParentID: "series1"},
	}
	records := map[string][]*domainmeta.ArtworkRecord{
		"season1": {{ID: "sp1", MediaItemID: "season1", Type: "poster", SourceURL: &seasonSourceURL}},
		// episodes deliberately carry no artwork record of their own, forcing
		// the walk up to the season on every single request.
	}
	for i := range numEpisodes {
		id := fmt.Sprintf("ep%d", i)
		items[id] = &domainlibrary.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib1", ParentID: "season1"}
	}

	repo := newCountingArtworkRepo(&fakeArtworkRepo{records: records})
	getter := newCountingMediaItemGetter(&mapMediaItemGetter{items: items})

	h := mediaHandler(getter, repo, "http://localhost", allowAll())

	for i := range numEpisodes {
		id := fmt.Sprintf("ep%d", i)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+id+"/artwork/poster", nil)
		req.SetPathValue("id", id)
		req.SetPathValue("artType", "poster")
		req = withHTTPClaims(req, testClaims(false))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusFound {
			t.Fatalf("episode %s: expected 302 redirect to season poster, got %d: %s", id, w.Code, w.Body.String())
		}
		if loc := w.Header().Get("Location"); loc != seasonSourceURL {
			t.Fatalf("episode %s: expected redirect to season poster %s, got %s", id, seasonSourceURL, loc)
		}
	}

	if got := repo.count("season1"); got != 1 {
		t.Errorf("GetByMediaItem(season1) called %d times across %d siblings, want 1 (memoized)", got, numEpisodes)
	}
	if got := getter.count("season1"); got != 1 {
		t.Errorf("GetMediaItem(season1) called %d times across %d siblings, want 1 (memoized)", got, numEpisodes)
	}
}
