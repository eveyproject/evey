package transcoding_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The per-backend twin of HWCanDecodeSource must read a capability row the
// same way across probe generations: a matrix row (v6+) answers from the exact
// decode class, a class row (v5) knows hevc10 apart from hevc, and a pre-class
// row lets "hevc" cover Main10 but refuses chroma it never measured.
func TestHWBackendCap_CanDecodeSource_PerProbeGeneration(t *testing.T) {
	matrix := transcoding.HWBackendCap{Backend: "vaapi", DecodeCodecs: []string{"h264", "hevc", "hevc10", "hevc:yuv444p"}}
	if !matrix.CanDecodeSource("hevc", true, "yuv420p10le", 6) || !matrix.CanDecodeSource("h264", false, "yuv420p", 6) || !matrix.CanDecodeSource("hevc", false, "yuv444p", 6) {
		t.Error("a matrix row answers from its measured classes")
	}
	if matrix.CanDecodeSource("h264", true, "yuv420p10le", 6) || matrix.CanDecodeSource("hevc", false, "gbrp", 6) {
		t.Error("a class the matrix never listed is not decodable")
	}
	// No pixel format known: the matrix row falls back to the codec/ten-bit reading.
	if !matrix.CanDecodeSource("hevc", true, "", 6) || !matrix.CanDecodeSource("h264", false, "", 6) {
		t.Error("without a pix_fmt the codec and depth decide")
	}

	class := transcoding.HWBackendCap{Backend: "qsv", DecodeCodecs: []string{"h264", "hevc"}}
	if class.CanDecodeSource("hevc", true, "yuv420p10le", 5) {
		t.Error("a class row without hevc10 does not decode Main10")
	}
	if !class.CanDecodeSource("hevc", false, "yuv420p", 5) || class.CanDecodeSource("hevc", false, "yuv444p", 5) {
		t.Error("a class row decodes 4:2:0 only")
	}
	if class.CanDecodeSource("av1", true, "yuv420p10le", 5) {
		t.Error("ten-bit is only ever HEVC on these backends")
	}

	preClass := transcoding.HWBackendCap{Backend: "nvenc", DecodeCodecs: []string{"hevc"}}
	if !preClass.CanDecodeSource("hevc", true, "yuv420p10le", 4) {
		t.Error("a pre-class row: \"hevc\" covered Main10")
	}
	if preClass.CanDecodeSource("h264", false, "yuv420p", 4) || preClass.CanDecodeSource("hevc", false, "yuv422p", 4) {
		t.Error("a codec never listed, or unmeasured chroma, stays on the CPU")
	}
}

// The AAC rendition bitrate follows the channel count: stereo keeps the base,
// multichannel scales per channel but never below the base nor above the cap.
func TestServedAudioBitrateKbps(t *testing.T) {
	cases := []struct {
		base     int64
		channels int
		want     int64
	}{
		{192, 2, 192},
		{192, 1, 192},
		{192, 0, 192},
		{128, 6, 384},
		{192, 6, 384},
		{448, 6, 448},
		{192, 8, 512},
		{192, 16, 512},
	}
	for _, tc := range cases {
		if got := transcoding.ServedAudioBitrateKbps(tc.base, tc.channels); got != tc.want {
			t.Errorf("ServedAudioBitrateKbps(%d, %d) = %d, want %d", tc.base, tc.channels, got, tc.want)
		}
	}
}
