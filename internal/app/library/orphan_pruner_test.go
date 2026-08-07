package library_test

import (
	"context"
	"errors"
	"testing"

	applib "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

func newPrunerHarness() (*applib.OrphanPruner, *libfakes.FakeMediaItemCommandRepo, *libfakes.FakeSearchIndex, *sharedfakes.FakeEventBus) {
	items := libfakes.NewFakeMediaItemCommandRepo()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()
	return applib.NewOrphanPruner(items, search, bus), items, search, bus
}

func TestOrphanPruner_AllPresent_NothingRemoved(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "a", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/a.mkv"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "b", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/b.mkv"},
	})

	seen := map[string]struct{}{
		"/movies/a.mkv": {},
		"/movies/b.mkv": {},
	}

	removed, err := pruner.Prune(context.Background(), "lib-1", seen, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("want 0 removed, got %d", removed)
	}
}

func TestOrphanPruner_PartialOrphans(t *testing.T) {
	t.Parallel()
	pruner, items, _, bus := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "keep", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/keep.mkv"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "gone", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/gone.mkv"},
	})

	seen := map[string]struct{}{"/movies/keep.mkv": {}}

	removed, err := pruner.Prune(context.Background(), "lib-1", seen, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("want 1 removed, got %d", removed)
	}

	var rm *library.MediaItemRemoved
	for _, e := range bus.PublishedEvents() {
		if r, ok := e.(library.MediaItemRemoved); ok {
			rm = &r
			break
		}
	}
	if rm == nil || rm.MediaItemID != "gone" {
		t.Fatalf("want MediaItemRemoved for gone, got %+v", rm)
	}
}

// Safety guard: a scan that saw ZERO files while the library still has items
// is a degraded mount, not a real wipe. Prune must refuse to delete the whole
// catalog (which would CASCADE-delete non-regenerable user data).
func TestOrphanPruner_EmptyScanNonEmptyLibrary_RefusesToPrune(t *testing.T) {
	t.Parallel()
	pruner, items, _, bus := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "a", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/a.mkv"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "b", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/b.mkv"},
	})

	// Empty seenPaths == the scanner saw nothing (dropped NFS mount, etc.).
	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("guard must delete nothing, got %d removed", removed)
	}
	for _, e := range bus.PublishedEvents() {
		if _, ok := e.(library.MediaItemRemoved); ok {
			t.Fatal("no MediaItemRemoved must be published when the prune is refused")
		}
	}
	// Both items must still be in the DB.
	paths, err := items.ListFilePaths(context.Background(), "lib-1")
	if err != nil {
		t.Fatalf("list paths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("guard must keep all items, got %d", len(paths))
	}
}

// Per-root guard: a multi-path library where ONE root scanned empty (a dropped
// mount) keeps seenPaths non-empty via the other healthy root, so the aggregate
// guard does not fire. Items under the dropped root must still NOT be pruned,
// while a genuine orphan under the healthy root is.
func TestOrphanPruner_OneRootDropped_KeepsItemsUnderItButPrunesHealthyOrphan(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()

	// Healthy root: a.mkv still present, gone.mkv genuinely deleted.
	items.Create(context.Background(), &library.MediaItem{
		ID: "a", LibraryID: "lib-1", File: &library.MediaFile{Path: "/mnt/healthy/a.mkv"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "gone", LibraryID: "lib-1", File: &library.MediaFile{Path: "/mnt/healthy/gone.mkv"},
	})
	// Dropped root: b.mkv unseen because the mount fell over (root in emptyRoots).
	items.Create(context.Background(), &library.MediaItem{
		ID: "b", LibraryID: "lib-1", File: &library.MediaFile{Path: "/mnt/dropped/b.mkv"},
	})

	seen := map[string]struct{}{"/mnt/healthy/a.mkv": {}}
	emptyRoots := []string{"/mnt/dropped"}

	removed, err := pruner.Prune(context.Background(), "lib-1", seen, emptyRoots)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("only the healthy-root orphan should be pruned, got %d", removed)
	}
	paths, _ := items.ListFilePaths(context.Background(), "lib-1")
	got := map[string]bool{}
	for _, p := range paths {
		got[p] = true
	}
	if !got["/mnt/healthy/a.mkv"] {
		t.Error("present item must remain")
	}
	if !got["/mnt/dropped/b.mkv"] {
		t.Error("item under the dropped mount must NOT be pruned")
	}
	if got["/mnt/healthy/gone.mkv"] {
		t.Error("genuine orphan under the healthy root must be pruned")
	}
}

// A genuinely empty library (no DB items) with an empty scan is unaffected by
// the guard: nothing to delete, no error.
func TestOrphanPruner_EmptyScanEmptyLibrary_NoOp(t *testing.T) {
	t.Parallel()
	pruner, _, _, _ := newPrunerHarness()
	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("want 0 removed, got %d", removed)
	}
}

func TestOrphanPruner_SearchIndexCleared_ForOrphans(t *testing.T) {
	t.Parallel()
	pruner, items, search, _ := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "orph", LibraryID: "lib-1",
		File:  &library.MediaFile{Path: "/movies/orph.mkv"},
		Title: "Orphan Film",
	})
	// A kept item so seenPaths is non-empty (the empty-scan safety guard would
	// otherwise refuse to prune); only "orph" is the actual orphan here.
	items.Create(context.Background(), &library.MediaItem{
		ID: "kept", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/kept.mkv"},
	})
	search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "orph", Title: "Orphan Film"})

	seen := map[string]struct{}{"/movies/kept.mkv": {}} // orph is the orphan

	_, err := pruner.Prune(context.Background(), "lib-1", seen, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hits, _, _ := search.Search(context.Background(), "orphan", nil, nil, 10)
	if len(hits) != 0 {
		t.Fatalf("orphan still in index: %+v", hits)
	}
}

