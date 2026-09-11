package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBenchFeatures(t *testing.T) {
	text := `frame:0    pts:0       pts_time:0
lavfi.signalstats.YDIF=2.500000
lavfi.scd.score=0.100000
lavfi.siti.si=40.5
lavfi.siti.ti=10.0
lavfi.entropy.normalized_entropy.normal.Y=0.900000
lavfi.bitplanenoise.0.1=0.050000
lavfi.blur=3.5
lavfi.block=0.2
frame:1    pts:1       pts_time:0.04
lavfi.signalstats.YDIF=3.500000
lavfi.scd.score=12.000000
lavfi.siti.si=41.5
lavfi.siti.ti=12.0
lavfi.entropy.normalized_entropy.normal.Y=0.700000
lavfi.bitplanenoise.0.1=0.150000
lavfi.blur=4.5
lavfi.block=0.4
`
	f := parseBenchFeatures(text, 60)
	assert.Equal(t, 2, f.Frames)
	assert.InDelta(t, 3.0, f.MotionYDIF, 1e-9)
	assert.InDelta(t, 41.0, f.SI, 1e-9)
	assert.InDelta(t, 11.0, f.TI, 1e-9)
	assert.InDelta(t, 0.8, f.EntropyY, 1e-9)
	assert.InDelta(t, 0.1, f.Grain, 1e-9)
	assert.InDelta(t, 4.0, f.Blur, 1e-9)
	assert.InDelta(t, 0.3, f.Block, 1e-9)
	assert.InDelta(t, 1.0, f.SceneCutsPerMin03, 1e-9, "one frame above the classifier's 0.3 threshold")
	assert.InDelta(t, 1.0, f.SceneCutsPerMin10, 1e-9, "one frame above scdet's 10 threshold")
}

func TestParseXPSNRMeanY(t *testing.T) {
	text := "n:    1  XPSNR y: 60.0000  XPSNR u: 58.1767  XPSNR v: 58.0253\nn:    2  XPSNR y: 40.0000  XPSNR u: 36.1190  XPSNR v: 36.5245\nn:    3  XPSNR y: inf  XPSNR u: inf  XPSNR v: inf\n"
	assert.InDelta(t, (60+40+100)/3.0, parseXPSNRMeanY(text), 1e-9)
	assert.InDelta(t, 0.0, parseXPSNRMeanY(""), 1e-12)
}

func TestBenchSceneStarts(t *testing.T) {
	assert.Equal(t, []float64{600, 2996, 5392}, benchSceneStarts(6000, 3, 8), "10 % to 90 % minus the window")
	assert.Equal(t, []float64{2996}, benchSceneStarts(6000, 1, 8))
	assert.Equal(t, []float64{2}, benchSceneStarts(20, 3, 30), "a window longer than the usable span collapses to the start")
}

func TestSpearman(t *testing.T) {
	assert.InDelta(t, 1.0, spearman([]float64{1, 2, 3, 4}, []float64{10, 20, 30, 40}), 1e-9)
	assert.InDelta(t, -1.0, spearman([]float64{1, 2, 3, 4}, []float64{40, 30, 20, 10}), 1e-9)
	assert.InDelta(t, 1.0, spearman([]float64{1, 2, 3, 4}, []float64{1, 10, 100, 1000}), 1e-9, "rank correlation ignores the curve's shape")
	assert.InDelta(t, 0.0, spearman([]float64{1, 1, 1}, []float64{1, 2, 3}), 1e-12, "a constant series correlates with nothing")
	assert.InDelta(t, 0.5, ranks([]float64{5, 5})[0]+ranks([]float64{5, 5})[1]-2.5, 1e-9, "ties share the mean rank")
}
