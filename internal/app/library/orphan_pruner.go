package library

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// OrphanPruner finalizes a scan by removing DB rows whose files no
// longer exist on disk (orphans) and cleaning up empty TV containers
// (seasons/series with no remaining children). Pure finalization —
// runs after the scan has collected every path it saw this pass.
type OrphanPruner struct {
	mediaItems library.MediaItemCommandRepo
	search     library.SearchIndex
	events     shared.EventBus
}

func NewOrphanPruner(
	mediaItems library.MediaItemCommandRepo,
	search library.SearchIndex,
	events shared.EventBus,
) *OrphanPruner {
	return &OrphanPruner{
		mediaItems: mediaItems,
		search:     search,
		events:     events,
	}
}

// Prune deletes media items whose paths are not in seenPaths and
// removes empty seasons/series left behind. Returns total count of
// deleted rows (orphans + pruned containers). Errors are logged, not
// returned, except a fatal failure to list DB paths up-front.
func (p *OrphanPruner) Prune(ctx context.Context, libraryID string, seenPaths map[string]struct{}) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.OrphanPruner.Prune")
	defer span.End()

	dbPaths, err := p.mediaItems.ListFilePaths(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, fmt.Errorf("list file paths: %w", err)
	}
	var orphanIDs []string
	for id, path := range dbPaths {
		if _, seen := seenPaths[path]; !seen {
			orphanIDs = append(orphanIDs, id)
		}
	}
	if len(orphanIDs) > 0 {
		if delErr := p.mediaItems.DeleteByIDs(ctx, orphanIDs); delErr != nil {
			slog.Warn("batch delete orphaned media items failed", slog.Any("err", delErr))
		}
		for _, id := range orphanIDs {
			if p.search != nil {
				if idxErr := p.search.RemoveMediaItem(ctx, id); idxErr != nil {
					slog.Warn("remove orphan from search index failed", slog.String("id", id), slog.Any("err", idxErr))
				}
			}
			evt := library.MediaItemRemoved{
				MediaItemID: shared.MediaItemID(id),
				LibraryID:   shared.LibraryID(libraryID),
				OccurredAt:  time.Now(),
			}
			if pubErr := p.events.Publish(ctx, evt); pubErr != nil {
				slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", pubErr))
			}
		}
	}

	pruned, pruneErr := p.pruneEmptyContainers(ctx, libraryID)
	if pruneErr != nil {
		slog.Warn("prune empty containers failed", slog.Any("err", pruneErr))
	}

	return len(orphanIDs) + pruned, nil
}

// pruneEmptyContainers deletes Season rows with zero children then
// Series rows left empty. Two-pass so deleting a season's only child
// can cascade into deleting the parent series in the same call.
func (p *OrphanPruner) pruneEmptyContainers(ctx context.Context, libraryID string) (int, error) {
	all, err := p.mediaItems.ListContainerInfo(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, err
	}
	childCount := make(map[string]int)
	for _, item := range all {
		if item.ParentID != "" {
			childCount[item.ParentID]++
		}
	}
	pruned := 0
	for _, item := range all {
		if item.Type != string(library.Season) {
			continue
		}
		if childCount[item.ID] > 0 {
			continue
		}
		if dErr := p.mediaItems.Delete(ctx, shared.MediaItemID(item.ID)); dErr != nil {
			slog.Warn("delete empty season", slog.String("id", item.ID), slog.Any("err", dErr))
			continue
		}
		childCount[item.ParentID]--
		pruned++
	}
	for _, item := range all {
		if item.Type != string(library.Series) {
			continue
		}
		if childCount[item.ID] > 0 {
			continue
		}
		if dErr := p.mediaItems.Delete(ctx, shared.MediaItemID(item.ID)); dErr != nil {
			slog.Warn("delete empty series", slog.String("id", item.ID), slog.Any("err", dErr))
			continue
		}
		if p.search != nil {
			if rmErr := p.search.RemoveMediaItem(ctx, item.ID); rmErr != nil {
				slog.Warn("remove empty series from search index", slog.String("id", item.ID), slog.Any("err", rmErr))
			}
		}
		pruned++
	}
	return pruned, nil
}
