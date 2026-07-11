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

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	override.CreatedAt = time.Now()
	if err := s.overrides.Set(ctx, override); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("CommandService.SetOverride: %w", err)
	}
	// Log-only: audit event — override already persisted above
	evtOverride := metadata.MetadataOverrideSet{MediaItemID: override.MediaItemID, FieldName: override.FieldName, EditedBy: override.EditedBy, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtOverride); err != nil {
		slog.Warn("publish event", slog.String("event", evtOverride.EventName()), slog.Any("err", err))
	}
	// When an admin edits the title on an orphan item (no TMDB/IMDB IDs yet),
	// the intent is "this is the correct title — please match it now". The
	// enricher reads media_items.title for its search query, so propagate
	// the override into the raw title and kick a background auto-enrich.
	// Without this hook, fixing a parser-polluted title (e.g. stripping
	// "720" or "arte") via the UI only updated the display layer; match-
	// orphans kept failing because the search used the still-dirty title.
	s.propagateTitleOverrideForOrphan(ctx, override)
	return nil
}

// propagateTitleOverrideForOrphan is the orphan-recovery path for title
// edits. Best-effort: every step that fails just logs and bails — the
// override write itself succeeded above, so we never want to return an
// error from this side-effect.
func (s *CommandService) propagateTitleOverrideForOrphan(ctx context.Context, override *metadata.MetadataOverride) {
	if s.mediaItems == nil || s.enricher == nil {
		return
	}
	// Only the title (or original_title as a fallback) is search-relevant.
	// Other fields (overview, tagline, year) don't change TMDB match outcomes.
	// Field names match what MediaDetailEdit writes (snake_case for
	// original_title — see frontend FIELD_MAP and library_handler.go:986).
	if override.FieldName != "title" && override.FieldName != "original_title" {
		return
	}
	// CustomValue is any so the same override row can carry numerics,
	// dates, etc. for other fields — but title/originalTitle must be a
	// non-empty string for the propagation path to make sense.
	val, ok := override.CustomValue.(string)
	if !ok || val == "" {
		return
	}
	item, err := s.mediaItems.GetByID(ctx, override.MediaItemID)
	if err != nil || item == nil {
		return
	}
	// Only propagate for orphans — a matched item's title edit is
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
	if err := s.mediaItems.Update(ctx, item); err != nil {
		slog.Warn("propagate title override to media_items failed",
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
	if err := s.overrides.Delete(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("CommandService.RemoveOverride: %w", err)
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
	// Log-only: audit event — overrides already deleted above
	evtRemoved := metadata.MetadataOverrideRemoved{MediaItemID: shared.MediaItemID(mediaItemID), FieldName: "*", OccurredAt: time.Now()}
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
	s.resetSingleItem(ctx, item)

	// Cascade to children (series → seasons + episodes, season → episodes)
	if item.Type == library.Series || item.Type == library.Season {
		s.resetChildren(ctx, string(item.ID))
	}

	// Trigger re-enrichment with background context (request ctx may be done)
	if se, ok := s.enricher.(SingleItemEnricher); ok {
		go func() {
			defer func() {
				if r := recover(); r != nil {
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

// resetSingleItem clears enriched metadata fields, artwork, overrides, and cast for one item.
func (s *CommandService) resetSingleItem(ctx context.Context, item *library.MediaItem) {
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

	if err := s.mediaItems.Update(ctx, item); err != nil {
		slog.Warn("reset: update item", slog.String("id", string(item.ID)), slog.Any("err", err))
	}
	if err := s.artwork.DeleteByMediaItemID(ctx, string(item.ID)); err != nil {
		slog.Warn("reset: delete artwork", slog.String("id", string(item.ID)), slog.Any("err", err))
	}
	if err := s.overrides.DeleteByMediaItemID(ctx, string(item.ID)); err != nil {
		slog.Warn("reset: delete overrides", slog.String("id", string(item.ID)), slog.Any("err", err))
	}
	if s.mediaPersons != nil {
		if err := s.mediaPersons.DeleteByMediaItemID(ctx, shared.MediaItemID(item.ID)); err != nil {
			slog.Warn("reset: delete media persons", slog.String("id", string(item.ID)), slog.Any("err", err))
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
		if child.Type == library.Season {
			s.resetChildren(ctx, string(child.ID))
		}
	}
}
