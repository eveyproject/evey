package metadata

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	domain "github.com/evey-project/evey/internal/domain/metadata"
)

// Prod 2026-09-10: every boot re-queried the provider for every episode and
// every trailer-less film (9500 items at 4/s, 40 min of TMDB and search
// re-indexing per restart). An enriched item is skipped; force still refreshes.
func TestAutoEnrichAll_SkipsEnrichedItemsWithoutTrailers(t *testing.T) {
	primary := &countingProvider{fakeProvider: fakeProvider{
		name:          "tmdb",
		searchResults: []domain.MetadataResult{{Title: "X", ProviderIDs: map[string]string{"tmdb": "1"}}},
		detailResult:  &domain.MetadataResult{Title: "X", Overview: "new"},
	}}
	e, miRepo := newTestEnricherMulti(primary, nil)

	noTrailer := &domainlib.MediaItem{ID: "m1", Title: "X", Type: "movie", Overview: "already enriched"}
	miRepo.Create(context.Background(), noTrailer)

	e.AutoEnrichAll(context.Background(), []*domainlib.MediaItem{noTrailer}, nil)
	assert.Equal(t, int32(0), primary.searchCalls.Load(), "an enriched film is not re-queried because it lacks a trailer")

	e.ForceAutoEnrichAll(context.Background(), []*domainlib.MediaItem{noTrailer}, nil)
	assert.GreaterOrEqual(t, primary.searchCalls.Load(), int32(1), "force still refreshes")
}

func TestAutoEnrichItem_SkipsEnrichedEpisodesAndSeasons(t *testing.T) {
	primary := &countingProvider{fakeProvider: fakeProvider{name: "tmdb"}}
	e, miRepo := newTestEnricherMulti(primary, nil)

	series := &domainlib.MediaItem{ID: "s", Title: "Show", Type: "series", Overview: "ov"}
	one := 1
	season := &domainlib.MediaItem{ID: "se", Title: "S1", Type: "season", ParentID: "s", IndexNumber: &one, Overview: "ov"}
	ep := &domainlib.MediaItem{ID: "ep", Title: "E1", Type: "episode", ParentID: "se", IndexNumber: &one, Overview: "ov"}
	for _, it := range []*domainlib.MediaItem{series, season, ep} {
		miRepo.Create(context.Background(), it)
	}

	e.AutoEnrichAll(context.Background(), []*domainlib.MediaItem{season, ep}, nil)
	e.AutoEnrichItem(context.Background(), ep, nil)
	assert.Equal(t, int32(0), primary.searchCalls.Load(), "enriched episodes and seasons are left alone")
}
