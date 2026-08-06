package metadata_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
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
	provider.SearchResults["some movie"] = []metadata.MetadataResult{
		{Title: "Some Movie", Overview: "A computer hacker..."},
	}

	svc := appmetadata.NewQueryService(
		overrideRepo, artworkRepo,
		[]metadata.MetadataProvider{provider},
	)

	results, err := svc.SearchMetadata(context.Background(), "some movie", "movie")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Some Movie", results[0].Title)
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
	provider.ByProviderID["tmdb-42"] = &metadata.MetadataResult{Title: "Some Movie"}
	cmdSvc := appmetadata.NewCommandService(overrideRepo, artworkRepo, downloader, bus, []metadata.MetadataProvider{provider})

	result, err := cmdSvc.RefreshMetadata(context.Background(), "item-1", "tmdb-42", "tmdb", "movie")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Some Movie", result.Title)
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

// fakeBulkEnricher records calls for testing.
type fakeBulkEnricher struct {
	enrichedItems []*library.MediaItem
}

func (f *fakeBulkEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}

func (f *fakeBulkEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}

func (f *fakeBulkEnricher) AutoEnrichAll(_ context.Context, items []*library.MediaItem, _ *slog.Logger) {
	f.enrichedItems = append(f.enrichedItems, items...)
}

func TestReEnrichWithoutOverrides_SkipsOverriddenItems(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, artworkRepo, downloader, bus, nil)

	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	ctx := context.Background()

	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Movie A"}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-2", Title: "Movie B"}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-3", Title: "Movie C"}))

	require.NoError(t, overrideRepo.Set(ctx, &metadata.MetadataOverride{
		ID: "ov-1", MediaItemID: "item-2", FieldName: "title", CustomValue: "Custom",
	}))

	refreshed, skipped := cmdSvc.ReEnrichWithoutOverrides(ctx, enricher)

	assert.Equal(t, 2, refreshed)
	assert.Equal(t, 1, skipped)
	assert.Len(t, enricher.enrichedItems, 2)
	ids := make(map[string]bool)
	for _, it := range enricher.enrichedItems {
		ids[string(it.ID)] = true
	}
	assert.True(t, ids["item-1"])
	assert.True(t, ids["item-3"])
	assert.False(t, ids["item-2"])
}

func TestReEnrichWithoutOverrides_AllOverridden(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(), bus, nil)

	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()

	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))
	require.NoError(t, overrideRepo.Set(ctx, &metadata.MetadataOverride{
		ID: "ov-1", MediaItemID: "item-1", FieldName: "title", CustomValue: "X",
	}))

	refreshed, skipped := cmdSvc.ReEnrichWithoutOverrides(ctx, enricher)
	assert.Equal(t, 0, refreshed)
	assert.Equal(t, 1, skipped)
	assert.Empty(t, enricher.enrichedItems)
}

func TestReEnrichWithoutOverrides_NoItems(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(), bus, nil)

	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	refreshed, skipped := cmdSvc.ReEnrichWithoutOverrides(context.Background(), enricher)
	assert.Equal(t, 0, refreshed)
	assert.Equal(t, 0, skipped)
}

// --- RefreshItemMetadata ---

type fakeItemEnricher struct {
	enrichedPairs []enrichPair
	err           error
}
type enrichPair struct {
	item   *library.MediaItem
	result *metadata.MetadataResult
}

func (f *fakeItemEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}

func (f *fakeItemEnricher) EnrichItem(_ context.Context, item *library.MediaItem, result *metadata.MetadataResult) error {
	if f.err != nil {
		return f.err
	}
	f.enrichedPairs = append(f.enrichedPairs, enrichPair{item: item, result: result})
	return nil
}

func TestRefreshItemMetadata_NoEnricher(t *testing.T) {
	cmdSvc, _, _, _ := newTestServices()
	err := cmdSvc.RefreshItemMetadata(context.Background(), "item-1", "123", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enricher not configured")
}

func TestRefreshItemMetadata_ItemNotFound(t *testing.T) {
	cmdSvc, _, _, _ := newTestServices()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeItemEnricher{}, mediaRepo)(cmdSvc)

	err := cmdSvc.RefreshItemMetadata(context.Background(), "missing", "123", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media item")
}

func TestRefreshItemMetadata_NoProviderResult(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus,
		[]metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeItemEnricher{}, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "X"}))

	err := cmdSvc.RefreshItemMetadata(ctx, "item-1", "unknown-tmdb", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no provider returned metadata")
}

