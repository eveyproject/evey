package metadata

// Bulk/background enrichment: orphan matching, admin research re-match,
// the enrichment worker pool, and periodic re-enrichment.

import (
	"context"
	"encoding/json"
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

// Single-item enrich pipeline sizing: a small worker pool keeps provider
// rate limits comfortable while the buffered queue absorbs scan bursts.
const (
	enrichQueueDepth  = 100
	enrichWorkerCount = 4
)

// enrichableItemType reports whether an added item lives in media_items and can
// be enriched by an external provider. Music tracks/albums and photos publish
// MediaItemAdded too (since the 2026-08-26 audio/image ingestion work) but have
// no media_items row, so a GetByID would miss - and worse, a bulk music import
// floods the bounded enrich queue and drops the real video items waiting in it.
// Same allowlist shape as the trickplay and intro subscribers.
func enrichableItemType(t library.MediaItemType) bool {
	switch t {
	case library.MovieItem, library.Series, library.Season, library.Episode:
		return true
	default:
		return false
	}
}

// BulkItemEnricher is an optional extension of ItemEnricher for batch auto-enrichment.
//
// Kept separate from ForceBulkItemEnricher (rather than folded into one
// interface with both methods) because MatchOrphansInLibrary only ever needs
// this narrower, non-force capability and some ItemEnricher test doubles
// outside this package implement only AutoEnrichAll - widening this
// interface would break their assertion.
type BulkItemEnricher interface {
	AutoEnrichAll(ctx context.Context, items []*library.MediaItem, log *slog.Logger)
}

// ForceBulkItemEnricher is the admin-triggered refresh variant - it re-runs
// the TMDB search even on items that already have an overview. Admin field
// overrides are still respected. Wired by the RefreshAllMetadata RPC.
type ForceBulkItemEnricher interface {
	ForceAutoEnrichAll(ctx context.Context, items []*library.MediaItem, log *slog.Logger)
}

// SingleItemEnricher is an optional extension of ItemEnricher for per-item
// auto-enrichment. Folds what used to be two separate capability interfaces
// (plain AutoEnrichItem and a force variant bypassing the "already enriched"
// short-circuit, used by ResearchMetadata to correct a wrong TMDB match in
// place) into one: nothing outside this package needs to detect just one
// half, so a single type assertion covers both. Admin field overrides are
// still respected by the underlying enricher.
type SingleItemEnricher interface {
	AutoEnrichItem(ctx context.Context, item *library.MediaItem, log *slog.Logger)
	ForceAutoEnrichItem(ctx context.Context, item *library.MediaItem, log *slog.Logger)
}

// TranslationClearer wipes an item's stored locale translations. Optional
// capability used by ResearchMetadata to drop rows tied to a wrong match.
type TranslationClearer interface {
	ClearTranslations(ctx context.Context, mediaItemID string)
}

// ResearchMetadataResult is what the gRPC handler reports back: did we land on
// a different title (i.e. the re-search produced a new match) and what is it.
type ResearchMetadataResult struct {
	Item     *library.MediaItem
	Matched  bool
	NewTitle string
}

// ResearchMetadata clears the wrong-match traces from an item (overrides,
// provider IDs, year, enriched title/overview/tagline/originalTitle) and
// re-runs auto-enrichment from the stable ScanTitle. Used by the admin
// "Re-match" button to fix a bad TMDB match in one click without forcing
// the user to manually walk through the override repo.
//
// Steps:
//  1. Load item.
//  2. Delete every override targeting the item.
//  3. Strip ProviderIDs (tmdb/imdb/thetvdb) and Year from the catalog.
//  4. Reset Title to ScanTitle (when set), clear OriginalTitle/Overview/Tagline.
//  5. Persist.
//  6. Force-enrich (bypasses overview short-circuit so the search runs even
//     though the item has been touched many times before).
//  7. Re-load and report whether we ended up with a different title.
func (s *CommandService) ResearchMetadata(ctx context.Context, mediaItemID string) (*ResearchMetadataResult, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ResearchMetadata")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	if s.mediaItems == nil {
		err := errors.New("media item repository not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("get media item: %w", err)
	}

	previousTitle := item.Title

	// 1. Drop every override on the item - they were tied to the bad match.
	if err = s.overrides.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("delete overrides: %w", err)
	}

	// 1b. Drop stored locale translations - they belong to the wrong match and
	// would otherwise overlay the corrected base title on the read path (this
	// is what kept a re-matched season showing the foreign show's title).
	if tc, ok := s.enricher.(TranslationClearer); ok {
		tc.ClearTranslations(ctx, mediaItemID)
	}

	// 2. Strip wrong provider IDs + year so the next enrichment runs a fresh
	// search instead of rehydrating from the stale TMDB record.
	if item.Catalog != nil {
		if item.Catalog.ProviderIDs != nil {
			delete(item.Catalog.ProviderIDs, "tmdb")
			delete(item.Catalog.ProviderIDs, "imdb")
			delete(item.Catalog.ProviderIDs, "thetvdb")
		}
		item.Catalog.Year = nil
	}

	// 3. Reset enriched fields so re-enrichment writes a clean record.
	// ScanTitle is the filesystem-derived title that survived prior matches -
	// it's the source of truth for the search query.
	if item.ScanTitle != "" {
		item.Title = item.ScanTitle
	}
	item.OriginalTitle = ""
	item.Overview = ""
	item.Tagline = ""
	item.UpdatedAt = time.Now()

	if err = s.itemUpdater.Update(ctx, item); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("update media item: %w", err)
	}

	// Audit-style event so admin tooling can surface the action.
	evtRemoved := metadata.MetadataOverrideRemoved{MediaItemID: shared.MediaItemID(mediaItemID), FieldName: "*", OccurredAt: time.Now()}
	if err = s.events.Publish(ctx, evtRemoved); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtRemoved.EventName()), slog.Any("err", err))
	}

	// 4. Re-fetch (state pre-enrich) so we hand the enricher the persisted form.
	item, err = s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("reload media item: %w", err)
	}

	// 5. Force-enrich. ForceAutoEnrichItem bypasses the "already has overview"
	// short-circuit; AutoEnrichItem would refuse to act because we just left
	// an overview-less item but its enricher chain still walks the same path.
	if se, ok := s.enricher.(SingleItemEnricher); ok {
		se.ForceAutoEnrichItem(ctx, item, slog.Default())
	}

	// 6. Reload to pick up enrichment writes, decide whether the title moved.
	final, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("reload after enrich: %w", err)
	}
	matched := final.Title != "" && final.Title != previousTitle && final.Overview != ""
	return &ResearchMetadataResult{Item: final, Matched: matched, NewTitle: final.Title}, nil
}

