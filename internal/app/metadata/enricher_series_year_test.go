package metadata

import (
	"context"
	"log/slog"
	"testing"

	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	domain "github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two series share the exact title "Sherlock Holmes" and neither S01 count
// matches the three episodes on disk. The year the user wrote on the folder
// must pick the 1954 series; without a year the fallback score keeps its
// recency preference (the 1984 one), the symmetric case.
func TestSearchAndFetchBest_SeriesYearPicksAmongExactTitles(t *testing.T) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	y1954, y1984 := 1954, 1984
	old := domain.MetadataResult{Title: "Sherlock Holmes", Year: &y1954, ProviderIDs: map[string]string{"tmdb": "6560"}, SeasonEpisodeCounts: map[int]int{1: 39}}
	remake := domain.MetadataResult{Title: "Sherlock Holmes", Year: &y1984, ProviderIDs: map[string]string{"tmdb": "1030"}, SeasonEpisodeCounts: map[int]int{1: 13}, Popularity: 40}
	provider.SearchResults["Sherlock Holmes"] = []domain.MetadataResult{remake, old}
	provider.ByProviderID["6560"] = &old
	provider.ByProviderID["1030"] = &remake

	miRepo := libfakes.NewFakeMediaItemStore()
	e := NewEnricher(metafakes.NewFakeArtworkRepository(), libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		miRepo, miRepo, libfakes.NewFakeSearchIndex(), provider, nil, nil, nil)
	ctx := context.Background()

	got := e.searchAndFetchBest(ctx, provider, "Sherlock Holmes", &y1954, "tv", 3, slog.Default())
	require.NotNil(t, got)
	assert.Equal(t, "6560", got.ProviderIDs["tmdb"], "the folder year picks the 1954 series")

	got = e.searchAndFetchBest(ctx, provider, "Sherlock Holmes", nil, "tv", 3, slog.Default())
	require.NotNil(t, got)
	assert.Equal(t, "1030", got.ProviderIDs["tmdb"], "without a year the recency fallback still wins")
}
