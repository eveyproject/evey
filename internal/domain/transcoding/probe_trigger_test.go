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

	// Never classified: NOT a probe candidate - the cheap classifier runs first.
	_, ok = ProbeCandidate(ComplexityMetrics{FramesSampled: 0}, ProbeSignals{}, m)
	assert.False(t, ok, "no metrics at all is the classifier's job, not the probe's")

	// A pre-canonical row (native YDIF, no metric_version): also the classifier's
	// job (it resamples to canonical), never a probe on the native value.
	_, ok = ProbeCandidate(ComplexityMetrics{MotionMean: 4, FramesSampled: 200}, ProbeSignals{}, m)
	assert.False(t, ok)

	// The classifier ran but sampled no motion (fallback row): a probe candidate.
	r, ok = ProbeCandidate(ComplexityMetrics{FramesSampled: 0, SourceHeight: 2160, MetricVersion: CanonicalMetricVersion}, ProbeSignals{}, m)
	assert.True(t, ok)
	assert.Equal(t, ProbeClassifyFailed, r)

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

	t.Run("already probed at this identity: never again", func(t *testing.T) {
		probed := mid
		probed.MotionMean = 0.2 // even out of range
		probed.Probe = &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version, Encoder: TitleProbeEncoderID}
		_, ok := ProbeCandidate(probed, ProbeSignals{CeilingHitSegments: 99, Animation: true}, m)
		assert.False(t, ok)
	})
	t.Run("a failed probe at the current identity is terminal", func(t *testing.T) {
		// The source could not be measured; a later ceiling-hit signal (or the
		// animation weak spot) must NOT re-queue it - only an identity bump does.
		probed := mid
		probed.MotionMean = 0.2
		probed.Probe = &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version, Encoder: TitleProbeEncoderID, Failed: true}
		_, ok := ProbeCandidate(probed, ProbeSignals{CeilingHitSegments: 99, Animation: true}, m)
		assert.False(t, ok, "a failed probe at the current identity is not retried")
	})
	t.Run("probed at an older model version: reopened", func(t *testing.T) {
		probed := mid
		probed.MotionMean = 0.2
		probed.Probe = &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version - 1, Encoder: TitleProbeEncoderID}
		r, ok := ProbeCandidate(probed, ProbeSignals{}, m)
		assert.True(t, ok)
		assert.Equal(t, ProbeYDIFOutOfRange, r)
	})
	t.Run("probed at an older encoder identity: reopened", func(t *testing.T) {
		probed := mid
		probed.MotionMean = 0.2
		probed.Probe = &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version, Encoder: "libx264 old tuning"}
		r, ok := ProbeCandidate(probed, ProbeSignals{}, m)
		assert.True(t, ok)
		assert.Equal(t, ProbeYDIFOutOfRange, r)
	})
	t.Run("a failed probe does not block a retry at a new model", func(t *testing.T) {
		// A classify-failed row (canonical, no motion) probed at an OLDER model
		// version is reopened at the current one.
		probed := ComplexityMetrics{FramesSampled: 0, SourceHeight: 2160, MetricVersion: CanonicalMetricVersion,
			Probe: &TitleProbe{Protocol: TitleProbeProtocol, ModelVersion: m.Version - 1, Encoder: TitleProbeEncoderID, Failed: true}}
		r, ok := ProbeCandidate(probed, ProbeSignals{}, m)
		assert.True(t, ok)
		assert.Equal(t, ProbeClassifyFailed, r)
	})
}