func TestRefreshItemMetadata_Success(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.ByProviderID["tmdb-42"] = &metadata.MetadataResult{Title: "Some Movie"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus,
		[]metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeItemEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	err := cmdSvc.RefreshItemMetadata(ctx, "item-1", "tmdb-42", "movie")
	require.NoError(t, err)
	require.Len(t, enricher.enrichedPairs, 1)
	assert.Equal(t, "Some Movie", enricher.enrichedPairs[0].result.Title)
	assert.Equal(t, shared.MediaItemID("item-1"), enricher.enrichedPairs[0].item.ID)
	require.Len(t, bus.PublishedEvents(), 1)
	assert.Equal(t, "metadata.refreshed", bus.PublishedEvents()[0].EventName())
}

func TestRefreshItemMetadata_EnrichError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.ByProviderID["tmdb-42"] = &metadata.MetadataResult{Title: "X"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus,
		[]metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeItemEnricher{err: errors.New("enrich boom")}, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	err := cmdSvc.RefreshItemMetadata(ctx, "item-1", "tmdb-42", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrich item")
}

// --- RefreshItemMetadataByIMDB ---

func TestRefreshItemMetadataByIMDB_NoEnricher(t *testing.T) {
	cmdSvc, _, _, _ := newTestServices()
	err := cmdSvc.RefreshItemMetadataByIMDB(context.Background(), "item-1", "tt123", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enricher not configured")
}

func TestRefreshItemMetadataByIMDB_Success_PreferredProvider(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	imdbProvider := metafakes.NewFakeMetadataProvider("imdbapi")
	imdbProvider.ByProviderID["tt999"] = &metadata.MetadataResult{Title: "IMDB Movie"}
	tmdbProvider := metafakes.NewFakeMetadataProvider("tmdb")
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus,
		[]metadata.MetadataProvider{tmdbProvider, imdbProvider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeItemEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	err := cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt999", "movie")
	require.NoError(t, err)
	require.Len(t, enricher.enrichedPairs, 1)
	assert.Equal(t, "IMDB Movie", enricher.enrichedPairs[0].result.Title)
	assert.Equal(t, "tt999", enricher.enrichedPairs[0].result.ProviderIDs["imdb"])
}

func TestRefreshItemMetadataByIMDB_FallbackToOtherProvider(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	imdbProvider := metafakes.NewFakeMetadataProvider("imdbapi")
	// imdbapi has no result for tt999 -> will fall back to tmdb
	tmdbProvider := metafakes.NewFakeMetadataProvider("tmdb")
	tmdbProvider.ByProviderID["tt999"] = &metadata.MetadataResult{Title: "TMDB Fallback"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus,
		[]metadata.MetadataProvider{tmdbProvider, imdbProvider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeItemEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	err := cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt999", "movie")
	require.NoError(t, err)
	require.Len(t, enricher.enrichedPairs, 1)
	assert.Equal(t, "TMDB Fallback", enricher.enrichedPairs[0].result.Title)
	assert.Equal(t, "tt999", enricher.enrichedPairs[0].result.ProviderIDs["imdb"])
}

func TestRefreshItemMetadataByIMDB_NilProviderIDs(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("imdbapi")
	provider.ByProviderID["tt111"] = &metadata.MetadataResult{Title: "No IDs", ProviderIDs: nil}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus,
		[]metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeItemEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	err := cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt111", "movie")
	require.NoError(t, err)
	require.Len(t, enricher.enrichedPairs, 1)
	assert.Equal(t, "tt111", enricher.enrichedPairs[0].result.ProviderIDs["imdb"])
}

func TestRefreshItemMetadataByIMDB_NoProviderResult(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus,
		[]metadata.MetadataProvider{metafakes.NewFakeMetadataProvider("tmdb")},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeItemEnricher{}, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	err := cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt404", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no provider returned metadata")
}

// --- RefreshMetadata error path (provider returns error) ---

func TestRefreshMetadata_ProviderReturnsError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.Err = errors.New("api down")
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus,
		[]metadata.MetadataProvider{provider},
	)

	_, err := cmdSvc.RefreshMetadata(context.Background(), "item-1", "tmdb-42", "tmdb", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get metadata from tmdb")
}

// --- SetOverride error path ---

func TestSetOverride_RepoError(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	overrideRepo.Err = errors.New("db down")
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(), bus, nil)

	err := cmdSvc.SetOverride(context.Background(), &metadata.MetadataOverride{ID: "o1", MediaItemID: "i1", FieldName: "title", CustomValue: "X"})
	require.Error(t, err)
	assert.Empty(t, bus.PublishedEvents())
}

// --- ResetOverrides error path ---

func TestResetOverrides_RepoError(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	overrideRepo.Err = errors.New("db down")
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(), bus, nil)

	err := cmdSvc.ResetOverrides(context.Background(), "item-1")
	require.Error(t, err)
	assert.Empty(t, bus.PublishedEvents())
}

// --- DownloadArtwork error path ---

func TestDownloadArtwork_DownloaderError(t *testing.T) {
	downloader := metafakes.NewFakeArtworkDownloader()
	downloader.Err = errors.New("network error")
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(metafakes.NewFakeMetadataOverrideRepository(), metafakes.NewFakeArtworkRepository(), downloader, bus, nil)

	url := "http://example.com/art.jpg"
	record := &metadata.ArtworkRecord{ID: "art-1", MediaItemID: "item-1", Type: metadata.ArtworkTypePoster, SourceURL: &url}
	err := cmdSvc.DownloadArtwork(context.Background(), record, "/tmp/art.jpg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download artwork")
	assert.Nil(t, record.LocalPath)
}

// --- AutoRefreshAllItems ---

func TestAutoRefreshAllItems_NoEnricher(t *testing.T) {
	cmdSvc, _, _, _ := newTestServices()
	err := cmdSvc.AutoRefreshAllItems(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enricher does not support bulk refresh")
}

func TestAutoRefreshAllItems_Success(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()

	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-2", Title: "B"}))

	err := cmdSvc.AutoRefreshAllItems(ctx, "")
	require.NoError(t, err)
	assert.Len(t, enricher.enrichedItems, 2)
}

func TestAutoRefreshAllItems_EmptyRepo(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	err := cmdSvc.AutoRefreshAllItems(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, enricher.enrichedItems)
}

func TestAutoRefreshAllItems_ListError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	mediaRepo.ErrList = errors.New("db down")
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	err := cmdSvc.AutoRefreshAllItems(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list items")
}

// --- BatchGetArtwork ---

func TestBatchGetArtwork(t *testing.T) {
	artworkRepo := metafakes.NewFakeArtworkRepository()
	querySvc := appmetadata.NewQueryService(metafakes.NewFakeMetadataOverrideRepository(), artworkRepo, nil)
	ctx := context.Background()

	url := "http://example.com/poster.jpg"
	require.NoError(t, artworkRepo.Create(ctx, &metadata.ArtworkRecord{ID: "art-1", MediaItemID: "item-1", Type: metadata.ArtworkTypePoster, SourceURL: &url}))
	require.NoError(t, artworkRepo.Create(ctx, &metadata.ArtworkRecord{ID: "art-2", MediaItemID: "item-2", Type: metadata.ArtworkTypeBackdrop, SourceURL: &url}))

	result, err := querySvc.BatchGetArtwork(ctx, []string{"item-1", "item-2", "item-3"})
	require.NoError(t, err)
	assert.Len(t, result["item-1"], 1)
	assert.Len(t, result["item-2"], 1)
	assert.Empty(t, result["item-3"])
}

func TestBatchGetArtwork_Empty(t *testing.T) {
	querySvc := appmetadata.NewQueryService(metafakes.NewFakeMetadataOverrideRepository(), metafakes.NewFakeArtworkRepository(), nil)
	result, err := querySvc.BatchGetArtwork(context.Background(), []string{"item-1"})
	require.NoError(t, err)
	assert.Empty(t, result)
}

// --- SearchMetadata with provider error ---

// --- StartEnrichmentWorker + WaitWorkers ---

type fakeBulkSingleEnricher struct {
	mu            sync.Mutex
	enrichedItems []*library.MediaItem
	autoItems     []*library.MediaItem
}

func (f *fakeBulkSingleEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *metadata.MetadataResult) error {
	return nil
}
func (f *fakeBulkSingleEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}

func (f *fakeBulkSingleEnricher) AutoEnrichAll(_ context.Context, items []*library.MediaItem, _ *slog.Logger) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enrichedItems = append(f.enrichedItems, items...)
}

func (f *fakeBulkSingleEnricher) AutoEnrichItem(_ context.Context, item *library.MediaItem, _ *slog.Logger) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.autoItems = append(f.autoItems, item)
}

// ForceAutoEnrichItem is a no-op stub: this fake is only wired for
// StartEnrichmentWorker's bulk/single (non-force) paths, but the merged
// SingleItemEnricher interface requires the method to exist.
func (f *fakeBulkSingleEnricher) ForceAutoEnrichItem(_ context.Context, _ *library.MediaItem, _ *slog.Logger) {
}

func TestStartEnrichmentWorker_BulkEnrich(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeBulkSingleEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx, cancel := context.WithCancel(context.Background())

	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-2", Title: "B"}))

	cmdSvc.StartEnrichmentWorker(ctx)
	cancel()
	cmdSvc.WaitWorkers()

	enricher.mu.Lock()
	defer enricher.mu.Unlock()
	assert.Len(t, enricher.enrichedItems, 2)
}

func TestStartEnrichmentWorker_SingleEnrich_ViaEvent(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeBulkSingleEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx, cancel := context.WithCancel(context.Background())

	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-99", Title: "New Movie"}))

	cmdSvc.StartEnrichmentWorker(ctx)

	evt := library.MediaItemAdded{MediaItemID: "item-99"}
	_ = bus.Publish(ctx, evt)

	// give worker time to process
	time.Sleep(50 * time.Millisecond)
	cancel()
	cmdSvc.WaitWorkers()

	enricher.mu.Lock()
	defer enricher.mu.Unlock()
	assert.Len(t, enricher.autoItems, 1)
	assert.Equal(t, shared.MediaItemID("item-99"), enricher.autoItems[0].ID)
}

func TestStartEnrichmentWorker_NoEnricher(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	// no enricher configured - should return immediately
	cmdSvc.StartEnrichmentWorker(context.Background())
	cmdSvc.WaitWorkers()
}

// --- event publish error paths (log-only, no failure) ---

func TestRefreshMetadata_EventPublishError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.ByProviderID["tmdb-1"] = &metadata.MetadataResult{Title: "X"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{provider},
	)
	result, err := cmdSvc.RefreshMetadata(context.Background(), "item-1", "tmdb-1", "tmdb", "movie")
	require.NoError(t, err)
	assert.Equal(t, "X", result.Title)
}

