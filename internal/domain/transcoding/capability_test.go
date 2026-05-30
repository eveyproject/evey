package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeFallback_Is720pSoftware(t *testing.T) {
	c := SafeFallback()
	assert.Equal(t, 720, c.SoftwareMaxHeight)
	assert.Zero(t, c.HWMaxHeight)
	assert.Empty(t, c.HWBackend)
}

func TestMaxHeightFor_HWUsableAndCalibrated_UsesHW(t *testing.T) {
	c := EncodeCapability{SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc"}
	assert.Equal(t, 2160, c.MaxHeightFor(true))
}

func TestMaxHeightFor_HWUsableButNotCalibrated_FallsBackToSoftware(t *testing.T) {
	c := EncodeCapability{SoftwareMaxHeight: 1080, HWMaxHeight: 0}
	assert.Equal(t, 1080, c.MaxHeightFor(true))
}

func TestMaxHeightFor_HWNotUsable_AlwaysSoftware(t *testing.T) {
	c := EncodeCapability{SoftwareMaxHeight: 1080, HWMaxHeight: 2160, HWBackend: "nvenc"}
	// Even with HW calibrated, software path is used when the request
	// can't take HW (e.g. unsupported source codec, sidecar burn-in).
	assert.Equal(t, 1080, c.MaxHeightFor(false))
}

func TestMaxHeightFor_Uncalibrated_FallsBackTo720(t *testing.T) {
	c := EncodeCapability{} // zero value
	assert.Equal(t, 720, c.MaxHeightFor(false))
	assert.Equal(t, 720, c.MaxHeightFor(true))
}
