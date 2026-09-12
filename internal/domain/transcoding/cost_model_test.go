package transcoding

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The constants are the corpus fit; these pin the arithmetic to numbers read
// off the same corpus (ladder-corpus2-2026-09-12, 1080p crf 23 rows).
func TestCostModelV1_Predict(t *testing.T) {
	m := CostModelV1
	// The Rock: YDIF 5.81 measured 2899 kbps at 1080p crf 23 (1920x1080, 23.976 fps); the
	// leave-one-out prediction was 3621, the all-titles fit is close to it.
	c, ok := m.Predict(ComplexityMetrics{MotionMean: 5.81, FramesSampled: 192})
	require.True(t, ok)
	assert.InDelta(t, 1.0, c.Confidence, 1e-9)
	assert.InDelta(t, 3550, float64(m.RungKbps(c, 1920, 1080, 23.976, 23)), 150, "the fit on all titles at the reference point")
	// a remux: YDIF 1.46, measured 886; the model over-predicts it (its worst case, x1.9).
	v, _ := m.Predict(ComplexityMetrics{MotionMean: 1.46, FramesSampled: 192})
	assert.InDelta(t, 1340, float64(m.RungKbps(v, 1920, 1080, 23.976, 23)), 60)

	t.Run("too few frames: no prediction, the class table applies", func(t *testing.T) {
		_, ok := m.Predict(ComplexityMetrics{MotionMean: 3, FramesSampled: 20})
		assert.False(t, ok)
		_, ok = m.Predict(ComplexityMetrics{MotionMean: 0, FramesSampled: 200})
		assert.False(t, ok, "no motion measured is no measurement")
	})
	t.Run("outside the fitted range: clamped, half confidence", func(t *testing.T) {
		still, ok := m.Predict(ComplexityMetrics{MotionMean: 0.2, FramesSampled: 200})
		require.True(t, ok)
		assert.InDelta(t, m.YDIFMin, still.YDIF, 1e-9)
		assert.InDelta(t, 0.5, still.Confidence, 1e-9)
		fast, _ := m.Predict(ComplexityMetrics{MotionMean: 40, FramesSampled: 200})
		assert.InDelta(t, m.YDIFMax, fast.YDIF, 1e-9)
	})
}

func TestCostModelV1_RungScaling(t *testing.T) {
	m := CostModelV1
	c, _ := m.Predict(ComplexityMetrics{MotionMean: 4, FramesSampled: 200})
	ref := float64(m.RungKbps(c, 1920, 1080, 24, 23))
	// Bitrate follows pixels^0.8: 720p is (0.444)^0.8 = 0.52x the 1080p bitrate, 2160p 3.03x.
	assert.InDelta(t, 0.523, float64(m.RungKbps(c, 1280, 720, 24, 23))/ref, 0.01)
	assert.InDelta(t, 3.03, float64(m.RungKbps(c, 3840, 2160, 24, 23))/ref, 0.02)
	// CRF: 5 points down costs x2.39, up x0.42 (2^(-5/3.98)).
	assert.InDelta(t, 2.39, float64(m.RungKbps(c, 1920, 1080, 24, 18))/ref, 0.02)
	assert.InDelta(t, 0.42, float64(m.RungKbps(c, 1920, 1080, 24, 28))/ref, 0.02)
	// Frame rate is linear; unknown means film cadence.
	assert.InDelta(t, 2.5, float64(m.RungKbps(c, 1920, 1080, 60, 23))/ref, 0.01)
	assert.Equal(t, m.RungKbps(c, 1920, 1080, 0, 23), m.RungKbps(c, 1920, 1080, 24, 23))
	assert.InDelta(t, m.CeilingMargin, float64(m.CeilingKbps(c, 1920, 1080, 24, 23))/ref, 0.001)
}

func TestCostModelV1_PriceLadder(t *testing.T) {
	m := CostModelV1
	c, _ := m.Predict(ComplexityMetrics{MotionMean: 4, FramesSampled: 200})
	scope := SourceFacts{Width: 3840, Height: 1604, FPS: 24} // 2.39:1
	priced := m.PriceLadder(LadderFilmNormal, c, scope, StaticSoftwareCRF)
	require.Len(t, priced, len(LadderFilmNormal))
	for i, p := range priced {
		assert.True(t, p.CostModelled)
		assert.Equal(t, LadderFilmNormal[i].Height, p.Height, "the shape is the table's")
		assert.Equal(t, LadderFilmNormal[i].AudioBitrate, p.AudioBitrate)
	}
	assert.Equal(t, int64(800), LadderFilmNormal[0].VideoBitrate, "input not mutated")
	// A 2.39:1 source fills 1920x802 of the 1080p rung: priced at that frame, not 1920x1080.
	full := m.PriceLadder(LadderFilmNormal, c, SourceFacts{Width: 1920, Height: 1080, FPS: 24}, StaticSoftwareCRF)
	assert.Less(t, priced[3].VideoBitrate, full[3].VideoBitrate)
	assert.InDelta(t, 0.80, float64(priced[3].VideoBitrate)/float64(full[3].VideoBitrate), 0.02, "(802/1080)^0.8")
	for i := 1; i < len(priced); i++ {
		assert.Greater(t, priced[i].VideoBitrate, priced[i-1].VideoBitrate, "monotone")
	}
}

