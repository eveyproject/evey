package metadata

import (
	"context"
	"log/slog"
	"testing"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	domain "github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A series re-identified to another provider id: its season and episodes are
// re-fetched under the NEW id and overwrite what the old match wrote, even
// though they already had an overview (the bulk pass would skip them).
func TestRefreshSeriesChildren_FollowsTheSeriesNewIdentity(t *testing.T) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.SeasonDetails["6560:1"] = &domain.MetadataResult{Overview: "1954 season one", ProviderIDs: map[string]string{"tmdb": "s-1954"}}
	provider.EpisodeDetails["6560:1:1"] = &domain.MetadataResult{Title: "The Case of the Cunningham Heritage", Overview: "1954 pilot", ProviderIDs: map[string]string{"tmdb": "e-1954-1"}}
	provider.EpisodeDetails["6560:1:2"] = &domain.MetadataResult{Title: "The Case of Lady Beryl", Overview: "1954 second", ProviderIDs: map[string]string{"tmdb": "e-1954-2"}}

	miRepo := libfakes.NewFakeMediaItemStore()
	e := NewEnricher(metafakes.NewFakeArtworkRepository(), libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		miRepo, miRepo, libfakes.NewFakeSearchIndex(), provider, nil, nil, nil)
	ctx := context.Background()

	// The series already carries its new id (RefreshItemMetadata wrote it);
	// the children still carry the 1984 match.
	series := &domainlib.MediaItem{ID: "series-1", Type: domainlib.Series, Title: "Sherlock Holmes", Overview: "1954 series",
		Catalog: &domainlib.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "6560"}}}
	require.NoError(t, miRepo.Create(ctx, series))
	one, two := 1, 2
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "season-1", ParentID: "series-1", Type: domainlib.Season, Title: "Season 1", IndexNumber: &one, Overview: "1984 season"}))
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "ep-1", ParentID: "season-1", Type: domainlib.Episode, Title: "A Scandal in Bohemia", IndexNumber: &one, Overview: "1984 episode"}))
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "ep-2", ParentID: "season-1", Type: domainlib.Episode, Title: "The Dancing Men", IndexNumber: &two, Overview: "1984 episode"}))
	// An episode of another series must not be touched.
	require.NoError(t, miRepo.Create(ctx, &domainlib.MediaItem{ID: "other-ep", ParentID: "other-season", Type: domainlib.Episode, Title: "Elsewhere", IndexNumber: &one, Overview: "untouched"}))

	require.NoError(t, e.RefreshSeriesChildren(ctx, series, slog.Default()))

	season, err := miRepo.GetByID(ctx, "season-1")
	require.NoError(t, err)
	assert.Equal(t, "1954 season one", season.Overview)
	assert.Equal(t, "Season 1", season.Title, "a season keeps its index label")
	ep1, err := miRepo.GetByID(ctx, "ep-1")
	require.NoError(t, err)
	assert.Equal(t, "The Case of the Cunningham Heritage", ep1.Title)
	assert.Equal(t, "e-1954-1", ep1.Catalog.ProviderIDs["tmdb"])
	ep2, err := miRepo.GetByID(ctx, "ep-2")
	require.NoError(t, err)
	assert.Equal(t, "The Case of Lady Beryl", ep2.Title)
	other, err := miRepo.GetByID(ctx, "other-ep")
	require.NoError(t, err)
	assert.Equal(t, "Elsewhere", other.Title)

	// Not a series: nothing listed, nothing touched.
	require.NoError(t, e.RefreshSeriesChildren(ctx, &domainlib.MediaItem{ID: "film", Type: domainlib.MovieItem}, slog.Default()))
}
