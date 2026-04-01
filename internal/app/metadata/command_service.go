package metadata

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type CommandService struct {
	overrides    metadata.MetadataOverrideRepository
	artwork      metadata.ArtworkRepository
	downloader   metadata.ArtworkDownloader
	events       shared.EventBus
	providers    []metadata.MetadataProvider
	mediaItems   library.MediaItemRepository
	mediaPersons library.MediaPersonRepository
	enricher     ItemEnricher
	workerWg     sync.WaitGroup
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

func (s *CommandService) WithMediaPersons(repo library.MediaPersonRepository) {
	s.mediaPersons = repo
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
		// Log-only: notification event — actual metadata fetch already succeeded above
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
	result, _ := s.fetchFromProviders(ctx, tmdbID, mediaType, "")
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
	result, usedProvider := s.fetchFromProviders(ctx, imdbID, mediaType, "imdbapi")
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

func (s *CommandService) SetOverride(ctx context.Context, override *metadata.MetadataOverride) error {
	override.CreatedAt = time.Now()
	if err := s.overrides.Set(ctx, override); err != nil {
		return err
	}
	// Log-only: audit event — override already persisted above
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
	// Log-only: audit event — overrides already deleted above
	evtRemoved := metadata.MetadataOverrideRemoved{MediaItemID: mediaItemID, FieldName: "*", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtRemoved); err != nil {
		slog.Warn("publish event", slog.String("event", evtRemoved.EventName()), slog.Any("err", err))
	}
	return nil
}

// deriveTitle extracts a human-readable title from the file path.
// For series: uses the top-level directory name.
// For movies/episodes: uses the filename without extension.
func deriveTitle(filePath, itemType string) string {
	if filePath == "" {
		return ""
	}
	if itemType == "series" || itemType == "season" {
		// Use the directory name
		return filepath.Base(filePath)
	}
	// Use filename without extension
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// deriveTitleFromChildren gets the series title from the top-level media directory
// by looking at an episode's file path (e.g. /media2/It welcome to derry/S01/... → "It welcome to derry").
func (s *CommandService) deriveTitleFromChildren(ctx context.Context, seriesID string) string {
	seasonType := "season"
	seasons, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{ParentID: &seriesID, Type: &seasonType, Limit: 1})
	if err != nil || len(seasons) == 0 {
		return ""
	}
	episodeType := "episode"
	episodes, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{ParentID: &seasons[0].ID, Type: &episodeType, Limit: 1})
	if err != nil || len(episodes) == 0 {
		return ""
	}
	// Extract the series directory name: /media2/It welcome to derry/S01/file.mkv → "It welcome to derry"
	fp := episodes[0].FilePath
	dir := filepath.Dir(fp)     // /media2/It welcome to derry/S01
	dir = filepath.Dir(dir)     // /media2/It welcome to derry
	return filepath.Base(dir)   // It welcome to derry
}

// ResetItemMetadata clears enriched metadata on an item and all its children
// (seasons + episodes for series, episodes for seasons) then triggers re-enrichment.
func (s *CommandService) ResetItemMetadata(ctx context.Context, mediaItemID string) error {
	if s.mediaItems == nil {
		return fmt.Errorf("media item repository not configured")
	}
	item, err := s.mediaItems.GetByID(ctx, mediaItemID)
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}

	// Reset the item itself
	s.resetSingleItem(ctx, item)

	// Cascade to children (series → seasons + episodes, season → episodes)
	if item.Type == string(library.Series) || item.Type == string(library.Season) {
		s.resetChildren(ctx, item.ID)
	}

	// Trigger re-enrichment with background context (request ctx may be done)
	if se, ok := s.enricher.(SingleItemEnricher); ok {
		go se.AutoEnrichItem(context.Background(), item, slog.Default())
	}

	return nil
}

// resetSingleItem clears enriched metadata fields, artwork, overrides, and cast for one item.
func (s *CommandService) resetSingleItem(ctx context.Context, item *library.MediaItem) {
	// Derive title from file path or directory name
	if item.FilePath != "" {
		item.Title = deriveTitle(item.FilePath, item.Type)
	} else if item.Type == string(library.Series) {
		if dirName := s.deriveTitleFromChildren(ctx, item.ID); dirName != "" {
			item.Title = dirName
		}
	}

	item.Overview = ""
	item.Tagline = ""
	item.OriginalTitle = ""
	item.ProviderIDs = nil
	item.Ratings = nil
	item.Genres = nil
	item.Studios = nil
	item.CommunityRating = nil
	item.OfficialRating = ""
	item.Trailers = nil
	item.Year = nil
	item.PremiereDate = nil
	item.UpdatedAt = time.Now()

	if err := s.mediaItems.Update(ctx, item); err != nil {
		slog.Warn("reset: update item", slog.String("id", item.ID), slog.Any("err", err))
	}
	if err := s.artwork.DeleteByMediaItemID(ctx, item.ID); err != nil {
		slog.Warn("reset: delete artwork", slog.String("id", item.ID), slog.Any("err", err))
	}
	if err := s.overrides.DeleteByMediaItemID(ctx, item.ID); err != nil {
		slog.Warn("reset: delete overrides", slog.String("id", item.ID), slog.Any("err", err))
	}
	if s.mediaPersons != nil {
		if err := s.mediaPersons.DeleteByMediaItemID(ctx, item.ID); err != nil {
			slog.Warn("reset: delete media persons", slog.String("id", item.ID), slog.Any("err", err))
		}
	}
}