func TestSetOverride_EventPublishError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	err := cmdSvc.SetOverride(context.Background(), &metadata.MetadataOverride{
		ID: "o1", MediaItemID: "i1", FieldName: "title", CustomValue: "X",
	})
	require.NoError(t, err)
}

func TestResetOverrides_EventPublishError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	ctx := context.Background()
	_ = cmdSvc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "o1", MediaItemID: "i1", FieldName: "title", CustomValue: "X",
	})
	bus.PublishErr = errors.New("bus down")
	err := cmdSvc.ResetOverrides(ctx, "i1")
	require.NoError(t, err)
}

func TestDownloadArtwork_EventPublishError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	url := "http://example.com/art.jpg"
	record := &metadata.ArtworkRecord{ID: "a1", MediaItemID: "i1", Type: metadata.ArtworkTypePoster, SourceURL: &url}
	err := cmdSvc.DownloadArtwork(context.Background(), record, "/tmp/art.jpg")
	require.NoError(t, err)
	assert.Equal(t, "/tmp/art.jpg", *record.LocalPath)
}

func TestRefreshItemMetadata_EventPublishError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.ByProviderID["tmdb-1"] = &metadata.MetadataResult{Title: "X"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeItemEnricher{}, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))
	err := cmdSvc.RefreshItemMetadata(ctx, "item-1", "tmdb-1", "movie")
	require.NoError(t, err)
}

