package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Regression for the silent-zero bug: the original implementation grepped
// `lavfi.signalstats.YDEV=...` and `lavfi.scd.score=...` against output
// that contained neither (signalstats emits YDIF; scdet was missing from
// the filtergraph). Both regexes returned zero, classifier always saw
// frames_sampled=0 + scenecuts=0, and the heuristic collapsed to
// bitrate-only buckets. This test exercises a real metadata=print block
// so any future filter/regex drift trips here, not in production.
func TestParseSignalStats_ExtractsMotionAndScenecuts(t *testing.T) {
	// Trimmed from a live `ffmpeg -vf signalstats,scdet,metadata=print` on a
	// 1080p sample. Includes 3 frames: one scene-change (score 0.51 > 0.3) and
	// two ordinary frames.
	sample := `
frame:0    pts:0       pts_time:0
lavfi.signalstats.YMIN=0
lavfi.signalstats.YHIGH=235
lavfi.signalstats.YAVG=88.45
lavfi.signalstats.YDIF=1.30413
lavfi.scd.mafd=0.509
lavfi.scd.score=0.509
frame:1    pts:42      pts_time:0.042
lavfi.signalstats.YDIF=1.32702
lavfi.scd.mafd=0.518
lavfi.scd.score=0.009
frame:2    pts:84      pts_time:0.084
lavfi.signalstats.YDIF=1.25605
lavfi.scd.mafd=0.510
lavfi.scd.score=0.012
`

	motionSum, frameCount, sceneCuts := parseSignalStats(sample)

	assert.Equal(t, 3, frameCount, "must count every YDIF frame")
	assert.InDelta(t, 3.8872, motionSum, 0.001, "YDIF values must sum")
	assert.Equal(t, 1, sceneCuts, "only the 0.509 score crosses the 0.3 scenecut threshold")
}

// Empty input must not panic and must return zeros — the classifier
// relies on this to fall back to bitrate/resolution buckets.
func TestParseSignalStats_EmptyInput_Zero(t *testing.T) {
	motionSum, frameCount, sceneCuts := parseSignalStats("")
	assert.Zero(t, motionSum)
	assert.Zero(t, frameCount)
	assert.Zero(t, sceneCuts)
}

// Defensive: garbled keys (typo, truncated stream) must produce a
// best-effort partial parse, not a panic.
func TestParseSignalStats_GarbledInput_PartialParse(t *testing.T) {
	garbled := `
lavfi.signalstats.YDIF=2.5
lavfi.signalstats.YDIF=garbage
lavfi.scd.score=0.4
lavfi.scd.score=truncate
`
	motionSum, frameCount, sceneCuts := parseSignalStats(garbled)
	assert.InDelta(t, 2.5, motionSum, 0.001)
	assert.Equal(t, 1, frameCount, "only the well-formed YDIF counts")
	assert.Equal(t, 1, sceneCuts, "only the well-formed score above threshold counts")
}
