package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBenchmarkRunner_ConservativeMargins pins the calibration headroom: a
// comfortable 50 % default for SDR, and an even safer floor for the heavier HDR
// path so the auto top rung never overshoots what the box really sustains.
func TestBenchmarkRunner_ConservativeMargins(t *testing.T) {
	r := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x"}, nil)
	assert.InDelta(t, 1.5, r.margin, 0.001, "default SDR margin is a comfortable 1.5")
	assert.InDelta(t, 1.5, r.marginFor("software"), 0.001, "SDR uses the configured margin")
	assert.InDelta(t, softwareHDRMargin, r.marginFor("software-hdr"), 0.001, "HDR requires extra headroom")
	assert.Greater(t, r.marginFor("software-hdr"), r.marginFor("software"), "HDR is stricter than SDR")

	// An operator margin already above the HDR floor is never lowered.
	r2 := NewBenchmarkRunner(BenchmarkOpts{SamplePath: "/x", Margin: 2.0}, nil)
	assert.InDelta(t, 2.0, r2.marginFor("software-hdr"), 0.001, "a higher configured margin wins over the HDR floor")
}
