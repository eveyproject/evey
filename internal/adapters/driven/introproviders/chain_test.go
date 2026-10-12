package introproviders

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type fakeClient struct {
	source string
	segs   []domainlibrary.MediaSegment
	err    error
	calls  int
}

func (f *fakeClient) Source() string { return f.source }
func (f *fakeClient) Fetch(_ context.Context, _ Lookup) ([]domainlibrary.MediaSegment, error) {
	f.calls++
	return f.segs, f.err
}

type fakeRepo struct {
	items map[shared.MediaItemID]*domainlibrary.MediaItem
}

func (f *fakeRepo) GetByID(_ context.Context, id shared.MediaItemID) (*domainlibrary.MediaItem, error) {
	return f.items[id], nil
}

// minimalEpisodeFixture builds the (episode, season, series) trio
// AncestorResolver expects: episode points at season via ParentID,
// season points at series via ParentID, and the series carries the
// TMDB+IMDb IDs in its Catalog.
func minimalEpisodeFixture(t *testing.T) (*domainlibrary.MediaItem, *fakeRepo) {
	t.Helper()
	ep := 5
	se := 1
	episode := &domainlibrary.MediaItem{
		ID: "ep1", Type: domainlibrary.Episode, ParentID: "season1",
		IndexNumber: &ep,
	}
	season := &domainlibrary.MediaItem{
		ID: "season1", Type: domainlibrary.Season, ParentID: "series1",
		IndexNumber: &se,
	}
	series := &domainlibrary.MediaItem{
		ID: "series1", Type: domainlibrary.Series,
		Catalog: &domainlibrary.MediaCatalog{
			ProviderIDs: map[string]string{"tmdb": "1396", "imdb": "tt0903747"},
		},
	}
	return episode, &fakeRepo{items: map[shared.MediaItemID]*domainlibrary.MediaItem{
		"season1": season, "series1": series,
	}}
}

// First non-empty client wins; the second client is never called.
func TestChain_FirstHit_ShortCircuits(t *testing.T) {
	t.Parallel()
	episode, repo := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repo)

	winner := &fakeClient{source: "tidb", segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60000, Source: "tidb"},
	}}
	skipped := &fakeClient{source: "introdb"}

	provider := NewChainedProvider(resolver, winner, skipped)
	got, err := provider.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "tidb", got[0].Source)
	assert.Equal(t, 1, winner.calls)
	assert.Equal(t, 0, skipped.calls, "second client must not run after first hit")
}

// Empty result from first → fall through to second.
func TestChain_EmptyFirst_FallsThrough(t *testing.T) {
	t.Parallel()
	episode, repo := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repo)

	first := &fakeClient{source: "tidb"} // nil segs
	second := &fakeClient{source: "introdb", segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 60000, Source: "introdb"},
	}}

	provider := NewChainedProvider(resolver, first, second)
	got, err := provider.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "introdb", got[0].Source)
	assert.Equal(t, 1, first.calls)
	assert.Equal(t, 1, second.calls)
}

// 429 from first → cooldown set; subsequent calls within the cooldown
// window skip the rate-limited source. This is the throttling shield
// - without it a burst of episodes would burn through quotas.
func TestChain_RateLimited_SetsCooldown(t *testing.T) {
	t.Parallel()
	episode, repo := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repo)

	limited := &fakeClient{source: "tidb", err: ErrRateLimited{Source: "tidb"}}
	other := &fakeClient{source: "introdb"}

	provider := NewChainedProvider(resolver, limited, other).WithCooldown(50 * time.Millisecond)

	// 1st call: tidb 429 → cooldown engaged, fall through to introdb.
	_, err := provider.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	assert.Equal(t, 1, limited.calls)
	assert.Equal(t, 1, other.calls)

	// 2nd call within cooldown: tidb is skipped, only introdb is hit.
	_, err = provider.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	assert.Equal(t, 1, limited.calls, "tidb must still be silent during cooldown")
	assert.Equal(t, 2, other.calls)

	// Wait out the cooldown - tidb retries.
	time.Sleep(80 * time.Millisecond)
	_, err = provider.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	assert.Equal(t, 2, limited.calls, "tidb must retry after cooldown elapses")
}

// Generic non-429 error from one client logs+falls through; doesn't
// poison the chain.
func TestChain_NonRateError_FallsThrough(t *testing.T) {
	t.Parallel()
	episode, repo := minimalEpisodeFixture(t)
	resolver := NewAncestorResolver(repo)

	flaky := &fakeClient{source: "tidb", err: errors.New("connection refused")}
	winner := &fakeClient{source: "introdb", segs: []domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 1000, Source: "introdb"},
	}}

	provider := NewChainedProvider(resolver, flaky, winner)
	got, err := provider.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "introdb", got[0].Source)
}

// Item without IDs (no enriched catalog) → silent skip, neither client
// is called. The pipeline falls through to chapter pass.
func TestChain_UnresolvableLookup_NoCalls(t *testing.T) {
	t.Parallel()
	episode := &domainlibrary.MediaItem{ID: "ep1", Type: domainlibrary.Episode}
	resolver := NewAncestorResolver(&fakeRepo{items: map[shared.MediaItemID]*domainlibrary.MediaItem{}})

	tidb := &fakeClient{source: "tidb"}
	introdb := &fakeClient{source: "introdb"}

	provider := NewChainedProvider(resolver, tidb, introdb)
	got, err := provider.GetSegments(context.Background(), episode)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Equal(t, 0, tidb.calls)
	assert.Equal(t, 0, introdb.calls)
}
