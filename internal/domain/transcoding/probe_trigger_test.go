package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProbeCandidate(t *testing.T) {
	m := CostModelV2
	mid := ComplexityMetrics{MotionMean: 4, FramesSampled: 200, MetricVersion: CanonicalMetricVersion} // in range, predictable

	r, ok := ProbeCandidate(mid, ProbeSignals{}, m)
	assert.False(t, ok, "a predictable title in range earns no probe: %s", r)

	r, ok = ProbeCandidate(ComplexityMetrics{FramesSampled: 0}, ProbeSignals{}, m)
	assert.True(t, ok)
	assert.Equal(t, ProbeNoMetrics, r)

	// A pre-canonical row (native YDIF, no metric_version) is treated as having
	// no usable metric: a candidate, never priced on its native value.
	r, ok = ProbeCandidate(ComplexityMetrics{MotionMean: 4, FramesSampled: 200}, ProbeSignals{}, m)
	assert.True(t, ok)
	assert.Equal(t, ProbeNoMetrics, r)

	r, ok = ProbeCandidate(ComplexityMetrics{MotionMean: 0.2, FramesSampled: 200, MetricVersion: CanonicalMetricVersion}, ProbeSignals{}, m)
	assert.True(t, ok)
	assert.Equal(t, ProbeYDIFOutOfRange, r)

	r, ok = ProbeCandidate(mid, ProbeSignals{CeilingHitSegments: 20}, m)
	assert.True(t, ok)
	assert.Equal(t, ProbeCeilingHit, r, "a sustained ceiling hit outranks the in-range verdict")

	_, ok = ProbeCandidate(mid, ProbeSignals{CeilingHitSegments: 5}, m)
	assert.False(t, ok, "a couple of spikes is normal VBR, not a candidate")

	r, ok = ProbeCandidate(mid, ProbeSignals{Animation: true}, m)
	assert.True(t, ok)
	assert.Equal(t, ProbeModelWeakSpot, r)

	t.Run("already probed at this model version: never again", func(t *testing.T) {
		probed := mid
		probed.MotionMean = 0.2 // even out of range
		probed.Probe = &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version}
		_, ok := ProbeCandidate(probed, ProbeSignals{CeilingHitSegments: 99, Animation: true}, m)
		assert.False(t, ok)
	})
	t.Run("probed at an older model version: reopened", func(t *testing.T) {
		probed := mid
		probed.MotionMean = 0.2
		probed.Probe = &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version - 1}
		r, ok := ProbeCandidate(probed, ProbeSignals{}, m)
		assert.True(t, ok)
		assert.Equal(t, ProbeYDIFOutOfRange, r)
	})
	t.Run("a failed probe does not block a retry at a new model", func(t *testing.T) {
		probed := ComplexityMetrics{FramesSampled: 0, Probe: &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version, Failed: true}}
		r, ok := ProbeCandidate(probed, ProbeSignals{}, m)
		assert.True(t, ok)
		assert.Equal(t, ProbeNoMetrics, r)
	})
}
