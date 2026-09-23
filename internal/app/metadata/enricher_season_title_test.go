package metadata

import (
	"context"
	"log/slog"
	"testing"
	"time"

	domainlib "github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	domain "github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Product rule: a season has no name. Its label is derived from its index
// ("Season N" / "Saison N" client-side), so enrichment must never write a
// provider-supplied title onto a season - while every other enriched field
// (overview, artwork, air date, provider ids) must keep flowing.
//
// The provider here is the shared FakeMetadataProvider, which keys season
// details on "seriesID:seasonNumber" and returns not-found on any other key: a
// fake answering every key would hide whether the lookup addresses the right
// season.
func TestAutoEnrichSeason_NeverWritesProviderTitle(t *testing.T) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	air := time.Date(2002, 9, 29, 0, 0, 0, 0, time.UTC)
	provider.SeasonDetails["500:2"] = &domain.MetadataResult{
		Title:        "Gigantor",
		Overview:     "Second season",
		PremiereDate: &air,
		ProviderIDs:  map[string]string{"tmdb": "9002"},
		Artwork:      []domain.ArtworkResult{{Type: domain.ArtworkTypePoster, URL: "https://img/s2.jpg"}},
	}

	artRepo := metafakes.NewFakeArtworkRepository()
	miRepo := libfakes.NewFakeMediaItemStore()
	e := NewEnricher(artRepo, libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		miRepo,
		miRepo, libfakes.NewFakeSearchIndex(), provider, nil, nil)

	ctx := context.Background()
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{
		ID:       "series-1",
		Title:    "Some Series",
		Overview: "Series overview",
		Type:     domainlib.Series,
		Catalog:  &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "500"}},
	}))
	seasonNum := 2
	season := &domainlib.MediaItem{
		ID:          "season-2",
		ParentID:    "series-1",
		Type:        domainlib.Season,
		Title:       "Season 2",
		IndexNumber: &seasonNum,
	}
	require.NoError(t, miRepo.Create(ctx, season))

	e.AutoEnrichItem(ctx, season, slog.Default())

	// The rest of the enrichment must still land.
	assert.Equal(t, "Second season", season.Overview, "season overview must still be enriched")
	require.NotNil(t, season.Catalog)
	assert.Equal(t, "9002", season.Catalog.ProviderIDs["tmdb"])
	require.NotNil(t, season.Catalog.PremiereDate)
	assert.Equal(t, air, *season.Catalog.PremiereDate)
	art, err := artRepo.GetByMediaItem(ctx, "season-2")
	require.NoError(t, err)
	assert.Len(t, art, 1, "season poster must still be stored")

	// But the title must not.
	assert.Equal(t, "Season 2", season.Title, "provider title leaked onto the season in memory")
	stored, err := miRepo.GetByID(ctx, "season-2")
	require.NoError(t, err)
	assert.Equal(t, "Season 2", stored.Title, "provider title leaked onto the persisted season")
}

// A season the provider knows nothing about must not be touched either.
func TestAutoEnrichSeason_UnknownSeasonKeepsTitle(t *testing.T) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.SeasonDetails["500:1"] = &domain.MetadataResult{Title: "Gigantor", Overview: "First season"}

	miRepo := libfakes.NewFakeMediaItemStore()
	e := NewEnricher(metafakes.NewFakeArtworkRepository(), libfakes.NewFakePersonRepository(),
		libfakes.NewFakeMediaPersonRepository(), miRepo, miRepo, libfakes.NewFakeSearchIndex(), provider, nil, nil)

	ctx := context.Background()
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{
		ID:       "series-1",
		Title:    "Some Series",
		Overview: "Series overview",
		Type:     domainlib.Series,
		Catalog:  &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "500"}},
	}))
	seasonNum := 7
	season := &domainlib.MediaItem{
		ID:          "season-7",
		ParentID:    "series-1",
		Type:        domainlib.Season,
		Title:       "Season 7",
		IndexNumber: &seasonNum,
	}
	require.NoError(t, miRepo.Create(ctx, season))

	e.AutoEnrichItem(ctx, season, slog.Default())

	assert.Equal(t, "Season 7", season.Title)
	assert.Empty(t, season.Overview)
}

// Episodes and movies DO have a real title: the season guard must not reach them.
func TestApplyBasicFields_TitleByType(t *testing.T) {
	idx := 3
	tests := []struct {
		name     string
		itemType domainlib.MediaItemType
		current  string
		provided string
		want     string
	}{
		{"movie keeps provider title", domainlib.MovieItem, "some.movie.2019", "Some Movie", "Some Movie"},
		{"series keeps provider title", domainlib.Series, "some.series", "Some Series", "Some Series"},
		{"episode keeps provider title", domainlib.Episode, "s01e03", "Pilot", "Pilot"},
		{"season keeps its derived title", domainlib.Season, "Season 3", "Gigantor", "Season 3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := &domainlib.MediaItem{Type: tc.itemType, Title: tc.current, IndexNumber: &idx}
			applyBasicFields(item, &domain.MetadataResult{Title: tc.provided, Overview: "ov"})
			assert.Equal(t, tc.want, item.Title)
			assert.Equal(t, "ov", item.Overview)
		})
	}
}
