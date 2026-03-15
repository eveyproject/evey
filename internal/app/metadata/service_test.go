package metadata_test

import (
	"context"
	"testing"

	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	metafakes "github.com/gravity-zero/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServices() (*appmetadata.CommandService, *appmetadata.QueryService, *metafakes.FakeMetadataOverrideRepository, *sharedfakes.FakeEventBus) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	cmdSvc := appmetadata.NewCommandService(overrideRepo, artworkRepo, downloader, bus, []metadata.MetadataProvider{provider})
	querySvc := appmetadata.NewQueryService(overrideRepo, artworkRepo, []metadata.MetadataProvider{provider})
	return cmdSvc, querySvc, overrideRepo, bus
}

func TestSearchMetadata(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()

	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.SearchResults["matrix"] = []metadata.MetadataResult{
		{Title: "The Matrix", Overview: "A computer hacker..."},
	}

	svc := appmetadata.NewQueryService(
		overrideRepo, artworkRepo,
		[]metadata.MetadataProvider{provider},
	)

	results, err := svc.SearchMetadata(context.Background(), "matrix", "movie")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "The Matrix", results[0].Title)
}

func TestSetOverride(t *testing.T) {
	cmdSvc, querySvc, _, bus := newTestServices()
	ctx := context.Background()

	override := &metadata.MetadataOverride{
		ID:          "mo-1",
		MediaItemID: "item-1",
		FieldName:   "title",
		CustomValue: "Custom Title",
	}
	err := cmdSvc.SetOverride(ctx, override)
	require.NoError(t, err)
	assert.NotZero(t, override.CreatedAt)

	overrides, err := querySvc.ListOverrides(ctx, "item-1")
	require.NoError(t, err)
	assert.Len(t, overrides, 1)
	assert.Equal(t, "title", overrides[0].FieldName)

	assert.Len(t, bus.PublishedEvents(), 1)
	assert.Equal(t, "metadata.override.set", bus.PublishedEvents()[0].EventName())
}

func TestListOverrides_Empty(t *testing.T) {
	_, querySvc, _, _ := newTestServices()
	overrides, err := querySvc.ListOverrides(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, overrides)
}

func TestRemoveOverride(t *testing.T) {
	cmdSvc, querySvc, _, _ := newTestServices()
	ctx := context.Background()

	override := &metadata.MetadataOverride{
		ID:          "mo-1",
		MediaItemID: "item-1",
		FieldName:   "title",
		CustomValue: "Custom Title",
	}
	_ = cmdSvc.SetOverride(ctx, override)

	err := cmdSvc.RemoveOverride(ctx, "mo-1")
	require.NoError(t, err)

	overrides, err := querySvc.ListOverrides(ctx, "item-1")
	require.NoError(t, err)
	assert.Empty(t, overrides)
}

func TestResetOverrides(t *testing.T) {
	cmdSvc, querySvc, _, bus := newTestServices()
	ctx := context.Background()

	_ = cmdSvc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-1", MediaItemID: "item-1", FieldName: "title", CustomValue: "T",
	})
	_ = cmdSvc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-2", MediaItemID: "item-1", FieldName: "overview", CustomValue: "O",
	})
	_ = cmdSvc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-3", MediaItemID: "item-2", FieldName: "title", CustomValue: "Other",
	})

	countBefore := len(bus.PublishedEvents())
	err := cmdSvc.ResetOverrides(ctx, "item-1")
	require.NoError(t, err)

	overrides, _ := querySvc.ListOverrides(ctx, "item-1")
	assert.Empty(t, overrides)

	other, _ := querySvc.ListOverrides(ctx, "item-2")
	assert.Len(t, other, 1)

	eventsAfter := bus.PublishedEvents()[countBefore:]
	assert.Len(t, eventsAfter, 1)
	assert.Equal(t, "metadata.override.removed", eventsAfter[0].EventName())
}

func TestSetOverride_UpdatesExisting(t *testing.T) {
	cmdSvc, querySvc, _, _ := newTestServices()
	ctx := context.Background()

	_ = cmdSvc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-1", MediaItemID: "item-1", FieldName: "title", CustomValue: "V1",
	})
	_ = cmdSvc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-2", MediaItemID: "item-1", FieldName: "title", CustomValue: "V2",
	})

	overrides, _ := querySvc.ListOverrides(ctx, "item-1")
	assert.Len(t, overrides, 2)
}

