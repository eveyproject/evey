package metadata_test

import (
	"context"
	"sync"
	"testing"
	"time"

	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hangingByIDProvider answers episode lookups at once but hangs on a by-id
// lookup until the caller gives up, like an unreachable provider host.
type hangingByIDProvider struct {
	*metafakes.FakeMetadataProvider
	mu   sync.Mutex
	byID []string
}

func (p *hangingByIDProvider) GetByProviderID(ctx context.Context, id, _ string) (*domainmetadata.MetadataResult, error) {
	p.mu.Lock()
	p.byID = append(p.byID, id)
	p.mu.Unlock()
	<-ctx.Done()
	return nil, ctx.Err()
}

// Refreshing one episode is the gRPC RefreshMetadata call, which hands the
// episode's OWN stored TMDB id to RefreshItemMetadata. The by-id lookups only
// know movies and series, so that id was fetched as a series (/tv/{episode
// id}): an unrelated show, or every provider of the chain tried in turn until
// the request died. The episode is re-enriched through its series instead.
func TestRefreshItemMetadata_EpisodeGoesThroughItsSeries(t *testing.T) {
	provider := &hangingByIDProvider{FakeMetadataProvider: metafakes.NewFakeMetadataProvider("tmdb")}
	provider.EpisodeDetails["100:1:2"] = &domainmetadata.MetadataResult{
		Title: "The Right Episode", Overview: "what happens", ProviderIDs: map[string]string{"tmdb": "5555"},
	}

	mediaRepo := libfakes.NewFakeMediaItemStore()
	enricher := appmetadata.NewEnricher(metafakes.NewFakeArtworkRepository(), libfakes.NewFakePersonRepository(),
		libfakes.NewFakeMediaPersonRepository(), mediaRepo, mediaRepo, libfakes.NewFakeSearchIndex(), provider, nil, nil, nil)
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(), metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(), []domainmetadata.MetadataProvider{provider},
	)
	appmetadata.WithEnricher(enricher, mediaRepo, mediaRepo)(cmdSvc)

	ctx := context.Background()
	season, episode := 1, 2
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: shared.MediaItemID("s1"), Type: library.Series, Title: "Show", Overview: "a show",
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "100"}},
	}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: shared.MediaItemID("se1"), ParentID: "s1", Type: library.Season, Title: "Season 1", IndexNumber: &season,
	}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: shared.MediaItemID("e1"), ParentID: "se1", Type: library.Episode, Title: "Stale", IndexNumber: &episode,
		File:    &library.MediaFile{Path: "/media/Show/S01/Show S01E02.mkv"},
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "5555"}},
	}))

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	require.NoError(t, cmdSvc.RefreshItemMetadata(reqCtx, "e1", "5555", "tv"))
	assert.Less(t, time.Since(start), time.Second, "an episode refresh must not wait on a by-id lookup")

	got, err := mediaRepo.GetByID(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, "The Right Episode", got.Title)
	provider.mu.Lock()
	defer provider.mu.Unlock()
	assert.Empty(t, provider.byID, "the episode's own id must never be looked up as a series id")
}

// A refresh no provider can answer is an error, and nothing is announced as
// refreshed: the admin refresh of an episode answered OK and published
// MetadataRefreshed whatever happened.
func TestRefreshItemMetadata_AnEpisodeNoProviderHasIsAnError(t *testing.T) {
	provider := metafakes.NewFakeMetadataProvider("tmdb") // no episode details at all
	mediaRepo := libfakes.NewFakeMediaItemStore()
	enricher := appmetadata.NewEnricher(metafakes.NewFakeArtworkRepository(), libfakes.NewFakePersonRepository(),
		libfakes.NewFakeMediaPersonRepository(), mediaRepo, mediaRepo, libfakes.NewFakeSearchIndex(), provider, nil, nil, nil)
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(), metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(),
		bus, []domainmetadata.MetadataProvider{provider},
	)
	appmetadata.WithEnricher(enricher, mediaRepo, mediaRepo)(cmdSvc)

	ctx := context.Background()
	season, episode := 1, 2
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: shared.MediaItemID("s1"), Type: library.Series, Title: "Show", Overview: "a show",
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "100"}},
	}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: shared.MediaItemID("se1"), ParentID: "s1", Type: library.Season, Title: "Season 1", IndexNumber: &season,
	}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: shared.MediaItemID("e1"), ParentID: "se1", Type: library.Episode, Title: "Stale", IndexNumber: &episode,
	}))

	require.Error(t, cmdSvc.RefreshItemMetadata(ctx, "e1", "5555", "tv"))
	for _, evt := range bus.PublishedEvents() {
		assert.NotEqual(t, "metadata.refreshed", evt.EventName(), "a refresh that found nothing must not be announced")
	}
}
