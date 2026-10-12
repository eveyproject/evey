package grpcapi

import (
	"context"
	"log/slog"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	metafakes "github.com/eveyproject/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// orphanBulkEnricher is a minimal BulkItemEnricher (+ ItemEnricher) that does
// nothing - sufficient to let MatchOrphansInLibrary pass the type-assertion
// guard and return the queued count without triggering real provider calls.
type orphanBulkEnricher struct{}

func (orphanBulkEnricher) EnrichItem(_ context.Context, _ *library.MediaItem, _ *domainmetadata.MetadataResult) error {
	return nil
}
func (orphanBulkEnricher) EnrichTranslationsForItem(_ context.Context, _ *library.MediaItem, _, _ string) {
}
func (orphanBulkEnricher) AutoEnrichAll(_ context.Context, _ []*library.MediaItem, _ *slog.Logger) {}

// newOrphansHandler wires a metadataCmd with a bulk enricher and returns it
// together with the underlying media-item repo for test setup.
func newOrphansHandler() (*adminMediaHandler, *libfakes.FakeMediaItemStore) {
	mediaRepo := libfakes.NewFakeMediaItemStore()
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	events := sharedfakes.NewFakeEventBus()
	providers := []domainmetadata.MetadataProvider{metafakes.NewFakeMetadataProvider("tmdb")}

	cmd := appmetadata.NewCommandService(
		overrideRepo, artworkRepo,
		metafakes.NewFakeArtworkDownloader(),
		events, providers,
		appmetadata.WithEnricher(orphanBulkEnricher{}, mediaRepo, mediaRepo),
	)
	h := &adminMediaHandler{metadataCmd: cmd}
	return h, mediaRepo
}

func TestMatchOrphansInLibrary_NilService(t *testing.T) {
	h := &adminMediaHandler{}
	_, err := h.MatchOrphansInLibrary(context.Background(), &eveyv1.MatchOrphansInLibraryRequest{LibraryId: "lib-1"})
	require.Error(t, err)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestMatchOrphansInLibrary_ZeroOrphans(t *testing.T) {
	h, _ := newOrphansHandler()
	// repo is empty → Count=0 → Queued=0 with no error.
	resp, err := h.MatchOrphansInLibrary(context.Background(), &eveyv1.MatchOrphansInLibraryRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Queued)
}

func TestMatchOrphansInLibrary_ReturnsOrphanCount(t *testing.T) {
	h, mediaRepo := newOrphansHandler()

	// Items with no ProviderIDs are orphans.
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Type: library.MovieItem, Title: "Orphan A"}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "mi-2", LibraryID: "lib-1", Type: library.MovieItem, Title: "Orphan B"}))
	// This one has provider IDs → not an orphan.
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "mi-3", LibraryID: "lib-1", Type: library.MovieItem, Title: "Matched",
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "42"}},
	}))

	resp, err := h.MatchOrphansInLibrary(ctx, &eveyv1.MatchOrphansInLibraryRequest{})
	require.NoError(t, err)
	// mi-1 and mi-2 are orphans; mi-3 is not.
	assert.Equal(t, int32(2), resp.Queued)
}

func TestMatchOrphansInLibrary_LibraryFilter(t *testing.T) {
	h, mediaRepo := newOrphansHandler()

	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "mi-1", LibraryID: "lib-A", Type: library.MovieItem, Title: "Orphan in A"}))
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "mi-2", LibraryID: "lib-B", Type: library.MovieItem, Title: "Orphan in B"}))

	resp, err := h.MatchOrphansInLibrary(ctx, &eveyv1.MatchOrphansInLibraryRequest{LibraryId: "lib-A"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Queued)
}

func TestMatchOrphansInLibrary_NoBulkEnricher_ReturnsError(t *testing.T) {
	// CommandService with no enricher set → enricher is nil → type-assertion
	// to BulkItemEnricher fails → internal error propagated.
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	events := sharedfakes.NewFakeEventBus()
	providers := []domainmetadata.MetadataProvider{metafakes.NewFakeMetadataProvider("tmdb")}
	cmd := appmetadata.NewCommandService(overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(), events, providers)
	h := &adminMediaHandler{metadataCmd: cmd}

	_, err := h.MatchOrphansInLibrary(context.Background(), &eveyv1.MatchOrphansInLibraryRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