func TestStaticQualityPolicy_CostModelledRungHasNoHeadroom(t *testing.T) {
	table := HLSProfile{VideoBitrate: 5000}
	qt := StaticQualityPolicy{}.TargetFor(table, EncodeRoute{})
	assert.Equal(t, int64(8000), qt.MaxKbps, "table rung: 160 % headroom")
	priced := HLSProfile{VideoBitrate: 5000, CostModelled: true}
	qt = StaticQualityPolicy{}.TargetFor(priced, EncodeRoute{})
	assert.Equal(t, int64(5000), qt.MaxKbps, "cost-modelled rung: the margin is in the price")
	assert.Equal(t, int64(10000), qt.BufferKbps)
	assert.Equal(t, StaticSoftwareCRF, qt.Quality)
}

// A probe replaces the prediction only when it found the prediction out of
// its margin with at least two scenes; a confirmed probe, or a one-scene
// probe, leaves the model alone (measured: one scene is a worse estimate).
func TestCostModelV1_ProbeOverride(t *testing.T) {
	m := CostModelV1
	base := ComplexityMetrics{MotionMean: 4, FramesSampled: 200}
	pred, _ := m.Predict(base)
	pts := func(n int) []ProbePoint {
		out := []ProbePoint{}
		for i := range n {
			out = append(out, ProbePoint{StartSec: float64(600 * (i + 1)), CRF: StaticSoftwareCRF, Kbps: 9000})
		}
		return out
	}
	probe := &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version, Height: 1080, MeasuredBPP: 0.2, Confirmed: false, Spread: 1.8, Points: pts(2)}
	withProbe := base
	withProbe.Probe = probe
	c, ok := m.Predict(withProbe)
	require.True(t, ok)
	assert.True(t, c.Probed)
	assert.InDelta(t, 0.2/math.Pow(2, 3/m.CRFHalvingStep), c.BPPRef, 1e-9, "brought back from CRF 20 to the reference CRF 23")
	assert.InDelta(t, math.Sqrt(1.8), c.Margin, 1e-9, "half the observed spread, above the floor")
	assert.NotEqual(t, pred.BPPRef, c.BPPRef)
	assert.InDelta(t, math.Sqrt(1.8), float64(m.CeilingKbps(c, 1920, 1080, 24, 20))/float64(m.RungKbps(c, 1920, 1080, 24, 20)), 1e-3) // integer kbps rounding

	t.Run("floor", func(t *testing.T) {
		p2 := *probe
		p2.Spread = 1.1
		mm := base
		mm.Probe = &p2
		c, _ := m.Predict(mm)
		assert.InDelta(t, TruthMarginFloor, c.Margin, 1e-9)
	})
	t.Run("confirmed: the model stands", func(t *testing.T) {
		p2 := *probe
		p2.Confirmed = true
		mm := base
		mm.Probe = &p2
		c, _ := m.Predict(mm)
		assert.False(t, c.Probed)
		assert.InDelta(t, pred.BPPRef, c.BPPRef, 1e-12)
	})
	t.Run("one scene: the model stands", func(t *testing.T) {
		p2 := *probe
		p2.Points = pts(1)
		mm := base
		mm.Probe = &p2
		c, _ := m.Predict(mm)
		assert.False(t, c.Probed)
	})
	t.Run("another model version or protocol: ignored", func(t *testing.T) {
		p2 := *probe
		p2.ModelVersion = m.Version + 1
		mm := base
		mm.Probe = &p2
		c, _ := m.Predict(mm)
		assert.False(t, c.Probed)
		p3 := *probe
		p3.Protocol = TitleProbeProtocol + 1
		mm.Probe = &p3
		c, _ = m.Predict(mm)
		assert.False(t, c.Probed)
	})
	t.Run("a probe alone, without metrics, is still a prediction", func(t *testing.T) {
		c, ok := m.Predict(ComplexityMetrics{Probe: probe})
		assert.True(t, ok)
		assert.True(t, c.Probed)
	})
}
