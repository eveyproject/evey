package library

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

	guard PruneGuard
	// mu guards the in-memory suspension state. A restart forgets it: the next
	// pass re-suspends and re-announces once, which is safe.
	mu        sync.Mutex
	suspended map[string]map[string]PruneSuspension // library -> root -> state
	confirmed map[string]bool                       // library -> admin override armed
}

// PruneGuard bounds how much of a root may vanish in one pass before the
// prune stops deleting under it. A mount that is healthy but lost a branch
// (a union pool missing one disk) makes files disappear for real from the
// root's view, which the empty-root checks cannot tell from a deletion.
type PruneGuard struct {
	// MaxMissingRatio is the share of a root's items (0.05 = 5 %) that may be
	// absent in one pass; above it, and past MinMissing, the root is held.
	MaxMissingRatio float64
	// MinMissing is the absolute floor: fewer missing files never suspend,
	// so a small library can still lose a few titles.
	MinMissing int
}

// DefaultPruneGuard is 5 % and 20 files.
var DefaultPruneGuard = PruneGuard{MaxMissingRatio: 0.05, MinMissing: 20}

// PruneSuspension is a root whose deletions are held back.
type PruneSuspension struct {
	LibraryID string
	Root      string
	Missing   int
	Total     int
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
		guard:      DefaultPruneGuard,
		suspended:  make(map[string]map[string]PruneSuspension),
		confirmed:  make(map[string]bool),
	}
}

// WithGuard overrides the mass-disappearance thresholds.
func (p *OrphanPruner) WithGuard(g PruneGuard) *OrphanPruner {
	p.guard = g
	return p
}

// Suspensions lists the roots of a library whose deletions are held, by root.
func (p *OrphanPruner) Suspensions(libraryID string) []PruneSuspension {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]PruneSuspension, 0, len(p.suspended[libraryID]))
	for _, s := range p.suspended[libraryID] {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Root < out[j].Root })
	return out
}

// AllSuspensions lists the held roots of every library.
func (p *OrphanPruner) AllSuspensions() []PruneSuspension {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []PruneSuspension
	for _, roots := range p.suspended {
		for _, s := range roots {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LibraryID != out[j].LibraryID {
			return out[i].LibraryID < out[j].LibraryID
		}
		return out[i].Root < out[j].Root
	})
	return out
}

// ConfirmPrune arms a one-shot override: the next pass of the library deletes
// under the suspended roots whatever the ratio. It reports false, arming
// nothing, when no root of the library is suspended.
func (p *OrphanPruner) ConfirmPrune(libraryID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.suspended[libraryID]) == 0 {
		return false
	}
	p.confirmed[libraryID] = true
	return true
}

// rootOf returns the most specific of roots containing path, "" when none.
func rootOf(path string, roots []string) string {
	best := ""
	for _, root := range roots {
		if pathUnderAnyRoot(path, []string{root}) && len(filepath.Clean(root)) > len(best) {
			best = filepath.Clean(root)
		}
	}
	return best
}

// holdSuspendedRoots drops from orphanIDs every candidate under a root where
// the missing share is past the guard, unless an admin confirmed; it returns
// the kept candidates and publishes the event of each newly suspended root.
func (p *OrphanPruner) holdSuspendedRoots(ctx context.Context, libraryID string, roots []string, dbPaths map[string]string, orphanIDs []string) []string {
	if len(roots) == 0 {
		return orphanIDs
	}
	total := map[string]int{}
	for _, path := range dbPaths {
		if r := rootOf(path, roots); r != "" {
			total[r]++
		}
	}
	missing := map[string]int{}
	for _, id := range orphanIDs {
		if r := rootOf(dbPaths[id], roots); r != "" {
			missing[r]++
		}
	}

	p.mu.Lock()
	override := p.confirmed[libraryID]
	delete(p.confirmed, libraryID)
	previous := p.suspended[libraryID]
	next := make(map[string]PruneSuspension)
	var announce []PruneSuspension
	for root, m := range missing {
		t := total[root]
		if override || m < p.guard.MinMissing || float64(m) <= p.guard.MaxMissingRatio*float64(t) {
			continue
		}
		s := PruneSuspension{LibraryID: libraryID, Root: root, Missing: m, Total: t}
		next[root] = s
		if _, already := previous[root]; !already {
			announce = append(announce, s)
		}
	}
	if len(next) == 0 {
		delete(p.suspended, libraryID)
	} else {
		p.suspended[libraryID] = next
	}
	p.mu.Unlock()

	sort.Slice(announce, func(i, j int) bool { return announce[i].Root < announce[j].Root })
	for _, s := range announce {
		slog.WarnContext(ctx, "orphan prune suspended: too many files missing under a root (a mount missing a branch?); admin confirmation needed to delete",
			slog.String("library_id", libraryID), slog.String("root", s.Root),
			slog.Int("missing", s.Missing), slog.Int("total", s.Total))
		evt := library.PruneSuspended{LibraryID: shared.LibraryID(libraryID), Root: s.Root, Missing: s.Missing, Total: s.Total, OccurredAt: time.Now()}
		if pubErr := p.events.Publish(ctx, evt); pubErr != nil {
			slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", pubErr))
		}
	}
	if len(next) == 0 {
		return orphanIDs
	}
	kept := orphanIDs[:0:0]
	for _, id := range orphanIDs {
		if _, held := next[rootOf(dbPaths[id], roots)]; !held {
			kept = append(kept, id)
		}
	}
	return kept
}

// Prune deletes media items whose paths are not in seenPaths and
// removes empty seasons/series left behind. roots are the library's
// configured paths: under a root where a large share of the items is absent
// at once, nothing is deleted until an admin confirms (see PruneGuard). Returns total count of
// deleted rows (orphans + pruned containers). Errors are logged, not
// returned, except a fatal failure to list DB paths up-front.
func (p *OrphanPruner) Prune(ctx context.Context, libraryID string, roots []string, seenPaths map[string]struct{}, emptyRoots []string) (int, error) {
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
	orphanIDs = p.holdSuspendedRoots(ctx, libraryID, roots, dbPaths, orphanIDs)
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
