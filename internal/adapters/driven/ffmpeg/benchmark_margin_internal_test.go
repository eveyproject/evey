package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBenchmarkRunner_DefaultMargin pins the calibration headroom: a small 20 %
// over realtime. The cap is deliberately NOT over-conservative - the auto-degrade
// ABR ladder is the safety net, so a rung the box genuinely sustains (e.g. 1080p
// HDR at ~1.37x realtime) stays the cap instead of being dropped.
func TestBenchmarkRunner_DefaultMargin(t *testing.T) {
	r := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x"}, nil)
	assert.InDelta(t, 1.2, r.margin, 0.001, "default margin is 1.2 (20% headroom)")

	// An explicit operator margin is honoured.
	r2 := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x", Margin: 1.4}, nil)
	assert.InDelta(t, 1.4, r2.margin, 0.001, "an explicit margin overrides the default")
}

// TestBenchmarkRunner_DefaultBenchSamples pins the median sample count: 3 by
// default (odd → a lone slow outlier is rejected), overridable to 1 for fast tests.
func TestBenchmarkRunner_DefaultBenchSamples(t *testing.T) {
	r := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x"}, nil)
	assert.Equal(t, 3, r.benchSamples, "default is a 3-sample median")

	r1 := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x", BenchSamples: 1}, nil)
	assert.Equal(t, 1, r1.benchSamples, "an explicit sample count overrides the default")

	rNeg := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x", BenchSamples: -2}, nil)
	assert.Equal(t, 3, rNeg.benchSamples, "a non-positive sample count falls back to 3")
}

// TestMedianFloat covers the outlier-rejection estimator that decides a rung's
// pass/fail across samples: a single slow run (a momentarily-busy host) must not
// move the median, which is the whole point of sampling N times.
func TestMedianFloat(t *testing.T) {
	assert.InDelta(t, 0.0, medianFloat(nil), 0.001, "empty → 0")
	assert.InDelta(t, 30.0, medianFloat([]float64{30}), 0.001, "single value")
	assert.InDelta(t, 35.0, medianFloat([]float64{30, 40}), 0.001, "even → mean of middles")
	assert.InDelta(t, 30.0, medianFloat([]float64{30, 31, 29}), 0.001, "odd → middle, order-independent")
	// The motivating case: two healthy reads + one outlier from a busy moment.
	// The median holds at the healthy value instead of being dragged down.
	assert.InDelta(t, 31.0, medianFloat([]float64{31, 32, 12}), 0.001, "a lone slow outlier doesn't move the median")
	assert.InDelta(t, 31.0, medianFloat([]float64{12, 31, 32}), 0.001, "outlier position doesn't matter")
}
