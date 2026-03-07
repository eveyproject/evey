package metadata

import (
	"context"
	"fmt"
	"time"

	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type Service struct {
	overrides  metadata.MetadataOverrideRepository
	artwork    metadata.ArtworkRepository
	downloader metadata.ArtworkDownloader
	events     shared.EventBus
	providers  []metadata.MetadataProvider
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
