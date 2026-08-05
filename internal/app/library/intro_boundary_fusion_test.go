package library

import "testing"

func TestFuseBoundary(t *testing.T) {
	t.Parallel()

	// Three independent signals agree (~315s) and one outlier (audio end 349s).
	// The agreeing cluster wins; the outlier is excluded.
	fb, ok := fuseBoundary([]boundaryVote{
		{ms: 349_000, weight: 0.4, source: "audio"},
		{ms: 315_000, weight: 1.0, source: "subtitle"},
		{ms: 316_500, weight: 0.7, source: "silence"},
		{ms: 314_000, weight: 0.6, source: "scene"},
	}, 4_000)
	if !ok {
		t.Fatal("expected a fused boundary")
	}
	if fb.ms < 314_000 || fb.ms > 316_500 {
		t.Errorf("consensus ms = %d, want ~315000", fb.ms)
	}
	if fb.agree != 3 {
		t.Errorf("agree = %d, want 3", fb.agree)
	}
	if fb.confidence < 0.7 {
		t.Errorf("confidence = %.2f, want high (>0.7)", fb.confidence)
	}

	// A lone signal: ok, but capped confidence - it carries 100% of its own
	// weight, which proves nothing about corroboration.
	fb, ok = fuseBoundary([]boundaryVote{{ms: 200_000, weight: 0.4, source: "audio"}}, 4_000)
	if !ok || fb.ms != 200_000 || fb.agree != 1 {
		t.Fatalf("lone vote: got ms=%d agree=%d", fb.ms, fb.agree)
	}
	if fb.confidence != 0.5 {
		t.Errorf("lone confidence = %.2f, want the 0.5 no-corroboration cap", fb.confidence)
	}

	// Two votes disagreeing beyond tolerance: the heavier wins alone → same cap.
	fb, ok = fuseBoundary([]boundaryVote{
		{ms: 200_000, weight: 0.4, source: "audio"},
		{ms: 230_000, weight: 1.0, source: "subtitle"},
	}, 4_000)
	if !ok || fb.ms != 230_000 {
		t.Fatalf("disagreeing votes: got ms=%d, want the heavier witness", fb.ms)
	}
	if fb.confidence != 0.5 {
		t.Errorf("uncorroborated winner confidence = %.2f, want 0.5", fb.confidence)
	}

	// No votes → not ok.
	if _, ok := fuseBoundary(nil, 4_000); ok {
		t.Error("no votes must return ok=false")
	}
	// Zero-weight votes are ignored.
	if _, ok := fuseBoundary([]boundaryVote{{ms: 1, weight: 0, source: "x"}}, 4_000); ok {
		t.Error("zero-weight votes must be ignored")
	}
}

func TestSnapToNearest(t *testing.T) {
	t.Parallel()
	cuts := []int64{310_000, 314_200, 330_000}
	if got, ok := snapToNearest(315_000, cuts, 2_000); !ok || got != 314_200 {
		t.Errorf("snap: got %d ok=%v, want 314200 true", got, ok)
	}
	// Nothing within tolerance → keep target.
	if got, ok := snapToNearest(315_000, cuts, 500); ok || got != 315_000 {
		t.Errorf("snap none: got %d ok=%v, want 315000 false", got, ok)
	}
}
