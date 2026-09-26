package metadata

// Metadata refresh: provider-driven re-fetch (by provider ID / IMDB / TMDB)
// and read-side lookups used by the admin refresh flows.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

func (s *CommandService) RefreshMetadata(ctx context.Context, mediaItemID, providerID, providerName, mediaType string) (*metadata.MetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshMetadata")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", mediaItemID),
		attribute.String("provider.id", providerID),
		attribute.String("provider.name", providerName),
		attribute.String("media.type", mediaType),
	)
	for _, p := range s.providers {
		if p.Name() != providerName {
			continue
		}
		result, err := p.GetByProviderID(ctx, providerID, mediaType)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, fmt.Errorf("get metadata from %s: %w", providerName, err)
		}
		// Log-only: notification event - actual metadata fetch already succeeded above
		evt := metadata.MetadataRefreshed{MediaItemID: shared.MediaItemID(mediaItemID), Provider: providerName, OccurredAt: time.Now()}
		if err := s.events.Publish(ctx, evt); err != nil {
			slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
		}
		return result, nil
	}
	err := fmt.Errorf("provider %s not found", providerName)
	span.RecordError(err)
	span.SetStatus(otelcodes.Error, err.Error())
	return nil, err
}

func (s *CommandService) RefreshAllTranslations(ctx context.Context, onProgress func(done, total int)) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshAllTranslations")
	defer span.End()
	if s.enricher == nil || s.mediaItems == nil {
		err := errors.New("enricher not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	// A single List with an empty filter reads at most DefaultListLimit (500)
	// rows - the repository clamps a zero Limit to it - and this sweep then
	// reported "refresh all translations complete". On any catalogue past 500
	// items, everything after the first page never had its translations
	// refreshed, with no error and no warning. The sibling sweep in this same
	// package (AutoRefreshAllItems) already pages; this one had been left
	// behind.
	total, err := s.mediaItems.Count(ctx, library.ListMediaItemsFilter{})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("count all items: %w", err)
	}
	span.SetAttributes(attribute.Int("items.count", total))

	const pageSize = 500
	seen := 0
	for offset := 0; ; offset += pageSize {
		items, listErr := s.mediaItems.List(ctx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
		if listErr != nil {
			span.RecordError(listErr)
			span.SetStatus(otelcodes.Error, listErr.Error())
			return fmt.Errorf("list all items: %w", listErr)
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			seen++
			if item.Catalog == nil {
				continue
			}
			tmdbID, ok := item.Catalog.ProviderIDs["tmdb"]
			if !ok || tmdbID == "" {
				continue
			}
			mediaType := "movie"
			if item.Type == "series" || item.Type == "season" || item.Type == "episode" {
				mediaType = "tv"
			}
			s.enricher.EnrichTranslationsForItem(ctx, item, tmdbID, mediaType)
			if onProgress != nil {
				onProgress(seen, total)
			}
		}
		if len(items) < pageSize {
			break
		}
	}
	return nil
}

func (s *CommandService) GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.GetMediaItem")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", id))
	if s.mediaItems == nil {
		err := errors.New("media items repo not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return item, err
}

func (s *CommandService) RefreshItemMetadata(ctx context.Context, mediaItemID, tmdbID, mediaType string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshItemMetadata")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", mediaItemID),
		attribute.String("tmdb.id", tmdbID),
		attribute.String("media.type", mediaType),
	)
	if s.enricher == nil || s.mediaItems == nil {
		err := errors.New("enricher not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("get media item: %w", err)
	}
	result, _ := s.fetchFromProviders(ctx, tmdbID, mediaType, "")
	if result == nil {
		err := fmt.Errorf("no provider returned metadata for tmdb id %s", tmdbID)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	if err := s.enricher.EnrichItem(ctx, item, result); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("enrich item: %w", err)
	}
	evtTmdb := metadata.MetadataRefreshed{MediaItemID: shared.MediaItemID(mediaItemID), Provider: "tmdb", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtTmdb); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtTmdb.EventName()), slog.Any("err", err))
	}
	s.publishItemUpdated(ctx, shared.MediaItemID(mediaItemID), library.UpdateReasonMetadataRefreshed)
	return nil
}

func (s *CommandService) RefreshItemMetadataByIMDB(ctx context.Context, mediaItemID, imdbID, mediaType string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RefreshItemMetadataByIMDB")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", mediaItemID),
		attribute.String("imdb.id", imdbID),
		attribute.String("media.type", mediaType),
	)
	if s.enricher == nil || s.mediaItems == nil {
		err := errors.New("enricher not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("get media item: %w", err)
	}
	result, usedProvider := s.fetchFromProviders(ctx, imdbID, mediaType, "imdbapi")
	if result == nil {
		err := fmt.Errorf("no provider returned metadata for imdb id %s", imdbID)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}

	if result.ProviderIDs == nil {
		result.ProviderIDs = make(map[string]string)
	}
	result.ProviderIDs["imdb"] = imdbID

	// Cross-ref IMDB → TMDB so locale translations hit the right record.
	// Without this, the old item.Catalog.ProviderIDs["tmdb"] sticks and
	// FR/ES/… get translated titles+overviews from the PREVIOUS movie.
	// Drop the stale TMDB ID if no mapping exists so translations skip
	// rather than poisoning the item.
	if _, hasTmdb := result.ProviderIDs["tmdb"]; !hasTmdb {
		if newTmdbID, ok := s.lookupTMDBByIMDB(ctx, imdbID); ok {
			result.ProviderIDs["tmdb"] = newTmdbID
		} else if item.Catalog != nil {
			delete(item.Catalog.ProviderIDs, "tmdb")
		}
	}

	if err := s.enricher.EnrichItem(ctx, item, result); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("enrich item: %w", err)
	}
	evtImdb := metadata.MetadataRefreshed{MediaItemID: shared.MediaItemID(mediaItemID), Provider: usedProvider, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtImdb); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtImdb.EventName()), slog.Any("err", err))
	}
	s.publishItemUpdated(ctx, shared.MediaItemID(mediaItemID), library.UpdateReasonMetadataRefreshed)
	return nil
}

// lookupTMDBByIMDB asks any provider that implements ExternalIDLookup to
// map an IMDB ID to its TMDB counterpart. Only TMDB reliably answers this
// today; others return no-op. Used on IMDB-based refresh so translations
// follow the new item rather than the stale TMDB ID.
func (s *CommandService) lookupTMDBByIMDB(ctx context.Context, imdbID string) (string, bool) {
	for _, p := range s.providers {
		lookup, ok := p.(metadata.ExternalIDLookup)
		if !ok {
			continue
		}
		res, err := lookup.LookupByExternalID(ctx, "imdb", imdbID)
		if err != nil || res == nil {
			continue
		}
		if id := res.ProviderIDs["tmdb"]; id != "" {
			return id, true
		}
	}
	return "", false
}

// fetchFromProviders tries providers in order. If preferProvider is set,
// that provider is tried first, then the rest as fallback.
func (s *CommandService) fetchFromProviders(ctx context.Context, providerID, mediaType, preferProvider string) (*metadata.MetadataResult, string) {
	if preferProvider != "" {
		for _, p := range s.providers {
			if p.Name() == preferProvider {
				r, err := p.GetByProviderID(ctx, providerID, mediaType)
				if err == nil {
					return r, p.Name()
				}
				break
			}
		}
	}
	for _, p := range s.providers {
		if p.Name() == preferProvider {
			continue
		}
		r, err := p.GetByProviderID(ctx, providerID, mediaType)
		if err == nil {
			return r, p.Name()
		}
	}
	return nil, ""
}