// MatchOrphansInLibrary auto-searches and enriches *only* the items in the
// library whose Catalog.ProviderIDs is effectively empty - i.e. items the
// initial scan failed to match against any provider (typically TMDB rate
// limit or network blip during bulk import). Idempotent: items that get
// matched leave the orphan set, so re-running the action just picks up
// what remains.
//
// Unlike AutoRefreshAllItems, this one returns immediately with the total
// count of orphans detected; the actual enrichment happens in a detached
// goroutine that paginates through the orphan set (the postgres adapter
// caps each List at DefaultListLimit=500). The underlying enricher pool
// (max 3 concurrent + provider-side rate limit) drains each batch before
// the next page is fetched. Two safety stops: zero orphans remaining, or
// no progress between iterations (e.g. provider unreachable).
func (s *CommandService) MatchOrphansInLibrary(ctx context.Context, libraryID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.MatchOrphansInLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	be, ok := s.enricher.(BulkItemEnricher)
	if !ok {
		err := errors.New("enricher does not support bulk refresh")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return 0, err
	}
	missing := true
	filter := library.ListMediaItemsFilter{MissingProviderIDs: &missing}
	if libraryID != "" {
		libID := shared.LibraryID(libraryID)
		filter.LibraryID = &libID
	}
	total, err := s.mediaItems.Count(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return 0, fmt.Errorf("count orphans: %w", err)
	}
	span.SetAttributes(attribute.Int("orphans.total", total))
	if total == 0 {
		return 0, nil
	}
	// Drain in background - paginated, with stall detection so a downstream
	// provider outage doesn't burn CPU on the same 500 items forever. Registered
	// on workerWg + the service ctx so a shutdown cancels and awaits it, rather
	// than leaving it running on context.Background() past a redeploy.
	s.workerWg.Go(func() { s.drainOrphans(libraryID, filter, total, be) })
	return total, nil
}

