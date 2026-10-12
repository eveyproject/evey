package metadata

// Field-level metadata overrides (admin edits) and the metadata reset flow
// that clears enriched fields before triggering re-enrichment.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

func (s *CommandService) SetOverride(ctx context.Context, override *metadata.MetadataOverride) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.SetOverride")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", string(override.MediaItemID)),
		attribute.String("override.field", override.FieldName),
	)
	// The invariant lives in NewMetadataOverride; re-checking it here is what
	// keeps it an invariant rather than a property of one call site. Today the
	// only caller goes through that constructor, so this changes nothing - the
	// next one (an import job, a backfill) is why it is here.
	if err := override.Validate(); err != nil {
		return fmt.Errorf("CommandService.SetOverride: %w", err)
	}
	override.CreatedAt = time.Now()
	if err := s.overrides.Set(ctx, override); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("CommandService.SetOverride: %w", err)
	}
	// Log-only: audit event - override already persisted above
	evtOverride := metadata.MetadataOverrideSet{MediaItemID: override.MediaItemID, FieldName: override.FieldName, EditedBy: override.EditedBy, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtOverride); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtOverride.EventName()), slog.Any("err", err))
	}
	s.publishItemUpdated(ctx, override.MediaItemID, library.UpdateReasonMetadataEdited)
	// When an admin edits the title on an orphan item (no TMDB/IMDB IDs yet),
	// the intent is "this is the correct title - please match it now". The
	// enricher reads media_items.title for its search query, so propagate
	// the override into the raw title and kick a background auto-enrich.
	// Without this hook, fixing a parser-polluted title (e.g. stripping
	// "720" or "arte") via the UI only updated the display layer; match-
	// orphans kept failing because the search used the still-dirty title.
	s.propagateTitleOverrideForOrphan(ctx, override)
	return nil
}

// propagateTitleOverrideForOrphan is the orphan-recovery path for title
// edits. Best-effort: every step that fails just logs and bails - the
// override write itself succeeded above, so we never want to return an
// error from this side-effect.
func (s *CommandService) propagateTitleOverrideForOrphan(ctx context.Context, override *metadata.MetadataOverride) {
	if s.mediaItems == nil || s.enricher == nil {
		return
	}
	// Only the title (or original_title as a fallback) is search-relevant.
	// Other fields (overview, tagline, year) don't change TMDB match outcomes.
	// Field names match what MediaDetailEdit writes (snake_case for
	// original_title - see frontend FIELD_MAP and library_handler.go:986).
	if override.FieldName != "title" && override.FieldName != "original_title" {
		return
	}
	// CustomValue is any so the same override row can carry numerics,
	// dates, etc. for other fields - but title/originalTitle must be a
	// non-empty string for the propagation path to make sense.
	val, ok := override.CustomValue.(string)
	if !ok || val == "" {
		return
	}
	item, err := s.mediaItems.GetByID(ctx, override.MediaItemID)
	if err != nil || item == nil {
		return
	}
	// Only propagate for orphans - a matched item's title edit is
	// presentation-only and must not retrigger TMDB lookups.
	if item.Catalog != nil && len(item.Catalog.ProviderIDs) > 0 {
		return
	}
	switch override.FieldName {
	case "title":
		if item.Title == val {
			return // already aligned, nothing to do
		}
		item.Title = val
	case "original_title":
		if item.OriginalTitle == val {
			return
		}
		item.OriginalTitle = val
	}
	item.UpdatedAt = time.Now()
	if err := s.itemUpdater.Update(ctx, item); err != nil {
		slog.WarnContext(ctx, "propagate title override to media_items failed",
			slog.String("media_item.id", string(item.ID)),
			slog.Any("err", err))
		return
	}
	se, ok := s.enricher.(SingleItemEnricher)
	if !ok {
		return
	}
	// Detached context with a generous timeout so a TMDB rate-limit retry
	// chain doesn't burn the goroutine forever.
	go func(it *library.MediaItem) {
		defer func() {
			if r := recover(); r != nil {
				// ADR-0007: deliberately detached from the request ctx (see comment
				// above), so no request id survives to correlate here.
				slog.Error("panic in title-override auto-enrich",
					slog.Any("recover", r),
					slog.String("media_item.id", string(it.ID)))
			}
		}()
		enrichCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		se.AutoEnrichItem(enrichCtx, it, slog.Default())
	}(item)
}

