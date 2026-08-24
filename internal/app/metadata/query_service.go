package metadata

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

type QueryService struct {
	overrides metadata.MetadataOverrideRepository
	artwork   metadata.ArtworkQueryRepo
	providers []metadata.MetadataProvider
}

func NewQueryService(
	overrides metadata.MetadataOverrideRepository,
	artwork metadata.ArtworkQueryRepo,
	providers []metadata.MetadataProvider,
) *QueryService {
	return &QueryService{overrides: overrides, artwork: artwork, providers: providers}
}

func (s *QueryService) SearchMetadata(ctx context.Context, query, mediaType string) ([]metadata.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.SearchMetadata")
	defer span.End()
	span.SetAttributes(attribute.String("media.type", mediaType))
	var results []metadata.MetadataResult
	var failed int
	for _, p := range s.providers {
		r, err := p.Search(ctx, query, nil, mediaType)
		if err != nil {
			// Skipping ONE provider while another answers is the point of a
			// chain. Skipping it silently is not: a rate-limited provider makes
			// the admin's manual search look complete when it is short.
			failed++
			slog.WarnContext(ctx, "metadata search: provider failed",
				slog.String("provider", p.Name()), slog.Any("err", err))
			continue
		}
		results = append(results, r...)
	}
	span.SetAttributes(attribute.Int("results.count", len(results)))
	// Every provider failing is not an empty catalogue. Returning (nil, nil)
	// tells the admin "no provider knows this title" about a question nobody
	// managed to ask.
	if failed > 0 && failed == len(s.providers) {
		err := fmt.Errorf("metadata search: all %d provider(s) failed", failed)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	return results, nil
}

func (s *QueryService) ListOverrides(ctx context.Context, mediaItemID string) ([]*metadata.MetadataOverride, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ListOverrides")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	overrides, err := s.overrides.GetByMediaItem(ctx, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return overrides, err
}

// BatchListOverrides returns the overrides for each of the given items, keyed
// by media item ID and only including items that actually have overrides.
// Overrides are rare, so it first asks which items have any (one query) and
// fetches the full rows only for those, avoiding an N+1 over a whole grid.
func (s *QueryService) BatchListOverrides(ctx context.Context, mediaItemIDs []string) (map[string][]*metadata.MetadataOverride, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.BatchListOverrides")
	defer span.End()
	if len(mediaItemIDs) == 0 {
		return map[string][]*metadata.MetadataOverride{}, nil
	}
	// Single batched query (WHERE media_item_id = ANY) grouped by item, instead
	// of one round-trip per overridden item on the grid hot path.
	out, err := s.overrides.GetByMediaItems(ctx, mediaItemIDs)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return map[string][]*metadata.MetadataOverride{}, err
	}
	return out, nil
}

func (s *QueryService) GetArtwork(ctx context.Context, mediaItemID string) ([]*metadata.ArtworkRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.GetArtwork")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	records, err := s.artwork.GetByMediaItem(ctx, mediaItemID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return records, err
}

func (s *QueryService) BatchGetArtwork(ctx context.Context, mediaItemIDs []string) (map[string][]*metadata.ArtworkRecord, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.BatchGetArtwork")
	defer span.End()
	span.SetAttributes(attribute.Int("media_items.count", len(mediaItemIDs)))
	records, err := s.artwork.GetByMediaItems(ctx, mediaItemIDs)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return records, err
}

func (s *QueryService) ListProviders() []string {
	names := make([]string, 0, len(s.providers))
	for _, p := range s.providers {
		names = append(names, p.Name())
	}
	return names
}
