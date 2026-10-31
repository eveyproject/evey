package library_test

import (
	"context"
	"fmt"
	"testing"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// seedUnder creates n items "<root>/f<i>.mkv" and returns their paths.
func seedUnder(t *testing.T, items *libfakes.FakeMediaItemCommandRepo, root string, n int) []string {
	t.Helper()
	paths := make([]string, 0, n)
	for i := 0; i < n; i++ {
		p := fmt.Sprintf("%s/f%d.mkv", root, i)
		if err := items.Create(context.Background(), &library.MediaItem{
			ID: shared.MediaItemID(fmt.Sprintf("%s-%d", root, i)), LibraryID: "lib-1",
			File: &library.MediaFile{Path: p},
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		paths = append(paths, p)
	}
	return paths
}

func seen(paths ...[]string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, ps := range paths {
		for _, p := range ps {
			m[p] = struct{}{}
		}
	}
	return m
}

func suspendedEvents(bus *sharedfakes.FakeEventBus) []library.PruneSuspended {
	var out []library.PruneSuspended
	for _, e := range bus.PublishedEvents() {
		if s, ok := e.(library.PruneSuspended); ok {
			out = append(out, s)
		}
	}
	return out
}

func TestOrphanPruner_BranchMissing_SuspendsRootAndNotifiesOnce(t *testing.T) {
	t.Parallel()
	pruner, items, _, bus := newPrunerHarness()
	ctx := context.Background()
	roots := []string{"/pool", "/other"}
	pool := seedUnder(t, items, "/pool", 90)
	other := seedUnder(t, items, "/other", 10)

	// A third of the pool vanished (one branch disk gone), /other is intact.
	s := seen(pool[:60], other)
	removed, err := pruner.Prune(ctx, "lib-1", roots, s, nil)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 0 {
		t.Fatalf("want nothing deleted, got %d", removed)
	}
	if got := len(items.All()); got != 100 {
		t.Fatalf("want 100 items kept, got %d", got)
	}
	evs := suspendedEvents(bus)
	if len(evs) != 1 || evs[0].Root != "/pool" || evs[0].Missing != 30 || evs[0].Total != 90 || evs[0].LibraryID != "lib-1" {
		t.Fatalf("want one PruneSuspended for /pool 30/90, got %+v", evs)
	}

	// Same state on the next pass: still suspended, not announced again.
	if _, err := pruner.Prune(ctx, "lib-1", roots, s, nil); err != nil {
		t.Fatalf("prune 2: %v", err)
	}
	if n := len(suspendedEvents(bus)); n != 1 {
		t.Fatalf("want still 1 event, got %d", n)
	}
	sus := pruner.Suspensions("lib-1")
	if len(sus) != 1 || sus[0].Root != "/pool" || sus[0].Missing != 30 || sus[0].Total != 90 {
		t.Fatalf("want /pool suspended, got %+v", sus)
	}
}

func TestOrphanPruner_FewDeletions_PrunedNormally(t *testing.T) {
	t.Parallel()
	pruner, items, _, bus := newPrunerHarness()
	paths := seedUnder(t, items, "/pool", 1000)

	removed, err := pruner.Prune(context.Background(), "lib-1", []string{"/pool"}, seen(paths[3:]), nil)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 3 {
		t.Fatalf("want 3 removed, got %d", removed)
	}
	if n := len(suspendedEvents(bus)); n != 0 {
		t.Fatalf("want no event, got %d", n)
	}
	if len(pruner.Suspensions("lib-1")) != 0 {
		t.Fatalf("want nothing suspended")
	}
}

func TestOrphanPruner_BelowAbsoluteMinimum_PrunedEvenAboveRatio(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()
	paths := seedUnder(t, items, "/pool", 10)

	// 5 of 10 gone is 50 % but under the 20-file floor: a tiny library
	// legitimately loses that many.
	removed, err := pruner.Prune(context.Background(), "lib-1", []string{"/pool"}, seen(paths[5:]), nil)
	if err != nil || removed != 5 {
		t.Fatalf("want 5 removed, got %d err=%v", removed, err)
	}
}

func TestOrphanPruner_EmptyRoot_StillKeptWithoutEvent(t *testing.T) {
	t.Parallel()
	pruner, items, _, bus := newPrunerHarness()
	roots := []string{"/a", "/b"}
	a := seedUnder(t, items, "/a", 50)
	seedUnder(t, items, "/b", 50)

	removed, err := pruner.Prune(context.Background(), "lib-1", roots, seen(a), []string{"/b"})
	if err != nil || removed != 0 {
		t.Fatalf("want 0 removed, got %d err=%v", removed, err)
	}
	if len(items.All()) != 100 {
		t.Fatalf("items were deleted")
	}
	if n := len(suspendedEvents(bus)); n != 0 {
		t.Fatalf("want no event for an empty root, got %d", n)
	}
}

func TestOrphanPruner_RatioBackUnderThreshold_ResumesAndReannounces(t *testing.T) {
	t.Parallel()
	pruner, items, _, bus := newPrunerHarness()
	ctx := context.Background()
	roots := []string{"/pool"}
	pool := seedUnder(t, items, "/pool", 100)

	// Suspended: 40 missing.
	if _, err := pruner.Prune(ctx, "lib-1", roots, seen(pool[:60]), nil); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(pruner.Suspensions("lib-1")) != 1 {
		t.Fatalf("want suspended")
	}

	// Branch is back, only 2 files really gone: prune resumes, state clears.
	removed, err := pruner.Prune(ctx, "lib-1", roots, seen(pool[2:]), nil)
	if err != nil || removed != 2 {
		t.Fatalf("want 2 removed, got %d err=%v", removed, err)
	}
	if len(pruner.Suspensions("lib-1")) != 0 {
		t.Fatalf("want state cleared")
	}

	// Suspended anew: announced again.
	left := items.All()
	if len(left) != 98 {
		t.Fatalf("want 98 left, got %d", len(left))
	}
	if _, err := pruner.Prune(ctx, "lib-1", roots, seen(pool[2:50]), nil); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n := len(suspendedEvents(bus)); n != 2 {
		t.Fatalf("want 2 events across two suspensions, got %d", n)
	}
}

func TestOrphanPruner_Confirm_PrunesNextPassOnce(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()
	ctx := context.Background()
	roots := []string{"/pool"}
	pool := seedUnder(t, items, "/pool", 100)
	s := seen(pool[:60])

	if _, err := pruner.Prune(ctx, "lib-1", roots, s, nil); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if pruner.ConfirmPrune("lib-2") {
		t.Fatalf("a library with nothing suspended must not arm")
	}
	if !pruner.ConfirmPrune("lib-1") {
		t.Fatalf("want confirm accepted")
	}
	removed, err := pruner.Prune(ctx, "lib-1", roots, s, nil)
	if err != nil || removed != 40 {
		t.Fatalf("want 40 removed after confirm, got %d err=%v", removed, err)
	}
	if len(pruner.Suspensions("lib-1")) != 0 {
		t.Fatalf("want state cleared after override")
	}

	// One-shot: a new mass disappearance is suspended again.
	more := seedUnder(t, items, "/pool2", 100)
	roots = []string{"/pool", "/pool2"}
	if _, err := pruner.Prune(ctx, "lib-1", roots, seen(pool[:60], more[:50]), nil); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(items.All()) != 160 {
		t.Fatalf("override was not consumed: %d items left", len(items.All()))
	}
}

func TestOrphanPruner_WithGuard_Overrides(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()
	pruner.WithGuard(applib.PruneGuard{MaxMissingRatio: 0.5, MinMissing: 1})
	paths := seedUnder(t, items, "/pool", 10)

	removed, err := pruner.Prune(context.Background(), "lib-1", []string{"/pool"}, seen(paths[3:]), nil)
	if err != nil || removed != 3 {
		t.Fatalf("30%% under a 50%% guard must prune, got %d err=%v", removed, err)
	}
}
