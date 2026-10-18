package library

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// pathUnderAnyRoot reports whether path lies within one of the given roots
// (equal to a root or nested beneath it), separator-aware so "/a/bc" is not
// considered under "/a/b". Empty roots → false.
func pathUnderAnyRoot(path string, roots []string) bool {
	if len(roots) == 0 {
		return false
	}
	clean := filepath.Clean(path)
	for _, root := range roots {
		r := filepath.Clean(root)
		if clean == r || strings.HasPrefix(clean, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// OrphanPruner finalizes a scan by removing DB rows whose files no
// longer exist on disk (orphans) and cleaning up empty TV containers
// (seasons/series with no remaining children). Pure finalization -
// runs after the scan has collected every path it saw this pass.
type OrphanPruner struct {
	mediaItems library.MediaItemCommandRepo
	search     library.SearchIndex
	events     shared.EventBus
	files      fileChecker
}

// fileChecker re-checks a prune candidate on disk before its row goes.
type fileChecker interface {
	FileExists(path string) bool
}

func NewOrphanPruner(
	mediaItems library.MediaItemCommandRepo,
	search library.SearchIndex,
	events shared.EventBus,
	files fileChecker,
) *OrphanPruner {
	return &OrphanPruner{
		mediaItems: mediaItems,
		search:     search,
		events:     events,
		files:      files,
	}
}

// Prune deletes media items whose paths are not in seenPaths and
// removes empty seasons/series left behind. Returns total count of
// deleted rows (orphans + pruned containers). Errors are logged, not
// returned, except a fatal failure to list DB paths up-front.
func (p *OrphanPruner) Prune(ctx context.Context, libraryID string, seenPaths map[string]struct{}, emptyRoots []string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.OrphanPruner.Prune")
	defer span.End()

	dbPaths, err := p.mediaItems.ListFilePaths(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, fmt.Errorf("list file paths: %w", err)
	}

	// Safety net against a degraded mount wiping the catalog. A scan that saw
	// ZERO files while the library still has items is almost never a real "all
	// media deleted" - it is a dropped NFS/SMB export, a lazy/empty mountpoint,
	// or the media folder renamed under a healthy root (the scanner only errors
	// when the root itself is inaccessible, so an empty-but-present dir returns
	// no files and no error). Pruning here would DeleteByIDs every row and
	// CASCADE-delete non-regenerable user data (watch_progress, favorites,
	// play_sessions, playlist_items). Refuse. Also covers a library with no
	// configured paths. A genuinely emptied library has len(dbPaths)==0.
	if len(seenPaths) == 0 && len(dbPaths) > 0 {
		slog.WarnContext(ctx, "orphan prune skipped: scan saw no files but library is non-empty (likely a degraded mount); refusing to delete the catalog",
			slog.String("library_id", libraryID), slog.Int("db_items", len(dbPaths)))
		return 0, nil
	}

	var orphanIDs []string
	skippedDegraded := 0
	for id, path := range dbPaths {
		if _, seen := seenPaths[path]; seen {
			continue
		}
		// Per-root extension of the guard above: on a multi-path library, one
		// healthy root keeps seenPaths non-empty, but an item under a DIFFERENT
		// root that scanned empty (that mount dropped) must NOT be pruned - same
		// data-loss, narrower trigger. An item whose file genuinely vanished from
		// a HEALTHY root (one that returned other files) is still a real orphan.
		if pathUnderAnyRoot(path, emptyRoots) {
			skippedDegraded++
			continue
		}
		// The walk skips a folder it cannot read without failing the scan, so
		// "not seen" alone is not "deleted": the file must be confirmed gone
		// on disk, as the music and photo prunes already do.
		if p.files != nil && p.files.FileExists(path) {
			skippedDegraded++
			continue
		}
		orphanIDs = append(orphanIDs, id)
	}
	if skippedDegraded > 0 {
		slog.WarnContext(ctx, "orphan prune skipped items the scan did not see but that are not gone from disk (degraded mount or unreadable folder)",
			slog.String("library_id", libraryID), slog.Int("skipped", skippedDegraded))
	}
	deleted := 0
	if len(orphanIDs) > 0 {
		if delErr := p.mediaItems.DeleteByIDs(ctx, orphanIDs); delErr != nil {
			slog.WarnContext(ctx, "batch delete orphaned media items failed", slog.Any("err", delErr))
			// Fail closed: rows are still in the DB - do not de-index or emit
			// removal events for items that were NOT deleted.
		} else {
			deleted = len(orphanIDs)
			for _, id := range orphanIDs {
				if p.search != nil {
					if idxErr := p.search.RemoveMediaItem(ctx, id); idxErr != nil {
						slog.WarnContext(ctx, "remove orphan from search index failed", slog.String("id", id), slog.Any("err", idxErr))
					}
				}
				evt := library.MediaItemRemoved{
					MediaItemID: shared.MediaItemID(id),
					LibraryID:   shared.LibraryID(libraryID),
					OccurredAt:  time.Now(),
				}
				if pubErr := p.events.Publish(ctx, evt); pubErr != nil {
					slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", pubErr))
				}
			}
		}
	}

	pruned, pruneErr := p.pruneEmptyContainers(ctx, libraryID)
	if pruneErr != nil {
		slog.WarnContext(ctx, "prune empty containers failed", slog.Any("err", pruneErr))
	}

	return deleted + pruned, nil
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
			slog.WarnContext(ctx, "delete empty season", slog.String("id", item.ID), slog.Any("err", dErr))
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
			slog.WarnContext(ctx, "delete empty series", slog.String("id", item.ID), slog.Any("err", dErr))
			continue
		}
		if p.search != nil {
			if rmErr := p.search.RemoveMediaItem(ctx, item.ID); rmErr != nil {
				slog.WarnContext(ctx, "remove empty series from search index", slog.String("id", item.ID), slog.Any("err", rmErr))
			}
		}
		pruned++
	}
	return pruned, nil
}
