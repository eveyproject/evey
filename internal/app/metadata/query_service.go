package metadata

import (
	"context"

	"github.com/evey-project/evey/internal/domain/metadata"
)

type QueryService struct {
	overrides metadata.MetadataOverrideRepository
	artwork   metadata.ArtworkRepository
	providers []metadata.MetadataProvider
}

func NewQueryService(
	overrides metadata.MetadataOverrideRepository,
	artwork metadata.ArtworkRepository,
	providers []metadata.MetadataProvider,
) *QueryService {
	return &QueryService{overrides: overrides, artwork: artwork, providers: providers}
}

func (s *QueryService) SearchMetadata(ctx context.Context, query, mediaType string) ([]metadata.MetadataResult, error) {
	var results []metadata.MetadataResult
	for _, p := range s.providers {
		r, err := p.Search(ctx, query, nil, mediaType)
		if err != nil {
			continue
		}
		results = append(results, r...)
	}
	return results, nil
}

func (s *QueryService) ListOverrides(ctx context.Context, mediaItemID string) ([]*metadata.MetadataOverride, error) {
	return s.overrides.GetByMediaItem(ctx, mediaItemID)
}

func (s *QueryService) GetArtwork(ctx context.Context, mediaItemID string) ([]*metadata.ArtworkRecord, error) {
	return s.artwork.GetByMediaItem(ctx, mediaItemID)
}

func (s *QueryService) BatchGetArtwork(ctx context.Context, mediaItemIDs []string) (map[string][]*metadata.ArtworkRecord, error) {
	return s.artwork.GetByMediaItems(ctx, mediaItemIDs)
}

func (s *QueryService) ListProviders() []string {
	names := make([]string, 0, len(s.providers))
	for _, p := range s.providers {
		names = append(names, p.Name())
	}
	return names
}
