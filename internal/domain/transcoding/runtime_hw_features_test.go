package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The runtime drops the low-power encoder features unless the operator opted
// in; the calibration's route proofs and cost evidence read the same
// derivation, so what is measured is what runs.
func TestRuntimeHWFeatures(t *testing.T) {
	probed := []string{FeatureTonemapOpenCL, LowPowerFeature("h264"), BFramesFeature("h264", false), LowPowerFeature("hevc")}
	assert.Equal(t, []string{FeatureTonemapOpenCL, BFramesFeature("h264", false)}, RuntimeHWFeatures(probed, false))
	assert.Equal(t, probed, RuntimeHWFeatures(probed, true))
	assert.Equal(t, []string{}, RuntimeHWFeatures(nil, false), "no features is still a list, never nil")
}
