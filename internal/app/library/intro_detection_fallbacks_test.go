package library

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

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

// The service glue around discoverRecurringIntro: guards, sibling collection
// through the repo, per-episode fingerprints through the chromaprinter, and the
// segment shape when the season shares an intro.
func TestDiscoverIntroByAudio(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	intro := makeNoise(r, 130)
	fps := map[string][]uint32{
		"e1": episodeWithIntro(r, 900, 250, intro, 5),
		"e2": episodeWithIntro(r, 900, 320, intro, 5),
		"e3": episodeWithIntro(r, 900, 470, intro, 5),
		"e4": episodeWithIntro(r, 900, 600, intro, 5),
	}
	var eps []*domainlibrary.MediaItem
	for i, id := range []string{"e1", "e2", "e3", "e4"} {
		n := i + 1
		eps = append(eps, &domainlibrary.MediaItem{
			ID: shared.MediaItemID(id), ParentID: "season-1", Type: domainlibrary.Episode, IndexNumber: &n,
			File: &domainlibrary.MediaFile{Path: id},
		})
	}
	newSvc := func(cp Chromaprinter) *IntroDetectionService {
		svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{siblings: eps}, &fakeConfigRepo{}, &fakeDetector{}, t.TempDir())
		if cp != nil {
			svc.WithChromaprinter(cp)
		}
		return svc
	}

	seg, found := newSvc(&pathChromaprinter{byPath: fps}).discoverIntroByAudio(context.Background(), eps[0])
	if !found {
		t.Fatal("a season sharing one intro must be discovered")
	}
	if seg.Type != "intro" || seg.Source != "chromaprint_discovery" || seg.Confidence != 0.85 {
		t.Errorf("segment = %+v", seg)
	}
	if seg.StartMs < 29_000 || seg.StartMs > 37_000 || seg.EndMs <= seg.StartMs {
		t.Errorf("window = [%d, %d] ms, want a start near 33 s", seg.StartMs, seg.EndMs)
	}

	// Guards: no chromaprinter, no file, no parent - and nothing to compare to.
	if _, ok := newSvc(nil).discoverIntroByAudio(context.Background(), eps[0]); ok {
		t.Error("no chromaprinter must discover nothing")
	}
	cp := &pathChromaprinter{byPath: fps}
	if _, ok := newSvc(cp).discoverIntroByAudio(context.Background(), &domainlibrary.MediaItem{ID: "x", ParentID: "season-1"}); ok {
		t.Error("no file must discover nothing")
	}
	if _, ok := newSvc(cp).discoverIntroByAudio(context.Background(), &domainlibrary.MediaItem{ID: "x", File: &domainlibrary.MediaFile{Path: "x"}}); ok {
		t.Error("no parent must discover nothing")
	}
	// The target's own fingerprint failing to extract ends the pass.
	if _, ok := newSvc(&pathChromaprinter{byPath: fps, failOn: map[string]bool{"e1": true}}).discoverIntroByAudio(context.Background(), eps[0]); ok {
		t.Error("an unextractable target must discover nothing")
	}
	// Siblings that share nothing with the target: no intro.
	noise := map[string][]uint32{"e1": fps["e1"], "e2": makeNoise(r, 900), "e3": makeNoise(r, 900), "e4": makeNoise(r, 900)}
	if _, ok := newSvc(&pathChromaprinter{byPath: noise}).discoverIntroByAudio(context.Background(), eps[0]); ok {
		t.Error("unrelated siblings must discover nothing")
	}
}

// A manifest that is not there yet ends the wait with errTrickplayPending
// after the bounded wait (2 min in production, not 24 h under a 30-minute
// episode budget), and that error is transient: the sweep does not count it
// as an attempt, so the episode comes back once the manifest exists.
func TestWaitForManifest_PendingIsBoundedAndTransient(t *testing.T) {
	svc := NewIntroDetectionService(&fakeSegmentRepo{}, &fakeTrickplayRepo{}, &fakeMediaItemRepo{}, &fakeConfigRepo{}, &fakeDetector{}, t.TempDir())
	svc.trickplayWait = 20 * time.Millisecond
	start := time.Now()
	_, err := svc.waitForManifest(context.Background(), "m1")
	if !errors.Is(err, errTrickplayPending) {
		t.Fatalf("want errTrickplayPending, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("the wait must be bounded by trickplayWait")
	}
	if failureConsumesAttempt(errTrickplayPending) {
		t.Fatal("a pending trickplay must not consume a detection attempt")
	}
}