func (s *CommandService) RemoveOverride(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.RemoveOverride")
	defer span.End()
	span.SetAttributes(attribute.String("override.id", id))
	// Read first: once deleted, nothing says which title the override was on.
	target, getErr := s.overrides.Get(ctx, id)
	if err := s.overrides.Delete(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("CommandService.RemoveOverride: %w", err)
	}
	if getErr == nil && target != nil {
		s.publishItemUpdated(ctx, target.MediaItemID, library.UpdateReasonMetadataEdited)
	}
	return nil
}

// GetOverride exposes a single override lookup so adapters can resolve
// the media item it targets (used by the gRPC handler to gate Remove
// calls behind a per-library access check without re-implementing the
// repository hop).
func (s *CommandService) GetOverride(ctx context.Context, id string) (*metadata.MetadataOverride, error) {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.GetOverride")
	defer span.End()
	return s.overrides.Get(ctx, id)
}

func (s *CommandService) ResetOverrides(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ResetOverrides")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	if err := s.overrides.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("CommandService.ResetOverrides: %w", err)
	}
	// Log-only: audit event - overrides already deleted above
	evtRemoved := metadata.MetadataOverrideRemoved{MediaItemID: shared.MediaItemID(mediaItemID), FieldName: "*", OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtRemoved); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtRemoved.EventName()), slog.Any("err", err))
	}
	s.publishItemUpdated(ctx, shared.MediaItemID(mediaItemID), library.UpdateReasonMetadataReset)
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
	seasonID := string(seasons[0].ID)
	episodes, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{ParentID: &seasonID, Type: &episodeType, Limit: 1})
	if err != nil || len(episodes) == 0 {
		return ""
	}
	// Extract the series directory name: /media2/It welcome to derry/S01/file.mkv → "It welcome to derry"
	fp := ""
	if episodes[0].File != nil {
		fp = episodes[0].File.Path
	}
	if fp == "" {
		return ""
	}
	dir := filepath.Dir(fp)   // /media2/It welcome to derry/S01
	dir = filepath.Dir(dir)   // /media2/It welcome to derry
	return filepath.Base(dir) // It welcome to derry
}

// ResetItemMetadata clears enriched metadata on an item and all its children
// (seasons + episodes for series, episodes for seasons) then triggers re-enrichment.
func (s *CommandService) ResetItemMetadata(ctx context.Context, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.metadata.ResetItemMetadata")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", mediaItemID))
	if s.mediaItems == nil {
		err := errors.New("media item repository not configured")
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

	// Reset the item itself
	if err := s.resetSingleItem(ctx, item); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("reset item: %w", err)
	}

	// Cascade to children (series → seasons + episodes, season → episodes)
	if item.Type == library.Series || item.Type == library.Season {
		if err := s.resetChildren(ctx, string(item.ID)); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return fmt.Errorf("reset children: %w", err)
		}
	}

	s.publishItemUpdated(ctx, item.ID, library.UpdateReasonMetadataReset)

	// Trigger re-enrichment with background context (request ctx may be done)
	if se, ok := s.enricher.(SingleItemEnricher); ok {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// ADR-0007: request ctx may already be done by the time this
					// fires (see comment above); nothing correlatable to carry.
					slog.Error("panic in metadata re-enrichment", slog.Any("recover", r))
				}
			}()
			enrichCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			se.AutoEnrichItem(enrichCtx, item, slog.Default())
		}()
	}

	return nil
}