// resetChildren resets all direct children and their children recursively.
func (s *CommandService) resetChildren(ctx context.Context, parentID string) {
	children, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{ParentID: &parentID})
	if err != nil {
		slog.Warn("reset: list children", slog.String("parentID", parentID), slog.Any("err", err))
		return
	}
	for _, child := range children {
		s.resetSingleItem(ctx, child)
		// Recurse (series → seasons → episodes)
		if child.Type == string(library.Season) {
			s.resetChildren(ctx, child.ID)
		}
	}
}

func (s *CommandService) DownloadArtwork(ctx context.Context, record *metadata.ArtworkRecord, destPath string) error {
	if record.SourceURL == nil {
		return fmt.Errorf("no source URL")
	}
	if err := s.downloader.Download(ctx, *record.SourceURL, destPath); err != nil {
		return fmt.Errorf("download artwork: %w", err)
	}
	record.LocalPath = &destPath
	// Log-only: notification event — file already downloaded to destPath above
	evtArtwork := metadata.ArtworkDownloaded{MediaItemID: record.MediaItemID, Type: record.Type, LocalPath: destPath, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtArtwork); err != nil {
		slog.Warn("publish event", slog.String("event", evtArtwork.EventName()), slog.Any("err", err))
	}
	return nil
}

// BulkItemEnricher is an optional extension of ItemEnricher for batch auto-enrichment.
type BulkItemEnricher interface {
	AutoEnrichAll(ctx context.Context, items []*library.MediaItem, log *slog.Logger)
}

// SingleItemEnricher is an optional extension of ItemEnricher for per-item auto-enrichment.
type SingleItemEnricher interface {
	AutoEnrichItem(ctx context.Context, item *library.MediaItem, log *slog.Logger)
}

// AutoRefreshAllItems auto-searches and enriches all media items using the enricher.
// Returns immediately if the enricher does not support bulk enrichment.
func (s *CommandService) AutoRefreshAllItems(ctx context.Context) error {
	be, ok := s.enricher.(BulkItemEnricher)
	if !ok {
		return fmt.Errorf("enricher does not support bulk refresh")
	}
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		items, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
		if err != nil {
			return fmt.Errorf("list items: %w", err)
		}
		if len(items) == 0 {
			break
		}
		be.AutoEnrichAll(ctx, items, slog.Default())
		if len(items) < pageSize {
			break
		}
	}
	return nil
}

func (s *CommandService) StartEnrichmentWorker(ctx context.Context) {
	be, bulkOK := s.enricher.(BulkItemEnricher)
	se, singleOK := s.enricher.(SingleItemEnricher)
	if !bulkOK && !singleOK {
		return
	}
	if bulkOK && s.mediaItems != nil {
		s.workerWg.Go(func() {
			const pageSize = 500
			for offset := 0; ; offset += pageSize {
				items, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
				if err != nil || len(items) == 0 {
					break
				}
				be.AutoEnrichAll(ctx, items, slog.Default())
				if len(items) < pageSize {
					break
				}
			}
		})
	}
	if bulkOK && s.mediaItems != nil {
		s.workerWg.Go(func() {
			s.dailyReEnrich(ctx, be)
		})
	}
	if singleOK && s.mediaItems != nil {
		queue := make(chan string, 100)
		for range 4 {
			s.workerWg.Go(func() {
				for {
					select {
					case <-ctx.Done():
						return
					case mediaID, ok := <-queue:
						if !ok {
							return
						}
						item, err := s.mediaItems.GetByID(ctx, mediaID)
						if err == nil {
							se.AutoEnrichItem(ctx, item, slog.Default())
						}
					}
				}
			})
		}
		s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
			if e, ok := ev.(library.MediaItemAdded); ok {
				select {
				case queue <- e.MediaItemID:
				default:
					slog.Warn("enrich queue full, dropping", slog.String("media_id", e.MediaItemID))
				}
			}
		})
	}
}

// WaitWorkers blocks until all enrichment goroutines finish (after ctx cancel).
func (s *CommandService) WaitWorkers() {
	s.workerWg.Wait()
}

func (s *CommandService) dailyReEnrich(ctx context.Context, be BulkItemEnricher) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ReEnrichWithoutOverrides(ctx, be)
		}
	}
}

// ReEnrichWithoutOverrides re-enriches all items that have no user overrides.
func (s *CommandService) ReEnrichWithoutOverrides(ctx context.Context, be BulkItemEnricher) (refreshed, skipped int) {
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		items, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
		if err != nil {
			slog.Warn("re-enrich: list items", slog.Any("err", err))
			return refreshed, skipped
		}
		if len(items) == 0 {
			break
		}

		ids := make([]string, len(items))
		for i, item := range items {
			ids[i] = item.ID
		}
		overridden, oErr := s.overrides.ListOverriddenMediaItemIDs(ctx, ids)
		if oErr != nil {
			slog.Warn("re-enrich: batch check overrides", slog.Any("err", oErr))
			return refreshed, skipped
		}

		var toEnrich []*library.MediaItem
		for _, item := range items {
			if overridden[item.ID] {
				skipped++
				continue
			}
			toEnrich = append(toEnrich, item)
		}
		if len(toEnrich) > 0 {
			slog.Info("re-enrich: page",
				slog.Int("items", len(toEnrich)),
				slog.Int("skipped_overrides", skipped),
			)
			be.AutoEnrichAll(ctx, toEnrich, slog.Default())
			refreshed += len(toEnrich)
		}

		if len(items) < pageSize {
			break
		}
	}
	slog.Info("re-enrich: done", slog.Int("refreshed", refreshed), slog.Int("skipped", skipped))
	return refreshed, skipped
}
