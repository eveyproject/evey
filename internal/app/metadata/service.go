package metadata

import (
	"context"
	"fmt"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

// ItemEnricher persists metadata results (artwork, people) for a media item.
type ItemEnricher interface {
	EnrichItem(ctx context.Context, item *library.MediaItem, result *metadata.MetadataResult) error
}

type Service struct {
	overrides  metadata.MetadataOverrideRepository
	artwork    metadata.ArtworkRepository
	downloader metadata.ArtworkDownloader
	events     shared.EventBus
	providers  []metadata.MetadataProvider
	mediaItems library.MediaItemRepository
	enricher   ItemEnricher
}

func NewService(
	overrides metadata.MetadataOverrideRepository,
	artwork metadata.ArtworkRepository,
	downloader metadata.ArtworkDownloader,
	events shared.EventBus,
	providers []metadata.MetadataProvider,
) *Service {
	return &Service{
		overrides:  overrides,
		artwork:    artwork,
		downloader: downloader,
		events:     events,
		providers:  providers,
	}
}

// WithEnricher sets the enricher and media item repo (optional, used for RefreshItemMetadata).
func (s *Service) WithEnricher(enricher ItemEnricher, mediaItems library.MediaItemRepository) {
	s.enricher = enricher
	s.mediaItems = mediaItems
}

func (s *Service) SearchMetadata(ctx context.Context, query, mediaType string) ([]metadata.MetadataResult, error) {
	var results []metadata.MetadataResult
	for _, p := range s.providers {
		r, err := p.Search(ctx, query, mediaType)
		if err != nil {
			continue
		}
		results = append(results, r...)
	}
	return results, nil
}

func (s *Service) RefreshMetadata(ctx context.Context, mediaItemID, providerID, providerName, mediaType string) (*metadata.MetadataResult, error) {
	for _, p := range s.providers {
		if p.Name() != providerName {
			continue
		}
		result, err := p.GetByProviderID(ctx, providerID, mediaType)
		if err != nil {
			return nil, fmt.Errorf("get metadata from %s: %w", providerName, err)
		}
		s.events.Publish(ctx, metadata.MetadataRefreshed{
			MediaItemID: mediaItemID,
			Provider:    providerName,
			OccurredAt:  time.Now(),
		})
		return result, nil
	}
	return nil, fmt.Errorf("provider %s not found", providerName)
}

// RefreshItemMetadata fetches fresh metadata from tmdbID and persists artwork/people.
func (s *Service) RefreshItemMetadata(ctx context.Context, mediaItemID, tmdbID, mediaType string) error {
	if s.enricher == nil || s.mediaItems == nil {
		return fmt.Errorf("enricher not configured")
	}
	item, err := s.mediaItems.GetByID(ctx, mediaItemID)
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	var result *metadata.MetadataResult
	for _, p := range s.providers {
		r, err := p.GetByProviderID(ctx, tmdbID, mediaType)
		if err != nil {
			continue
		}
		result = r
		break
	}
	if result == nil {
		return fmt.Errorf("no provider returned metadata for tmdb id %s", tmdbID)
	}
	if err := s.enricher.EnrichItem(ctx, item, result); err != nil {
		return fmt.Errorf("enrich item: %w", err)
	}
	s.events.Publish(ctx, metadata.MetadataRefreshed{
		MediaItemID: mediaItemID,
		Provider:    "tmdb",
		OccurredAt:  time.Now(),
	})
	return nil
}

// RefreshItemMetadataByIMDB fetches metadata by IMDB ID, trying providers that
// support IMDB IDs first, then falling back to the rest. The imdbID must be in
// tt-prefixed format (e.g. "tt0111161").
func (s *Service) RefreshItemMetadataByIMDB(ctx context.Context, mediaItemID, imdbID, mediaType string) error {
	if s.enricher == nil || s.mediaItems == nil {
		return fmt.Errorf("enricher not configured")
	}
	item, err := s.mediaItems.GetByID(ctx, mediaItemID)
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}

	// Try "imdbapi" provider first, then others as fallback.
	var result *metadata.MetadataResult
	var usedProvider string
	for _, p := range s.providers {
		if p.Name() == "imdbapi" {
			r, err := p.GetByProviderID(ctx, imdbID, mediaType)
			if err == nil {
				result = r
				usedProvider = p.Name()
				break
			}
		}
	}
	if result == nil {
		for _, p := range s.providers {
			if p.Name() == "imdbapi" {
				continue
			}
			r, err := p.GetByProviderID(ctx, imdbID, mediaType)
			if err == nil {
				result = r
				usedProvider = p.Name()
				break
			}
		}
	}
	if result == nil {
		return fmt.Errorf("no provider returned metadata for imdb id %s", imdbID)
	}

	// Preserve the imdb ID in provider IDs.
	if result.ProviderIDs == nil {
		result.ProviderIDs = make(map[string]string)
	}
	result.ProviderIDs["imdb"] = imdbID

	if err := s.enricher.EnrichItem(ctx, item, result); err != nil {
		return fmt.Errorf("enrich item: %w", err)
	}
	s.events.Publish(ctx, metadata.MetadataRefreshed{
		MediaItemID: mediaItemID,
		Provider:    usedProvider,
		OccurredAt:  time.Now(),
	})
	return nil
}

func (s *Service) SetOverride(ctx context.Context, override *metadata.MetadataOverride) error {
	override.CreatedAt = time.Now()
	if err := s.overrides.Set(ctx, override); err != nil {
		return err
	}
	s.events.Publish(ctx, metadata.MetadataOverrideSet{
		MediaItemID: override.MediaItemID,
		FieldName:   override.FieldName,
		EditedBy:    override.EditedBy,
		OccurredAt:  time.Now(),
	})
	return nil
}

func (s *Service) RemoveOverride(ctx context.Context, id string) error {
	return s.overrides.Delete(ctx, id)
}

func (s *Service) ResetOverrides(ctx context.Context, mediaItemID string) error {
	if err := s.overrides.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		return err
	}
	s.events.Publish(ctx, metadata.MetadataOverrideRemoved{
		MediaItemID: mediaItemID,
		FieldName:   "*",
		OccurredAt:  time.Now(),
	})
	return nil
}

func (s *Service) ListOverrides(ctx context.Context, mediaItemID string) ([]*metadata.MetadataOverride, error) {
	return s.overrides.GetByMediaItem(ctx, mediaItemID)
}

func (s *Service) GetArtwork(ctx context.Context, mediaItemID string) ([]*metadata.ArtworkRecord, error) {
	return s.artwork.GetByMediaItem(ctx, mediaItemID)
}

func (s *Service) ListProviders() []string {
	names := make([]string, 0, len(s.providers))
	for _, p := range s.providers {
		names = append(names, p.Name())
	}
	return names
}

func (s *Service) DownloadArtwork(ctx context.Context, record *metadata.ArtworkRecord, destPath string) error {
	if record.SourceURL == nil {
		return fmt.Errorf("no source URL")
	}
	if err := s.downloader.Download(ctx, *record.SourceURL, destPath); err != nil {
		return fmt.Errorf("download artwork: %w", err)
	}
	record.LocalPath = &destPath
	s.events.Publish(ctx, metadata.ArtworkDownloaded{
		MediaItemID: record.MediaItemID,
		Type:        record.Type,
		LocalPath:   destPath,
		OccurredAt:  time.Now(),
	})
	return nil
}