// resetSingleItem clears enriched metadata fields, artwork, overrides, and cast
// for one item. Errors propagate: they used to be logged and swallowed, so a
// half-completed reset still answered "ok" to the admin who asked for it.
func (s *CommandService) resetSingleItem(ctx context.Context, item *library.MediaItem) error {
	// Order matters, because these four writes are NOT in one transaction and a
	// failure can land between any two of them. Overrides go FIRST.
	//
	// The other way round - clearing the item's fields before dropping its
	// overrides - has a genuinely bad half-state: a failure after the update
	// leaves the fields blank while the overrides survive, so the next
	// enrichment re-applies exactly what the admin asked to remove. The reset
	// silently undoes itself. Dropping overrides first fails safe instead: the
	// fields are still populated, and re-enrichment converges on the reset
	// state anyway.
	if err := s.overrides.DeleteByMediaItemID(ctx, string(item.ID)); err != nil {
		return fmt.Errorf("delete overrides: %w", err)
	}
	if err := s.artworkCmd.DeleteByMediaItemID(ctx, string(item.ID)); err != nil {
		return fmt.Errorf("delete artwork: %w", err)
	}
	if s.mediaPersons != nil {
		if err := s.mediaPersons.DeleteByMediaItemID(ctx, item.ID); err != nil {
			return fmt.Errorf("delete media persons: %w", err)
		}
	}
	return s.clearFieldsAndUpdate(ctx, item)
}

// clearFieldsAndUpdate derives the title and blanks the enriched fields, then
// persists the item. Split out of resetSingleItem so resetChildren can batch
// the three deletes across an entire level and run only this per-item tail
// (title derivation + Update) afterwards, instead of one DELETE triplet per
// child.
func (s *CommandService) clearFieldsAndUpdate(ctx context.Context, item *library.MediaItem) error {
	// Derive title from file path or directory name
	filePath := ""
	if item.File != nil {
		filePath = item.File.Path
	}
	if filePath != "" {
		item.Title = deriveTitle(filePath, string(item.Type))
	} else if item.Type == library.Series {
		if dirName := s.deriveTitleFromChildren(ctx, string(item.ID)); dirName != "" {
			item.Title = dirName
		}
	}

	item.Overview = ""
	item.Tagline = ""
	item.OriginalTitle = ""
	item.Catalog = nil
	item.Classification = nil
	item.Trailers = nil
	item.UpdatedAt = time.Now()

	if err := s.itemUpdater.Update(ctx, item); err != nil {
		return fmt.Errorf("update item: %w", err)
	}
	return nil
}

// resetChildren resets all direct children and their children recursively.
// The three deletes (overrides, artwork, media persons) are batched across
// the whole level - one statement per type for every child of parentID -
// instead of the old per-item DELETE triplet, which was ~3*(S+S*E) queries
// for a series with S seasons and E episodes/season. The recursion itself
// (one List per season) is unchanged, only the DELETEs are batched.
//
// Stops at the first failure and reports it: a series reset that silently
// gave up halfway is worse than one that says so, because the admin has no
// way to tell which episodes were actually reset.
//
// Same fail-safe ordering as resetSingleItem: overrides for the whole level
// are dropped FIRST, then artwork, then media persons, and only then are the
// items' fields cleared and updated. A failure between any two steps leaves
// the fields still populated with (some of) the overrides already gone, so
// re-enrichment converges on the reset state instead of silently reinstating
// what the admin asked to remove.
func (s *CommandService) resetChildren(ctx context.Context, parentID string) error {
	children, err := s.mediaItems.List(ctx, library.ListMediaItemsFilter{ParentID: &parentID})
	if err != nil {
		return fmt.Errorf("list children of %s: %w", parentID, err)
	}
	if len(children) == 0 {
		return nil
	}

	ids := make([]string, len(children))
	for i, c := range children {
		ids[i] = string(c.ID)
	}

	if err := s.overrides.DeleteByMediaItemIDs(ctx, ids); err != nil {
		return fmt.Errorf("delete overrides for children of %s: %w", parentID, err)
	}
	if err := s.artworkCmd.DeleteByMediaItemIDs(ctx, ids); err != nil {
		return fmt.Errorf("delete artwork for children of %s: %w", parentID, err)
	}
	if s.mediaPersons != nil {
		mediaItemIDs := make([]shared.MediaItemID, len(children))
		for i, c := range children {
			mediaItemIDs[i] = c.ID
		}
		if err := s.mediaPersons.DeleteByMediaItemIDs(ctx, mediaItemIDs); err != nil {
			return fmt.Errorf("delete media persons for children of %s: %w", parentID, err)
		}
	}

	for _, child := range children {
		if err := s.clearFieldsAndUpdate(ctx, child); err != nil {
			return fmt.Errorf("update child %s: %w", child.ID, err)
		}
		// Recurse (series → seasons → episodes)
		if child.Type == library.Season {
			if err := s.resetChildren(ctx, string(child.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}
