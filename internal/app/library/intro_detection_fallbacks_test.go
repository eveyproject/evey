package library

import (
	"context"
	"errors"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// erringTrickplayRepo answers every lookup with one error.
type erringTrickplayRepo struct {
	fakeTrickplayRepo
	err error
}

func (r *erringTrickplayRepo) GetByMediaItemID(context.Context, shared.MediaItemID) (*domainlibrary.TrickplayManifest, error) {
	return nil, r.err
}

// waitForManifest returns at once when the manifest exists, fails at once on
// a real repository error, and honours cancellation while it waits for a
// manifest that is not there yet - it must never spin a detached 24h loop.
func TestWaitForManifest(t *testing.T) {
	manifest := &domainlibrary.TrickplayManifest{ID: "tp-1", MediaItemID: "m1", Interval: 15}
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepoWithManifest{manifest: manifest}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, &fakeDetector{}, t.TempDir())
	got, err := svc.waitForManifest(context.Background(), "m1")
	if err != nil || got != manifest {
		t.Fatalf("existing manifest: got %v err=%v", got, err)
	}

	boom := errors.New("db down")
	svc = NewIntroDetectionService(&fakeSegmentRepo{}, &erringTrickplayRepo{err: boom}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, &fakeDetector{}, t.TempDir())
	if _, err := svc.waitForManifest(context.Background(), "m1"); !errors.Is(err, boom) {
		t.Fatalf("repository error must surface, got %v", err)
	}

	svc = NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, &fakeDetector{}, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.waitForManifest(ctx, "m1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled wait must return the cancellation, got %v", err)
	}
}

// Only intro segments get their end fused; recap and credits keep theirs.
func TestRefineIntroEndsBySubtitle_TouchesIntrosOnly(t *testing.T) {
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, &fakeDetector{}, t.TempDir())
	segs := []domainlibrary.MediaSegment{
		{Type: "recap", StartMs: 0, EndMs: 30_000},
		{Type: "intro", StartMs: 30_000, EndMs: 90_000},
		{Type: "credits", StartMs: 2_400_000, EndMs: 2_500_000},
	}
	got := svc.refineIntroEndsBySubtitle(context.Background(), "/media/e1.mkv", segs)
	if len(got) != 3 {
		t.Fatalf("got %d segments", len(got))
	}
	if got[0].EndMs != 30_000 || got[2].EndMs != 2_500_000 {
		t.Errorf("recap/credits ends must not move: %+v", got)
	}
	// With no subtitle or scene-cut signal the intro end is left as is.
	if got[1].EndMs != 90_000 {
		t.Errorf("intro end without signal = %d, want 90000", got[1].EndMs)
	}
}
