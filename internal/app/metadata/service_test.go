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

func newTestService() (*appmetadata.Service, *metafakes.FakeMetadataOverrideRepository, *sharedfakes.FakeEventBus) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	svc := appmetadata.NewService(overrideRepo, artworkRepo, downloader, bus, []metadata.MetadataProvider{provider})
	return svc, overrideRepo, bus
}

func TestSearchMetadata(t *testing.T) {
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	downloader := metafakes.NewFakeArtworkDownloader()
	bus := sharedfakes.NewFakeEventBus()

	provider := metafakes.NewFakeMetadataProvider("tmdb")
	provider.SearchResults["matrix"] = []metadata.MetadataResult{
		{Title: "The Matrix", Overview: "A computer hacker..."},
	}

	svc := appmetadata.NewService(
		overrideRepo, artworkRepo, downloader, bus,
		[]metadata.MetadataProvider{provider},
	)

	results, err := svc.SearchMetadata(context.Background(), "matrix", "movie")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "The Matrix", results[0].Title)
}

func TestSetOverride(t *testing.T) {
	svc, _, bus := newTestService()
	ctx := context.Background()

	override := &metadata.MetadataOverride{
		ID:          "mo-1",
		MediaItemID: "item-1",
		FieldName:   "title",
		CustomValue: "Custom Title",
	}
	err := svc.SetOverride(ctx, override)
	require.NoError(t, err)
	assert.NotZero(t, override.CreatedAt)

	overrides, err := svc.ListOverrides(ctx, "item-1")
	require.NoError(t, err)
	assert.Len(t, overrides, 1)
	assert.Equal(t, "title", overrides[0].FieldName)

	assert.Len(t, bus.PublishedEvents(), 1)
	assert.Equal(t, "metadata.override.set", bus.PublishedEvents()[0].EventName())
}

func TestListOverrides_Empty(t *testing.T) {
	svc, _, _ := newTestService()
	overrides, err := svc.ListOverrides(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, overrides)
}

func TestRemoveOverride(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	override := &metadata.MetadataOverride{
		ID:          "mo-1",
		MediaItemID: "item-1",
		FieldName:   "title",
		CustomValue: "Custom Title",
	}
	_ = svc.SetOverride(ctx, override)

	err := svc.RemoveOverride(ctx, "mo-1")
	require.NoError(t, err)

	overrides, err := svc.ListOverrides(ctx, "item-1")
	require.NoError(t, err)
	assert.Empty(t, overrides)
}

func TestResetOverrides(t *testing.T) {
	svc, _, bus := newTestService()
	ctx := context.Background()

	_ = svc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-1", MediaItemID: "item-1", FieldName: "title", CustomValue: "T",
	})
	_ = svc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-2", MediaItemID: "item-1", FieldName: "overview", CustomValue: "O",
	})
	_ = svc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-3", MediaItemID: "item-2", FieldName: "title", CustomValue: "Other",
	})

	countBefore := len(bus.PublishedEvents())
	err := svc.ResetOverrides(ctx, "item-1")
	require.NoError(t, err)

	overrides, _ := svc.ListOverrides(ctx, "item-1")
	assert.Empty(t, overrides)

	other, _ := svc.ListOverrides(ctx, "item-2")
	assert.Len(t, other, 1)

	eventsAfter := bus.PublishedEvents()[countBefore:]
	assert.Len(t, eventsAfter, 1)
	assert.Equal(t, "metadata.override.removed", eventsAfter[0].EventName())
}

func TestSetOverride_UpdatesExisting(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()

	_ = svc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-1", MediaItemID: "item-1", FieldName: "title", CustomValue: "V1",
	})
	_ = svc.SetOverride(ctx, &metadata.MetadataOverride{
		ID: "mo-2", MediaItemID: "item-1", FieldName: "title", CustomValue: "V2",
	})

	overrides, _ := svc.ListOverrides(ctx, "item-1")
	assert.Len(t, overrides, 2)
}
