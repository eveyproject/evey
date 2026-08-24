package library

import (
	"context"
	"errors"
	"testing"
)

// The refined branch of refineWithPrecisionPass had never been executed.
//
// PrecisionPass returns its INPUT when it finds nothing, and the pipeline
// detects that no-op to fall through to a fixed pad. Both fake IntroDetector
// implementations in this repo did exactly that - echo the input - so
// `usePrecision` was false in every test that ever ran, and the whole
// silencedetect-refined path (plus fuseIntroEnd behind it) was dead weight in
// the suite. Delete the refinement and nothing went red.
//
// It decides the timestamps of the Skip Intro button, so a regression there is
// silent by construction: the button still appears, just at the wrong second.
func TestRefineWithPrecisionPass_UsesTheRefinedBoundaryWhenThereIsOne(t *testing.T) {
	windows := []confirmedWindow{{startMs: 60_000, endMs: 90_000}}

	t.Run("a real refinement replaces the pHash window", func(t *testing.T) {
		// Inside the +/-15s sanity margin, and different from the input.
		det := &fakeDetector{precisionStartMs: 58_500, precisionEndMs: 92_500}
		svc := newTestService(det, &fakeSegmentRepo{}, nil)

		got := svc.refineWithPrecisionPass(context.Background(), "/some/file.mkv", windows)

		if len(got) != 1 {
			t.Fatalf("want 1 window, got %d", len(got))
		}
		if got[0].startMs != 58_500 || got[0].endMs != 92_500 {
			t.Fatalf("want the refined boundary 58500-92500, got %d-%d - the refined branch is still unreachable",
				got[0].startMs, got[0].endMs)
		}
	})

	t.Run("an echo means 'found nothing' and falls through to the asymmetric pad", func(t *testing.T) {
		// The default fake echoes its input - the historical no-op.
		svc := newTestService(&fakeDetector{}, &fakeSegmentRepo{}, nil)

		got := svc.refineWithPrecisionPass(context.Background(), "/some/file.mkv", windows)

		// fallbackPadStartMs = 10s before, fallbackPadEndMs = 2s after.
		if got[0].startMs != 50_000 || got[0].endMs != 92_000 {
			t.Fatalf("want the padded window 50000-92000, got %d-%d", got[0].startMs, got[0].endMs)
		}
	})

	t.Run("a boundary far outside the anchor is refused as implausible", func(t *testing.T) {
		// 20s before the window start is beyond the 2*padSec = 30s margin?
		// No - pick something clearly outside: end BEFORE the window starts.
		det := &fakeDetector{precisionStartMs: 1_000, precisionEndMs: 2_000}
		svc := newTestService(det, &fakeSegmentRepo{}, nil)

		got := svc.refineWithPrecisionPass(context.Background(), "/some/file.mkv", windows)

		if got[0].startMs == 1_000 {
			t.Fatal("a refinement nowhere near the pHash anchor must be refused, not trusted")
		}
	})

	t.Run("an error falls through to the pad rather than dropping the window", func(t *testing.T) {
		det := &fakeDetector{precisionErr: errors.New("ffmpeg exploded")}
		svc := newTestService(det, &fakeSegmentRepo{}, nil)

		got := svc.refineWithPrecisionPass(context.Background(), "/some/file.mkv", windows)

		if len(got) != 1 {
			t.Fatalf("a failed refinement must keep the window, got %d", len(got))
		}
		if got[0].startMs != 50_000 || got[0].endMs != 92_000 {
			t.Fatalf("want the padded window on error, got %d-%d", got[0].startMs, got[0].endMs)
		}
	})
}
