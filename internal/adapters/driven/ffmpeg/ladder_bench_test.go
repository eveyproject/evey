package ffmpeg

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
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

func TestEfficiencyFactor(t *testing.T) {
	pts := []EffPoint{
		{ActualKbps: 1000, XPSNRY: 34}, {ActualKbps: 2000, XPSNRY: 37}, {ActualKbps: 4000, XPSNRY: 40},
	}
	// The reference spent 2000 for 37: an encoder that reaches 37 at 2000 has factor 1.
	f, oor := efficiencyFactor(pts, 2000, 37)
	assert.InDelta(t, 1.0, f, 1e-9)
	assert.False(t, oor)
	// Reaching 38.5 halfway (in log bitrate) between 2000 and 4000: sqrt(2) x 2000 over a 2000 reference.
	f, _ = efficiencyFactor(pts, 2000, 38.5)
	assert.InDelta(t, math.Sqrt2, f, 1e-6)
	// A cheaper encoder: the reference needed 3000 for 38.5.
	f, _ = efficiencyFactor(pts, 3000, 38.5)
	assert.InDelta(t, math.Sqrt2*2000/3000, f, 1e-6)
	// Outside the curve: nearest end, flagged.
	f, oor = efficiencyFactor(pts, 2000, 45)
	assert.True(t, oor)
	assert.InDelta(t, 2.0, f, 1e-9)
	f, oor = efficiencyFactor(pts, 2000, 30)
	assert.True(t, oor)
	assert.InDelta(t, 0.5, f, 1e-9)
	_, oor = efficiencyFactor(pts[:1], 2000, 37)
	assert.True(t, oor, "one point is no curve")
}

func TestEffSummaries(t *testing.T) {
	enc := BenchEncoder{Name: "nvenc-h264", Codec: "h264", Accel: transcoding.NVENC}
	mk := func(path string, factors ...float64) *EffResult {
		r := &EffResult{Source: BenchSource{Path: path}}
		for _, f := range factors {
			r.Scenes = append(r.Scenes, EffScene{Measures: []EffMeasure{{Encoder: enc, Height: 1080, Factor: f, Points: []EffPoint{{Ratio: 1, EncodeFP: 300}}}}})
		}
		return r
	}
	s := EffSummaries([]*EffResult{mk("a", 1.2, 1.3, 1.25), mk("b", 1.4, 1.5), mk("c", 1.1)})
	require.Len(t, s, 1)
	assert.Equal(t, 6, s[0].Scenes)
	assert.Equal(t, 3, s[0].Titles)
	assert.InDelta(t, 1.275, s[0].MedianFactor, 1e-9)
	assert.InDelta(t, 300, s[0].MedianFPS, 1e-9)
	assert.Greater(t, s[0].LOOWorst, s[0].LOOP50)
	assert.GreaterOrEqual(t, s[0].DispWorst, s[0].DispP90)
}

func TestBenchEncoderArgs_UseTheProductionBuilders(t *testing.T) {
	nv := strings.Join(benchEncoderArgs(BenchEncoder{Name: "nvenc-h264", Codec: "h264", Accel: transcoding.NVENC, Features: []string{"bframes"}}, 4000), " ")
	assert.Contains(t, nv, "-c:v h264_nvenc -preset p4 -forced-idr 1 -bf 3 -b:v 4000k -maxrate 4000k -bufsize 8000k")
	va := strings.Join(benchEncoderArgs(BenchEncoder{Name: "vaapi-hevc", Codec: "hevc", Accel: transcoding.VAAPI}, 4000), " ")
	assert.Contains(t, va, "-c:v hevc_vaapi -rc_mode VBR -b:v 4000k -maxrate 4000k -bufsize 8000k")
	sw := strings.Join(benchEncoderArgs(BenchEncoder{Name: "x265", Codec: "hevc", Preset: "veryfast"}, 4000), " ")
	assert.Contains(t, sw, "-c:v libx265 -preset veryfast -b:v 4000k -maxrate 4000k -bufsize 8000k -pix_fmt yuv420p")
}