// drainOrphans is the background enrichment loop for MatchOrphansInLibrary.
// Detached from any request context (HTTP/gRPC) so caller disconnect or
// the openresty 90s gateway timeout do not cancel a minutes-to-hours job.
// Stops on empty remainder (drain complete), Count stall (no progress),
// List failure, or the safety iteration cap.
func (s *CommandService) drainOrphans(libraryID string, filter library.ListMediaItemsFilter, total int, be BulkItemEnricher) {
	const (
		pageSize      = 500 // matches postgres DefaultListLimit
		maxIterations = 500 // safety cap: ≥ 250k items per invocation
	)
	log := slog.Default()
	// Detached from the request ctx (survives caller disconnect / gateway
	// timeout) but bound to the SERVICE ctx so a shutdown cancels it.
	ctx := s.shutdownCtx
	if ctx == nil {
		ctx = context.Background()
	}
	lastCount := total
	for i := range maxIterations {
		if ctx.Err() != nil {
			log.Info("match orphans: cancelled (shutdown)",
				slog.String("library_id", libraryID), slog.Int("iterations", i))
			return
		}
		f := filter
		f.Limit = pageSize
		items, err := s.mediaItems.List(ctx, f)
		if err != nil {
			log.Warn("match orphans: list page failed",
				slog.String("library_id", libraryID),
				slog.Any("err", err))
			return
		}
		if len(items) == 0 {
			log.Info("match orphans: drain complete",
				slog.String("library_id", libraryID),
				slog.Int("total_processed", total),
				slog.Int("iterations", i))
			return
		}
		be.AutoEnrichAll(ctx, items, log)
		current, err := s.mediaItems.Count(ctx, f)
		if err != nil {
			log.Warn("match orphans: recount failed",
				slog.String("library_id", libraryID),
				slog.Any("err", err))
			return
		}
		if current >= lastCount {
			log.Warn("match orphans: no progress between iterations, stopping",
				slog.String("library_id", libraryID),
				slog.Int("remaining", current),
				slog.Int("iterations", i+1))
			return
		}
		lastCount = current
	}
	log.Warn("match orphans: hit safety iteration cap",
		slog.String("library_id", libraryID),
		slog.Int("remaining", lastCount))
}

// AutoRefreshAllItems auto-searches and enriches all media items using the
// enricher. This is the admin-triggered "Refresh all metadata" entry point -
// it bypasses the "already has overview" short-circuit so past bad TMDB
// matches get a chance to be corrected. Items with admin field overrides
// are still skipped (manual intent wins).
// Falls back to non-force bulk if the enricher only implements the basic
// BulkItemEnricher interface.
func (s *CommandService) AutoRefreshAllItems(ctx context.Context, libraryID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.AutoRefreshAllItems")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", libraryID))
	fbe, forceOK := s.enricher.(ForceBulkItemEnricher)
	be, bulkOK := s.enricher.(BulkItemEnricher)
	if !forceOK && !bulkOK {
		err := errors.New("enricher does not support bulk refresh")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	const pageSize = 500
	// Resumable cursor: the sweep runs detached and dies with the process, so
	// the completed offset persists after every page. A re-trigger for the
	// SAME library scope picks up there instead of re-enriching the whole
	// catalogue from zero. Offset resume is approximate under catalogue
	// mutation (items may shift pages) - acceptable for an idempotent enrich.
	startOffset := 0
	if cur, ok := s.loadRefreshCursor(ctx, libraryID); ok {
		startOffset = cur
		slog.Info("metadata refresh-all: resuming from persisted cursor",
			slog.String("library_id", libraryID), slog.Int("offset", startOffset))
	}
	for offset := startOffset; ; offset += pageSize {
		filter := library.ListMediaItemsFilter{Limit: pageSize, Offset: offset}
		if libraryID != "" {
			libID := shared.LibraryID(libraryID)
			filter.LibraryID = &libID
		}
		items, err := s.mediaItems.List(ctx, filter)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return fmt.Errorf("list items: %w", err)
		}
		if len(items) == 0 {
			break
		}
		if forceOK {
			fbe.ForceAutoEnrichAll(ctx, items, slog.Default())
		} else {
			be.AutoEnrichAll(ctx, items, slog.Default())
		}
		s.storeRefreshCursor(ctx, libraryID, offset+pageSize)
		if len(items) < pageSize {
			break
		}
	}
	s.clearRefreshCursor(ctx, libraryID)
	return nil
}