func TestRefreshItemMetadataByIMDB_EventPublishError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	bus.PublishErr = errors.New("bus down")
	provider := metafakes.NewFakeMetadataProvider("imdbapi")
	provider.ByProviderID["tt1"] = &metadata.MetadataResult{Title: "X"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeItemEnricher{}, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))
	err := cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt1", "movie")
	require.NoError(t, err)
}

func TestRefreshItemMetadataByIMDB_ItemNotFound(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{metafakes.NewFakeMetadataProvider("tmdb")},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeItemEnricher{}, mediaRepo)(cmdSvc)
	err := cmdSvc.RefreshItemMetadataByIMDB(context.Background(), "missing", "tt1", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media item")
}

func TestRefreshItemMetadataByIMDB_EnrichError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("imdbapi")
	provider.ByProviderID["tt1"] = &metadata.MetadataResult{Title: "X"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{provider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	appmetadata.WithEnricher(&fakeItemEnricher{err: errors.New("enrich boom")}, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))
	err := cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt1", "movie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrich item")
}

// --- ReEnrichWithoutOverrides error paths ---

func TestReEnrichWithoutOverrides_ListError(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, nil,
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	mediaRepo.ErrList = errors.New("db down")
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)

	refreshed, skipped := cmdSvc.ReEnrichWithoutOverrides(context.Background(), enricher)
	assert.Equal(t, 0, refreshed)
	assert.Equal(t, 0, skipped)
}

