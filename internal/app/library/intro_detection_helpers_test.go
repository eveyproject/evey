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

func approxEq(got, want float64) bool {
	d := got - want
	return d > -1e-9 && d < 1e-9
}

func TestPearsonImpl(t *testing.T) {
	if got := pearsonImpl([]float64{1, 2, 3}, []float64{1, 2, 3}); !approxEq(got, 1) {
		t.Errorf("identical vectors -> 1.0, got %v", got)
	}
	// Positive linear scaling is still perfect correlation.
	if got := pearsonImpl([]float64{1, 2, 3}, []float64{2, 4, 6}); !approxEq(got, 1) {
		t.Errorf("positive scale -> 1.0, got %v", got)
	}
	// Reversed = perfect anti-correlation.
	if got := pearsonImpl([]float64{1, 2, 3}, []float64{3, 2, 1}); !approxEq(got, -1) {
		t.Errorf("reversed -> -1.0, got %v", got)
	}
	// A constant operand has zero variance -> defined as 0 (no correlation).
	if got := pearsonImpl([]float64{1, 2, 3}, []float64{5, 5, 5}); got != 0 {
		t.Errorf("constant operand -> 0, got %v", got)
	}
	// Length mismatch / empty -> 0.
	if got := pearsonImpl([]float64{1, 2}, []float64{1, 2, 3}); got != 0 {
		t.Errorf("length mismatch -> 0, got %v", got)
	}
	if got := pearsonImpl(nil, nil); got != 0 {
		t.Errorf("empty -> 0, got %v", got)
	}
}

func TestSlidingBestPearsonBandedImpl(t *testing.T) {
	type res struct {
		corr   float64
		offset int
	}
	check := func(name string, gotC float64, gotO int, want res) {
		if !approxEq(gotC, want.corr) || gotO != want.offset {
			t.Errorf("%s: got (corr=%v, off=%d), want (corr=%v, off=%d)", name, gotC, gotO, want.corr, want.offset)
		}
	}

	// Guard cases all return (0, 0).
	c, o := slidingBestPearsonBandedImpl([]float64{1, 2}, []float64{1, 2, 3, 4}, 0)
	check("vectorSize<=0", c, o, res{0, 0})
	c, o = slidingBestPearsonBandedImpl([]float64{1, 2, 3}, []float64{1, 2, 3, 4}, 2) // target len 3 not multiple of 2
	check("target not multiple", c, o, res{0, 0})
	c, o = slidingBestPearsonBandedImpl([]float64{1, 2, 3, 4}, []float64{1, 2}, 1) // reference shorter
	check("reference shorter", c, o, res{0, 0})

	// Exact match at offset 0.
	c, o = slidingBestPearsonBandedImpl([]float64{1, 2, 3}, []float64{1, 2, 3}, 1)
	check("identical -> corr 1 @ off 0", c, o, res{1, 0})

	// A distinctive down-up target [3,1,2] matches uniquely at bin-offset 3
	// (vectorSize=1) — the increasing window [1,2,3] anti-correlates, etc.
	c, o = slidingBestPearsonBandedImpl(
		[]float64{3, 1, 2},
		[]float64{1, 2, 3, 3, 1, 2, 1, 1, 1}, 1)
	check("offset found @ bin 3", c, o, res{1, 3})

	// Banded (vectorSize=2): the 2-bin target matches at bin-offset 1
	// (= float offset 2), proving the slide steps in vectorSize-float strides.
	c, o = slidingBestPearsonBandedImpl(
		[]float64{3, 1, 2, 4},
		[]float64{0, 0, 3, 1, 2, 4, 9, 9}, 2)
	check("banded offset @ bin 1", c, o, res{1, 1})
}
