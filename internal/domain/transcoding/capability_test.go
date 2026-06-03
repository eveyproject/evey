package transcoding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The admin calibration view must always show hw_10bit_transcode, including when
// it is false — that is the case that matters on a weak host (gate active). An
// omitempty tag would drop the false bool and hide the gate, so guard against a
// regression that re-adds it.
func TestEncodeCapability_HW10BitTranscode_AlwaysSerialized(t *testing.T) {
	b, err := json.Marshal(EncodeCapability{HWBackend: "vaapi"}) // HW10BitTranscode zero == false
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	v, present := m["hw_10bit_transcode"]
	assert.True(t, present, "hw_10bit_transcode must be serialized even when false (no omitempty)")
	assert.Equal(t, false, v)
}

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