func TestGetArtwork_Empty(t *testing.T) {
	_, querySvc, _, _ := newTestServices()
	records, err := querySvc.GetArtwork(context.Background(), "nonexistent-item")
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestGetArtwork_ReturnsRecords(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	querySvc := appmetadata.NewQueryService(metafakes.NewFakeMetadataOverrideRepository(), artworkRepo, nil)
	ctx := context.Background()

	url := "http://example.com/poster.jpg"
	require.NoError(t, artworkRepo.Create(ctx, &metadata.ArtworkRecord{
		ID: "art-1", MediaItemID: "item-1", Type: metadata.ArtworkTypePoster, SourceURL: &url,
	}))
	require.NoError(t, artworkRepo.Create(ctx, &metadata.ArtworkRecord{
		ID: "art-2", MediaItemID: "item-1", Type: metadata.ArtworkTypeBackdrop, SourceURL: &url,
	}))
	require.NoError(t, artworkRepo.Create(ctx, &metadata.ArtworkRecord{
		ID: "art-3", MediaItemID: "item-2", Type: metadata.ArtworkTypePoster, SourceURL: &url,
	}))

	records, err := querySvc.GetArtwork(ctx, "item-1")
	require.NoError(t, err)
	assert.Len(t, records, 2)
}

func TestListProviders(t *testing.T) {
	p1 := metafakes.NewFakeMetadataProvider("tmdb")
	p2 := metafakes.NewFakeMetadataProvider("imdbapi")
	querySvc := appmetadata.NewQueryService(metafakes.NewFakeMetadataOverrideRepository(), metafakes.NewFakeArtworkRepository(), []metadata.MetadataProvider{p1, p2})

	names := querySvc.ListProviders()
	assert.ElementsMatch(t, []string{"tmdb", "imdbapi"}, names)
}

func TestRefreshMetadata_ProviderNotFound(t *testing.T) {
	cmdSvc, _, _, _ := newTestServices()
	_, err := cmdSvc.RefreshMetadata(context.Background(), "item-1", "tt123", "nonexistent", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider nonexistent not found")
}

func TestRefreshMetadata_ProviderReturnsResult(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.ByProviderID["tmdb-42"] = &metadata.MetadataResult{Title: "Interstellar"}
	cmdSvc := appmetadata.NewCommandService(overrideRepo, artworkRepo, downloader, bus, []metadata.MetadataProvider{provider})

	result, err := cmdSvc.RefreshMetadata(context.Background(), "item-1", "tmdb-42", "tmdb", "movie")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Interstellar", result.Title)
	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "metadata.refreshed", events[0].EventName())
}

func TestSearchMetadata_NoProviders(t *testing.T) {
	querySvc := appmetadata.NewQueryService(metafakes.NewFakeMetadataOverrideRepository(), metafakes.NewFakeArtworkRepository(), nil)

	results, err := querySvc.SearchMetadata(context.Background(), "matrix", "movie")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestDownloadArtwork_NoSourceURL(t *testing.T) {
	cmdSvc, _, _, _ := newTestServices()
	record := &metadata.ArtworkRecord{ID: "art-1", MediaItemID: "item-1", Type: metadata.ArtworkTypePoster}
	err := cmdSvc.DownloadArtwork(context.Background(), record, "/tmp/poster.jpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no source URL")
}

func TestDownloadArtwork_Success(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, artworkRepo, downloader, bus, nil)

	url := "http://example.com/art.jpg"
	record := &metadata.ArtworkRecord{ID: "art-1", MediaItemID: "item-1", Type: metadata.ArtworkTypePoster, SourceURL: &url}
	err := cmdSvc.DownloadArtwork(context.Background(), record, "/tmp/art.jpg")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/art.jpg", *record.LocalPath)
	assert.Equal(t, "/tmp/art.jpg", downloader.Downloads[url])
	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "metadata.artwork.downloaded", events[0].EventName())
}
