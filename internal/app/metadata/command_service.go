package metadata

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type CommandService struct {
	overrides  metadata.MetadataOverrideRepository
	artwork    metadata.ArtworkRepository
	downloader metadata.ArtworkDownloader
	events     shared.EventBus
	providers  []metadata.MetadataProvider
	mediaItems library.MediaItemRepository
	enricher   ItemEnricher
}

func NewCommandService(
	overrides metadata.MetadataOverrideRepository,
	artwork metadata.ArtworkRepository,
	downloader metadata.ArtworkDownloader,
	events shared.EventBus,
	providers []metadata.MetadataProvider,
) *CommandService {
	return &CommandService{
		overrides:  overrides,
		artwork:    artwork,
		downloader: downloader,
		events:     events,
		providers:  providers,
	}
}

func (s *CommandService) WithEnricher(enricher ItemEnricher, mediaItems library.MediaItemRepository) {
	s.enricher = enricher
	s.mediaItems = mediaItems
}

func (s *CommandService) RefreshMetadata(ctx context.Context, mediaItemID, providerID, providerName, mediaType string) (*metadata.MetadataResult, error) {
	for _, p := range s.providers {
		if p.Name() != providerName {
			continue
		}
		result, err := p.GetByProviderID(ctx, providerID, mediaType)
		if err != nil {
			return nil, fmt.Errorf("get metadata from %s: %w", providerName, err)
		}
		evt := metadata.MetadataRefreshed{MediaItemID: mediaItemID, Provider: providerName, OccurredAt: time.Now()}
		if err := s.events.Publish(ctx, evt); err != nil {
			slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
		}
		return result, nil
	}
	return nil, fmt.Errorf("provider %s not found", providerName)
}

func (s *CommandService) RefreshItemMetadata(ctx context.Context, mediaItemID, tmdbID, mediaType string) error {
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
	evtTmdb := metadata.MetadataRefreshed{MediaItemID: mediaItemID, Provider: "tmdb", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtTmdb); err != nil {
		slog.Warn("publish event", slog.String("event", evtTmdb.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) RefreshItemMetadataByIMDB(ctx context.Context, mediaItemID, imdbID, mediaType string) error {
	if s.enricher == nil || s.mediaItems == nil {
		return fmt.Errorf("enricher not configured")
	}
	item, err := s.mediaItems.GetByID(ctx, mediaItemID)
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}

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

	if result.ProviderIDs == nil {
		result.ProviderIDs = make(map[string]string)
	}
	result.ProviderIDs["imdb"] = imdbID

	if err := s.enricher.EnrichItem(ctx, item, result); err != nil {
		return fmt.Errorf("enrich item: %w", err)
	}
	evtImdb := metadata.MetadataRefreshed{MediaItemID: mediaItemID, Provider: usedProvider, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtImdb); err != nil {
		slog.Warn("publish event", slog.String("event", evtImdb.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) SetOverride(ctx context.Context, override *metadata.MetadataOverride) error {
	override.CreatedAt = time.Now()
	if err := s.overrides.Set(ctx, override); err != nil {
		return err
	}
	evtOverride := metadata.MetadataOverrideSet{MediaItemID: override.MediaItemID, FieldName: override.FieldName, EditedBy: override.EditedBy, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtOverride); err != nil {
		slog.Warn("publish event", slog.String("event", evtOverride.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) RemoveOverride(ctx context.Context, id string) error {
	return s.overrides.Delete(ctx, id)
}

func (s *CommandService) ResetOverrides(ctx context.Context, mediaItemID string) error {
	if err := s.overrides.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		return err
	}
	evtRemoved := metadata.MetadataOverrideRemoved{MediaItemID: mediaItemID, FieldName: "*", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtRemoved); err != nil {
		slog.Warn("publish event", slog.String("event", evtRemoved.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) DownloadArtwork(ctx context.Context, record *metadata.ArtworkRecord, destPath string) error {
	if record.SourceURL == nil {
		return fmt.Errorf("no source URL")
	}
	if err := s.downloader.Download(ctx, *record.SourceURL, destPath); err != nil {
		return fmt.Errorf("download artwork: %w", err)
	}
	record.LocalPath = &destPath
	evtArtwork := metadata.ArtworkDownloaded{MediaItemID: record.MediaItemID, Type: record.Type, LocalPath: destPath, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtArtwork); err != nil {
		slog.Warn("publish event", slog.String("event", evtArtwork.EventName()), slog.Any("err", err))
	}
	return nil
}
