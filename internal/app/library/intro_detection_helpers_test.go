package library

import "testing"

func TestMedianFloat(t *testing.T) {
	if got := medianFloat(nil); got != 0 {
		t.Errorf("empty slice must be 0, got %v", got)
	}
	if got := medianFloat([]float64{5}); got != 5 {
		t.Errorf("single element is its own median, got %v", got)
	}
	if got := medianFloat([]float64{3, 1, 2}); got != 2 {
		t.Errorf("odd-length median (sorted middle), got %v", got)
	}
	// Even length: implementation returns the upper-median cp[n/2].
	if got := medianFloat([]float64{4, 1, 3, 2}); got != 3 {
		t.Errorf("even-length upper-median, got %v", got)
	}
	// Sorts a COPY — the caller's slice must be untouched.
	in := []float64{3, 1, 2}
	_ = medianFloat(in)
	if in[0] != 3 || in[1] != 1 || in[2] != 2 {
		t.Errorf("medianFloat mutated its input: %v", in)
	}
}

func TestEffectiveMinRatio(t *testing.T) {
	// Few siblings (<6) + a strict ratio is loosened to 0.6: with little
	// cross-episode confirmation available, demanding a high match ratio would
	// reject too many true intros.
	if got := effectiveMinRatio(introConfig{MinRatio: 0.7}, 3); got != 0.6 {
		t.Errorf("few siblings + strict ratio must loosen to 0.6, got %v", got)
	}
	// Few siblings but an already-lenient ratio is kept as-is.
	if got := effectiveMinRatio(introConfig{MinRatio: 0.5}, 3); got != 0.5 {
		t.Errorf("few siblings + lenient ratio unchanged, got %v", got)
	}
	// Enough siblings: the configured strict ratio is kept.
	if got := effectiveMinRatio(introConfig{MinRatio: 0.7}, 10); got != 0.7 {
		t.Errorf("enough siblings keeps the strict ratio, got %v", got)
	}
	// Boundary: 6 siblings counts as "enough" (the guard is strictly < 6).
	if got := effectiveMinRatio(introConfig{MinRatio: 0.7}, 6); got != 0.7 {
		t.Errorf("6 siblings is enough (not loosened), got %v", got)
	}
}
