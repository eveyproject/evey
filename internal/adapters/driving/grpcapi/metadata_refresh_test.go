package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmeta "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// refreshStream is a minimal server-streaming mock for RefreshMetadata tests.
type refreshStream struct {
	ctx     context.Context
	Sent    []*eveyv1.RefreshMetadataProgress
	SendErr error
}

func newRefreshStream(ctx context.Context) *refreshStream {
	return &refreshStream{ctx: ctx}
}

func (s *refreshStream) Send(m *eveyv1.RefreshMetadataProgress) error {
	if s.SendErr != nil {
		return s.SendErr
	}
	s.Sent = append(s.Sent, m)
	return nil
}

// grpc.ServerStream methods - no-op stubs sufficient for handler tests.
func (s *refreshStream) Context() context.Context     { return s.ctx }
func (s *refreshStream) SetHeader(grpcmeta.MD) error  { return nil }
func (s *refreshStream) SendHeader(grpcmeta.MD) error { return nil }
func (s *refreshStream) SetTrailer(grpcmeta.MD)       {}
func (s *refreshStream) SendMsg(any) error            { return nil }
func (s *refreshStream) RecvMsg(any) error            { return nil }

// compile-time check: refreshStream satisfies the stream interface.
var _ grpc.ServerStreamingServer[eveyv1.RefreshMetadataProgress] = (*refreshStream)(nil)

// newRefreshMetadataHandler wires a metadataCmd with a no-op enricher + a real
// media-item repo so the handler can resolve the item's provider IDs and run a
// re-enrich (the production path). Returns the handler, the seeded provider,
// and the repo for test setup.
func newRefreshMetadataHandler() (*metadataHandler, *metafakes.FakeMetadataProvider, *libfakes.FakeMediaItemStore) {
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	events := sharedfakes.NewFakeEventBus()
	providers := []domainmetadata.MetadataProvider{provider}
	mediaRepo := libfakes.NewFakeMediaItemStore()
	cmd := appmetadata.NewCommandService(overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(), events, providers,
		appmetadata.WithEnricher(orphanBulkEnricher{}, mediaRepo, mediaRepo),
	)
	query := appmetadata.NewQueryService(overrideRepo, artworkRepo, providers)
	return &metadataHandler{cmd: cmd, query: query, userAccess: newPermissiveTestUserAccess()}, provider, mediaRepo
}

// Forcing provider=tmdb resolves the item's stored TMDB id and re-enriches -
// it no longer passes an empty media type (the old 500) nor discards the result.
func TestRefreshMetadata_PreferredTMDB_ReenrichesFromStoredID(t *testing.T) {
	h, provider, mediaRepo := newRefreshMetadataHandler()
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "mi-1", Type: library.MovieItem, Title: "a prod title",
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "9772"}},
	}))
	provider.ByProviderID["9772"] = &domainmetadata.MetadataResult{Title: "a prod title"}

	stream := newRefreshStream(ctx)
	err := h.RefreshMetadata(&eveyv1.RefreshMetadataRequest{
		MediaItemId:       "mi-1",
		PreferredProvider: "tmdb",
	}, stream)

	require.NoError(t, err)
	require.Len(t, stream.Sent, 1)
	assert.Equal(t, "complete", stream.Sent[0].Phase)
}

// Forcing provider=imdb routes through the IMDB re-enrich path (keyed by the
// stored IMDB id), which the omdb/imdbapi-origin items need.
func TestRefreshMetadata_PreferredIMDB_UsesStoredIMDBID(t *testing.T) {
	h, provider, mediaRepo := newRefreshMetadataHandler()
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{
		ID: "mi-2", Type: library.MovieItem, Title: "a prod title",
		Catalog: &library.MediaCatalog{ProviderIDs: map[string]string{"imdb": "tt0118571"}},
	}))
	provider.ByProviderID["tt0118571"] = &domainmetadata.MetadataResult{Title: "a prod title"}

	stream := newRefreshStream(ctx)
	err := h.RefreshMetadata(&eveyv1.RefreshMetadataRequest{
		MediaItemId:       "mi-2",
		PreferredProvider: "imdb",
	}, stream)

	require.NoError(t, err)
	require.Len(t, stream.Sent, 1)
}

// A forced provider the item has no ID for fails cleanly (precondition), not 500.
func TestRefreshMetadata_PreferredTMDB_NoID_FailedPrecondition(t *testing.T) {
	h, _, mediaRepo := newRefreshMetadataHandler()
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "mi-3", Type: library.MovieItem, Title: "X"}))

	stream := newRefreshStream(ctx)
	err := h.RefreshMetadata(&eveyv1.RefreshMetadataRequest{MediaItemId: "mi-3", PreferredProvider: "tmdb"}, stream)
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestRefreshMetadata_UnsupportedProvider_InvalidArgument(t *testing.T) {
	h, _, mediaRepo := newRefreshMetadataHandler()
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "mi-4", Type: library.MovieItem, Title: "X"}))

	stream := newRefreshStream(ctx)
	err := h.RefreshMetadata(&eveyv1.RefreshMetadataRequest{MediaItemId: "mi-4", PreferredProvider: "wikipedia"}, stream)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRefreshMetadata_NoProvider_NoTMDBID_FailedPrecondition(t *testing.T) {
	// metadataCmd wired with mediaItems so GetMediaItem works.
	provider := metafakes.NewFakeMetadataProvider("tmdb")
	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	events := sharedfakes.NewFakeEventBus()
	providers := []domainmetadata.MetadataProvider{provider}
	mediaRepo := libfakes.NewFakeMediaItemStore()
	cmd := appmetadata.NewCommandService(overrideRepo, artworkRepo, metafakes.NewFakeArtworkDownloader(), events, providers,
		appmetadata.WithEnricher(orphanBulkEnricher{}, mediaRepo, mediaRepo),
	)
	query := appmetadata.NewQueryService(overrideRepo, artworkRepo, providers)
	h := &metadataHandler{cmd: cmd, query: query, userAccess: newPermissiveTestUserAccess()}

	// Create an item with no TMDB ID.
	ctx := context.Background()
	require.NoError(t, mediaRepo.Create(ctx, &library.MediaItem{ID: "mi-no-tmdb", Type: library.MovieItem, Title: "Unknown"}))

	stream := newRefreshStream(ctx)
	err := h.RefreshMetadata(&eveyv1.RefreshMetadataRequest{MediaItemId: "mi-no-tmdb"}, stream)

	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}