func TestReEnrichWithoutOverrides_OverrideCheckError(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	bus := sharedfakes.NewFakeEventBus()
	cmdSvc := appmetadata.NewCommandService(overrideRepo, metafakes.NewFakeArtworkRepository(), metafakes.NewFakeArtworkDownloader(), bus, nil)

	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeBulkEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "A"}))

	overrideRepo.Err = errors.New("db down")

	refreshed, skipped := cmdSvc.ReEnrichWithoutOverrides(ctx, enricher)
	assert.Equal(t, 0, refreshed)
	assert.Equal(t, 0, skipped)
	assert.Empty(t, enricher.enrichedItems)
}

// --- fetchFromProviders: preferred provider not in list ---

func TestFetchFromProviders_PreferredNotInList(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	tmdbProvider := metafakes.NewFakeMetadataProvider("tmdb")
	tmdbProvider.ByProviderID["tt1"] = &metadata.MetadataResult{Title: "TMDB Result"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{tmdbProvider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeItemEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	// imdbapi preferred but not in provider list; should fallback to tmdb
	err := cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt1", "movie")
	require.NoError(t, err)
	require.Len(t, enricher.enrichedPairs, 1)
	assert.Equal(t, "TMDB Result", enricher.enrichedPairs[0].result.Title)
}

// fetchFromProviders: preferred provider found but returns error -> break and fallback
func TestFetchFromProviders_PreferredErrorFallback(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	imdbProvider := metafakes.NewFakeMetadataProvider("imdbapi")
	imdbProvider.Err = errors.New("api down") // preferred but errors
	tmdbProvider := metafakes.NewFakeMetadataProvider("tmdb")
	tmdbProvider.ByProviderID["tt1"] = &metadata.MetadataResult{Title: "TMDB Fallback"}
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		bus, []metadata.MetadataProvider{tmdbProvider, imdbProvider},
	)
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	enricher := &fakeItemEnricher{}
	appmetadata.WithEnricher(enricher, mediaRepo)(cmdSvc)
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "item-1", Title: "Old"}))

	err := cmdSvc.RefreshItemMetadataByIMDB(ctx, "item-1", "tt1", "movie")
	require.NoError(t, err)
	require.Len(t, enricher.enrichedPairs, 1)
	assert.Equal(t, "TMDB Fallback", enricher.enrichedPairs[0].result.Title)
}

func TestSearchMetadata_ProviderErrorSkipped(t *testing.T) {
	errProvider := metafakes.NewFakeMetadataProvider("broken")
	errProvider.Err = errors.New("api error")
	okProvider := metafakes.NewFakeMetadataProvider("tmdb")
	okProvider.SearchResults["test"] = []metadata.MetadataResult{{Title: "OK Result"}}

	querySvc := appmetadata.NewQueryService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		[]metadata.MetadataProvider{errProvider, okProvider},
	)

	results, err := querySvc.SearchMetadata(context.Background(), "test", "movie")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "OK Result", results[0].Title)
}

func TestBatchListOverrides(t *testing.T) {
	cmdSvc, querySvc, _, _ := newTestServices()
	ctx := context.Background()
	_ = cmdSvc.SetOverride(ctx, &metadata.MetadataOverride{ID: "mo-1", MediaItemID: "item-1", FieldName: "title", CustomValue: "Fixed"})
	_ = cmdSvc.SetOverride(ctx, &metadata.MetadataOverride{ID: "mo-2", MediaItemID: "item-1", FieldName: "overview", CustomValue: "ov"})

	out, err := querySvc.BatchListOverrides(ctx, []string{"item-1", "item-2", "item-3"})
	require.NoError(t, err)
	// only item-1 has overrides; item-2/3 absent from the map
	require.Len(t, out["item-1"], 2)
	_, has2 := out["item-2"]
	assert.False(t, has2)
}

func TestBatchListOverrides_Empty(t *testing.T) {
	_, querySvc, _, _ := newTestServices()
	out, err := querySvc.BatchListOverrides(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, out)
}
