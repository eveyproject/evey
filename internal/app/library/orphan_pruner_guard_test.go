package library_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/app/prunehold"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// seedUnder creates n items "<root>/f<i>.mkv" and returns their paths.
func seedUnder(t *testing.T, items *libfakes.FakeMediaItemCommandRepo, root string, n int) []string {
	t.Helper()
	paths := make([]string, 0, n)
	for i := range n {
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
	sus := pruner.Suspensions("lib-1")
	if len(sus) != 1 || sus[0].Snapshot == "" {
		t.Fatalf("want /pool held with a snapshot, got %+v", sus)
	}
	if err := pruner.ConfirmPrune("lib-2", "/pool", sus[0].Snapshot); !errors.Is(err, prunehold.ErrNothingSuspended) {
		t.Fatalf("a library with nothing suspended must not arm, got %v", err)
	}
	if err := pruner.ConfirmPrune("lib-1", "/pool", "stale"); !errors.Is(err, prunehold.ErrSnapshotChanged) {
		t.Fatalf("a stale snapshot must not arm, got %v", err)
	}
	if err := pruner.ConfirmPrune("lib-1", "/pool", sus[0].Snapshot); err != nil {
		t.Fatalf("want confirm accepted: %v", err)
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

// The confirmation must not outlive a pass that stopped before the guard:
// whatever disappears afterwards is a different set and stays held.
func TestOrphanPruner_Confirm_EarlyExitPassThenMassLoss_NothingDeleted(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()
	ctx := context.Background()
	roots := []string{"/pool"}
	pool := seedUnder(t, items, "/pool", 4000)

	if _, err := pruner.Prune(ctx, "lib-1", roots, seen(pool[300:]), nil); err != nil {
		t.Fatalf("prune: %v", err)
	}
	sus := pruner.Suspensions("lib-1")
	if len(sus) != 1 || sus[0].Missing != 300 {
		t.Fatalf("want /pool held for 300, got %+v", sus)
	}
	if err := pruner.ConfirmPrune("lib-1", "/pool", sus[0].Snapshot); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	// The pass the confirmation started sees nothing (mount dropped): it
	// exits before judging any root.
	if removed, err := pruner.Prune(ctx, "lib-1", roots, map[string]struct{}{}, nil); err != nil || removed != 0 {
		t.Fatalf("early exit: removed %d err=%v", removed, err)
	}
	// Next pass: 3000 absent.
	removed, err := pruner.Prune(ctx, "lib-1", roots, seen(pool[3000:]), nil)
	if err != nil || removed != 0 {
		t.Fatalf("want nothing deleted, got %d err=%v", removed, err)
	}
	if n := len(items.All()); n != 4000 {
		t.Fatalf("want 4000 items kept, got %d", n)
	}
	if sus := pruner.Suspensions("lib-1"); len(sus) != 1 || sus[0].Missing != 3000 {
		t.Fatalf("want /pool held for 3000, got %+v", sus)
	}
}

func TestOrphanPruner_Confirm_OneRoot_OtherRootStaysHeld(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()
	ctx := context.Background()
	roots := []string{"/a", "/b"}
	a := seedUnder(t, items, "/a", 100)
	b := seedUnder(t, items, "/b", 100)
	s := seen(a[:60], b[:50])

	if _, err := pruner.Prune(ctx, "lib-1", roots, s, nil); err != nil {
		t.Fatalf("prune: %v", err)
	}
	sus := pruner.Suspensions("lib-1")
	if len(sus) != 2 || sus[0].Root != "/a" || sus[1].Root != "/b" {
		t.Fatalf("want /a and /b held, got %+v", sus)
	}
	if err := pruner.ConfirmPrune("lib-1", "/a", sus[0].Snapshot); err != nil {
		t.Fatalf("confirm /a: %v", err)
	}
	removed, err := pruner.Prune(ctx, "lib-1", roots, s, nil)
	if err != nil || removed != 40 {
		t.Fatalf("want only /a's 40 removed, got %d err=%v", removed, err)
	}
	if left := pruner.Suspensions("lib-1"); len(left) != 1 || left[0].Root != "/b" || left[0].Missing != 50 {
		t.Fatalf("want /b still held, got %+v", left)
	}
	if n := len(items.All()); n != 160 {
		t.Fatalf("want 160 items left, got %d", n)
	}
}

// A confirmation names one set: if the set grew between the page and the
// pass, nothing goes and the root is held on the new set.
func TestOrphanPruner_Confirm_SetChangedBeforePass_NothingDeleted(t *testing.T) {
	t.Parallel()
	pruner, items, _, _ := newPrunerHarness()
	ctx := context.Background()
	roots := []string{"/pool"}
	pool := seedUnder(t, items, "/pool", 100)
	if _, err := pruner.Prune(ctx, "lib-1", roots, seen(pool[:60]), nil); err != nil {
		t.Fatalf("prune: %v", err)
	}
	sus := pruner.Suspensions("lib-1")
	if err := pruner.ConfirmPrune("lib-1", "/pool", sus[0].Snapshot); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	removed, err := pruner.Prune(ctx, "lib-1", roots, seen(pool[:50]), nil)
	if err != nil || removed != 0 {
		t.Fatalf("want nothing deleted, got %d err=%v", removed, err)
	}
	if left := pruner.Suspensions("lib-1"); len(left) != 1 || left[0].Missing != 50 || left[0].Snapshot == sus[0].Snapshot {
		t.Fatalf("want /pool held on the new set, got %+v", left)
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
