package transcoding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The admin calibration view must always show hw_10bit_transcode, including when
// it is false - that is the case that matters on a weak host (gate active). An
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

func TestAV1SoftwareViable(t *testing.T) {
	assert.False(t, EncodeCapability{}.AV1SoftwareViable(), "0 ⇒ not viable")
	assert.True(t, EncodeCapability{SoftwareAV1MaxHeight: 720}.AV1SoftwareViable())
}

func TestSoftwareCeilingFor_AV1UsesOwnCeiling(t *testing.T) {
	c := EncodeCapability{SoftwareMaxHeight: 1080, SoftwareAV1MaxHeight: 480}
	// AV1 is heavier → its own (lower) ceiling, not the H.264 1080.
	assert.Equal(t, 480, c.SoftwareCeilingFor("av1"))
	assert.Equal(t, 480, c.SoftwareCeilingFor("AV1"))
	// Other codecs keep the H.264 software ceiling.
	assert.Equal(t, 1080, c.SoftwareCeilingFor("h264"))
	assert.Equal(t, 1080, c.SoftwareCeilingFor("hevc"))
	assert.Equal(t, 1080, c.SoftwareCeilingFor(""))
}

func TestSoftwareCeilingFor_AV1Uncalibrated_FallsBackToH264(t *testing.T) {
	// Defensive: AV1 with no calibrated ceiling falls back to the H.264 number
	// (the handler should not route AV1 to software in this state anyway).
	c := EncodeCapability{SoftwareMaxHeight: 1080}
	assert.Equal(t, 1080, c.SoftwareCeilingFor("av1"))
	// Fully uncalibrated → 720 floor.
	assert.Equal(t, 720, EncodeCapability{}.SoftwareCeilingFor("av1"))
}

// TestHWCanDecodeSource_OldIGPUEncodesH264ButNotHEVC locks the exact
// production case: a pre-Skylake Intel iGPU HW-encodes H.264 but
// has NO HEVC hardware decoder. A codec alone names its 4:2:0 8-bit decode
// class; the resolver relies on
// HWCanDecodeSource("hevc", false, "") == false on such a box to keep an HEVC
// source off -hwaccel instead of stalling on an undecodable stream.
func TestHWCanDecodeSource_OldIGPUEncodesH264ButNotHEVC(t *testing.T) {
	// No backend → false regardless of the decode list.
	assert.False(t, EncodeCapability{HWDecodeCodecs: []string{"hevc", "hevc10"}}.HWCanDecodeSource("hevc", false, ""))

	oldIGPU := EncodeCapability{
		HWBackend:      "vaapi",
		HWCodecs:       []string{"h264"}, // encodes H.264
		HWDecodeCodecs: []string{"h264"}, // but cannot decode HEVC
	}
	assert.False(t, oldIGPU.HWCanDecodeSource("hevc", false, ""), "an old iGPU cannot HW-decode HEVC")
	assert.False(t, oldIGPU.HWCanDecodeSource("h265", false, ""), "h265 alias normalises to hevc → also false")
	assert.True(t, oldIGPU.HWCanDecodeSource("h264", false, ""), "H.264 HW decode is supported")
	assert.True(t, oldIGPU.HWCanDecodeSource("avc1", false, ""), "avc1 alias normalises to h264 → true")
}