// metadataRefreshCursorKeyPrefix persists the refresh-all sweep position in
// server_config (no schema change needed): {"library_id":"...","offset":N}.
// Keyed PER LIBRARY: two concurrent sweeps used to share one global key and
// clobber (then clear) each other's cursor.
const metadataRefreshCursorKeyPrefix = "metadata.refresh_all.cursor."

func refreshCursorKey(libraryID string) string {
	return metadataRefreshCursorKeyPrefix + libraryID
}

type refreshCursor struct {
	LibraryID string `json:"library_id"`
	Offset    int    `json:"offset"`
}

func (s *CommandService) loadRefreshCursor(ctx context.Context, libraryID string) (int, bool) {
	if s.serverConfig == nil {
		return 0, false
	}
	entry, err := s.serverConfig.Get(ctx, refreshCursorKey(libraryID))
	if err != nil || entry == nil || entry.Value == "" {
		return 0, false
	}
	var cur refreshCursor
	if json.Unmarshal([]byte(entry.Value), &cur) != nil || cur.LibraryID != libraryID || cur.Offset <= 0 {
		return 0, false
	}
	return cur.Offset, true
}

func (s *CommandService) storeRefreshCursor(ctx context.Context, libraryID string, offset int) {
	if s.serverConfig == nil {
		return
	}
	raw, err := json.Marshal(refreshCursor{LibraryID: libraryID, Offset: offset})
	if err != nil {
		return
	}
	if err := s.serverConfig.Set(ctx, refreshCursorKey(libraryID), string(raw)); err != nil {
		slog.WarnContext(ctx, "metadata refresh-all: cursor persist failed", slog.Any("err", err))
	}
}

func (s *CommandService) clearRefreshCursor(ctx context.Context, libraryID string) {
	if s.serverConfig == nil {
		return
	}
	if err := s.serverConfig.Delete(ctx, refreshCursorKey(libraryID)); err != nil {
		slog.WarnContext(ctx, "metadata refresh-all: cursor clear failed", slog.Any("err", err))
	}
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
		queue := make(chan string, enrichQueueDepth)
		for range enrichWorkerCount {
			s.workerWg.Go(func() {
				for {
					select {
					case <-ctx.Done():
						return
					case mediaID, ok := <-queue:
						if !ok {
							return
						}
						item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaID))
						if err == nil {
							se.AutoEnrichItem(ctx, item, slog.Default())
						}
					}
				}
			})
		}
		s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
			e, ok := ev.(library.MediaItemAdded)
			if !ok || !enrichableItemType(e.Type) {
				return
			}
			select {
			case queue <- string(e.MediaItemID):
			default:
				slog.WarnContext(ctx, "enrich queue full, dropping", slog.String("media_id", string(e.MediaItemID)))
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
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ReEnrichWithoutOverrides")
	defer span.End()
	defer func() {
		span.SetAttributes(
			attribute.Int("items.refreshed", refreshed),
			attribute.Int("items.skipped", skipped),
		)
	}()
	const pageSize = 500
	for offset := 0; ; offset += pageSize {
		items, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{Limit: pageSize, Offset: offset})
		if err != nil {
			slog.WarnContext(ctx, "re-enrich: list items", slog.Any("err", err))
			return refreshed, skipped
		}
		if len(items) == 0 {
			break
		}

		ids := make([]string, len(items))
		for i, item := range items {
			ids[i] = string(item.ID)
		}
		overridden, oErr := s.overrides.ListOverriddenMediaItemIDs(ctx, ids)
		if oErr != nil {
			slog.WarnContext(ctx, "re-enrich: batch check overrides", slog.Any("err", oErr))
			return refreshed, skipped
		}

		var toEnrich []*library.MediaItem
		for _, item := range items {
			if overridden[string(item.ID)] {
				skipped++
				continue
			}
			toEnrich = append(toEnrich, item)
		}
		if len(toEnrich) > 0 {
			slog.InfoContext(ctx, "re-enrich: page",
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
	slog.InfoContext(ctx, "re-enrich: done", slog.Int("refreshed", refreshed), slog.Int("skipped", skipped))
	return refreshed, skipped
}
