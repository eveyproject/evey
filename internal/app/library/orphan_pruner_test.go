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

	removed, err := pruner.Prune(context.Background(), "lib-1", seen)
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

	removed, err := pruner.Prune(context.Background(), "lib-1", seen)
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

func TestOrphanPruner_SearchIndexCleared_ForOrphans(t *testing.T) {
	t.Parallel()
	pruner, items, search, _ := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "orph", LibraryID: "lib-1",
		File:  &library.MediaFile{Path: "/movies/orph.mkv"},
		Title: "Orphan Film",
	})
	search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "orph", Title: "Orphan Film"}, nil)

	seen := map[string]struct{}{} // all orphaned

	_, err := pruner.Prune(context.Background(), "lib-1", seen)
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

	// "seen" includes the container paths so they are not orphaned as files —
	// the pruner removes them because they have no children.
	seen := map[string]struct{}{
		"/tv/show":     {},
		"/tv/show/s01": {},
	}

	removed, err := pruner.Prune(context.Background(), "lib-1", seen)
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

	removed, err := pruner.Prune(context.Background(), "lib-1", seen)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// only season-empty pruned; show-2 kept because season-full has ep-1
	if removed != 1 {
		t.Fatalf("want 1 pruned (season-empty only), got %d", removed)
	}
}

func TestOrphanPruner_DeleteByIDsError_StillReportsCountAndCleansIndex(t *testing.T) {
	t.Parallel()
	pruner, items, search, bus := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "orph", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/orph.mkv"},
	})
	search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "orph", Title: "Orphan"}, nil)
	items.ErrDelete = errors.New("db kaboom")

	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{})
	if err != nil {
		t.Fatalf("Prune must not bubble batch-delete errors: %v", err)
	}
	if removed != 1 {
		t.Fatalf("want 1 reported orphan, got %d", removed)
	}
	// Index/event cleanup runs even when the DB delete fails — pruner is best-effort.
	hits, _, _ := search.Search(context.Background(), "orphan", nil, nil, 10)
	if len(hits) != 0 {
		t.Fatalf("orphan still in search index after batch-delete error: %+v", hits)
	}
	var saw bool
	for _, e := range bus.PublishedEvents() {
		if _, ok := e.(library.MediaItemRemoved); ok {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatalf("MediaItemRemoved should still publish despite batch-delete error")
	}
}

func TestOrphanPruner_SearchRemoveError_LoggedNotReturned(t *testing.T) {
	t.Parallel()
	pruner, items, search, _ := newPrunerHarness()

	items.Create(context.Background(), &library.MediaItem{
		ID: "orph", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/orph.mkv"},
	})
	search.Err = errors.New("meili down")

	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{})
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
	bus.PublishErr = errors.New("bus offline")

	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{})
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
	// nil search is allowed per the Prune contract — pruner skips index ops.
	pruner := applib.NewOrphanPruner(items, nil, bus)

	items.Create(context.Background(), &library.MediaItem{
		ID: "solo", LibraryID: "lib-1",
		File: &library.MediaFile{Path: "/movies/solo.mkv"},
	})

	removed, err := pruner.Prune(context.Background(), "lib-1", map[string]struct{}{})
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
