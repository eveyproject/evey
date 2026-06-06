package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBenchmarkRunner_DefaultMargin pins the calibration headroom: a small 20 %
// over realtime. The cap is deliberately NOT over-conservative — the auto-degrade
// ABR ladder is the safety net, so a rung the box genuinely sustains (e.g. 1080p
// HDR at ~1.37x realtime) stays the cap instead of being dropped.
func TestBenchmarkRunner_DefaultMargin(t *testing.T) {
	r := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x"}, nil)
	assert.InDelta(t, 1.2, r.margin, 0.001, "default margin is 1.2 (20% headroom)")

	// An explicit operator margin is honoured.
	r2 := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x", Margin: 1.4}, nil)
	assert.InDelta(t, 1.4, r2.margin, 0.001, "an explicit margin overrides the default")
}