func TestOrphanPruner_EmptySeasonsPruned(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()

	// Series + season with no episodes → both prune
	items.Create(context.Background(), &library.MediaItem{
		ID: "show-1", LibraryID: "lib-1", Type: library.Series,
		File: &library.MediaFile{Path: "/tv/show"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "season-1", LibraryID: "lib-1", ParentID: "show-1", Type: library.Season,
		File: &library.MediaFile{Path: "/tv/show/s01"},
	})

	// "seen" includes the container paths so they are not orphaned as files -
	// the pruner removes them because they have no children.
	seen := map[string]struct{}{
		"/tv/show":     {},
		"/tv/show/s01": {},
	}

	removed, err := pruner.Prune(context.Background(), "lib-1", seen, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 2 {
		t.Fatalf("want 2 pruned (empty season + empty series), got %d", removed)
	}
}

func TestOrphanPruner_SeriesWithRemainingSeason_NotPruned(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "show-2", LibraryID: "lib-1", Type: library.Series,
		File: &library.MediaFile{Path: "/tv/other"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "season-empty", LibraryID: "lib-1", ParentID: "show-2", Type: library.Season,
		File: &library.MediaFile{Path: "/tv/other/s01"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "season-full", LibraryID: "lib-1", ParentID: "show-2", Type: library.Season,
		File: &library.MediaFile{Path: "/tv/other/s02"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "ep-1", LibraryID: "lib-1", ParentID: "season-full", Type: library.Episode,
		File: &library.MediaFile{Path: "/tv/other/s02/e01.mkv"},
	})

	seen := map[string]struct{}{
		"/tv/other":             {},
		"/tv/other/s01":         {},
		"/tv/other/s02":         {},
		"/tv/other/s02/e01.mkv": {},
	}

	removed, err := pruner.Prune(context.Background(), "lib-1", seen, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// only season-empty pruned; show-2 kept because season-full has ep-1
	if removed != 1 {
		t.Fatalf("want 1 pruned (season-empty only), got %d", removed)
	}
}

func TestOrphanPruner_DeleteByIDsError_FailClosed(t *testing.T) {
	t.Parallel()
	pruner, items, search, bus := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "orph", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/orph.mkv"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "kept", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/kept.mkv"},
	})
	search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "orph", Title: "Orphan"})
	items.ErrDelete = errors.New("db kaboom")

	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{"/movies/kept.mkv": {}}, nil)
	if err != nil {
		t.Fatalf("Prune must not bubble batch-delete errors: %v", err)
	}
	// Nothing was actually deleted - count must reflect reality.
	if removed != 0 {
		t.Fatalf("want 0 removed when delete fails, got %d", removed)
	}
	// Row still in DB - must NOT be de-indexed (search still returns it).
	hits, _, _ := search.Search(context.Background(), "orphan", nil, nil, 10)
	if len(hits) == 0 {
		t.Fatal("orphan must remain in search index when DB delete failed")
	}
	// Must NOT emit MediaItemRemoved for an item still in the DB.
	for _, e := range bus.PublishedEvents() {
		if _, ok := e.(library.MediaItemRemoved); ok {
			t.Fatal("MediaItemRemoved must not be published when DB delete failed")
		}
	}
}

func TestOrphanPruner_SearchRemoveError_LoggedNotReturned(t *testing.T) {
	t.Parallel()
	pruner, items, search, _ := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "orph", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/orph.mkv"},
	})
	// Kept item so seenPaths is non-empty (avoids the empty-scan safety guard).
	items.Create(context.Background(), &library.MediaItem{
		ID: "kept", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/kept.mkv"},
	})
	search.Err = errors.New("meili down")

	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{"/movies/kept.mkv": {}}, nil)
	if err != nil {
		t.Fatalf("search index errors must not fail Prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("want 1 removed, got %d", removed)
	}
}

func TestOrphanPruner_PublishError_LoggedNotReturned(t *testing.T) {
	t.Parallel()
	pruner, items, _, bus := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "orph", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/orph.mkv"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "kept", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/kept.mkv"},
	})
	bus.PublishErr = errors.New("bus offline")

	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{"/movies/kept.mkv": {}}, nil)
	if err != nil {
		t.Fatalf("event bus errors must not fail Prune: %v", err)
	}
	if removed != 1 {
		t.Fatalf("want 1 removed, got %d", removed)
	}
}

func TestOrphanPruner_NilSearchIndex_StillPrunesAndPublishes(t *testing.T) {
	t.Parallel()
	items := libfakes.NewFakeMediaItemCommandRepo()
	bus := sharedfakes.NewFakeEventBus()
	// nil search is allowed per the Prune contract - pruner skips index ops.
	pruner := applib.NewOrphanPruner(items, nil, bus)

	items.Create(context.Background(), &library.MediaItem{
		ID: "solo", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/solo.mkv"},
	})
	items.Create(context.Background(), &library.MediaItem{
		ID: "kept", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/kept.mkv"},
	})

	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{"/movies/kept.mkv": {}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("want 1 removed, got %d", removed)
	}
	var rm *library.MediaItemRemoved
	for _, e := range bus.PublishedEvents() {
		if r, ok := e.(library.MediaItemRemoved); ok {
			rm = &r
		}
	}
	if rm == nil || rm.MediaItemID != shared.MediaItemID("solo") {
		t.Fatalf("want MediaItemRemoved for solo, got %+v", rm)
	}
}
