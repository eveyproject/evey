package transcoding_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The per-backend twin of HWCanDecodeSource reads a decode list in ONE
// vocabulary, whatever probe generation wrote it: a listed class is
// decodable, an unlisted one is not, and a pre-class row becomes a class row
// through Normalized (once, at load) rather than through a version branch.
func TestHWBackendCap_CanDecodeSource_OneVocabulary(t *testing.T) {
	matrix := transcoding.HWBackendCap{Backend: "vaapi", DecodeCodecs: []string{"h264", "hevc", "hevc10", "hevc:yuv444p"}}
	if !matrix.CanDecodeSource("hevc", true, "yuv420p10le") || !matrix.CanDecodeSource("h264", false, "yuv420p") || !matrix.CanDecodeSource("hevc", false, "yuv444p") {
		t.Error("a matrix row answers from its measured classes")
	}
	if matrix.CanDecodeSource("h264", true, "yuv420p10le") || matrix.CanDecodeSource("hevc", false, "gbrp") {
		t.Error("a class the matrix never listed is not decodable")
	}
	// No pixel format known: the codec and the depth name the class.
	if !matrix.CanDecodeSource("hevc", true, "") || !matrix.CanDecodeSource("h264", false, "") {
		t.Error("without a pix_fmt the codec and depth decide")
	}

	class := transcoding.HWBackendCap{Backend: "qsv", DecodeCodecs: []string{"h264", "hevc"}}
	if class.CanDecodeSource("hevc", true, "yuv420p10le") {
		t.Error("a class row without hevc10 does not decode Main10")
	}
	if !class.CanDecodeSource("hevc", false, "yuv420p") || class.CanDecodeSource("hevc", false, "yuv444p") {
		t.Error("a class row decodes 4:2:0 only")
	}
	if class.CanDecodeSource("av1", true, "yuv420p10le") {
		t.Error("ten-bit is only ever HEVC on these backends")
	}

	// A pre-class row (v4) listed "hevc" for Main10 too: the capability's
	// Normalized adds the class to every backend, the gate then reads it.
	preClass := transcoding.EncodeCapability{ProbeVersion: 4, HWBackend: "nvenc", HWDecodeCodecs: []string{"hevc"},
		Backends: []transcoding.HWBackendCap{{Backend: "nvenc", DecodeCodecs: []string{"hevc"}}}}
	if preClass.Backends[0].CanDecodeSource("hevc", true, "yuv420p10le") {
		t.Error("the gate reads the list as written: no version branch")
	}
	norm := preClass.Normalized().Backends[0]
	if !norm.CanDecodeSource("hevc", true, "yuv420p10le") {
		t.Error("a normalized pre-class row: \"hevc\" covered Main10")
	}
	if norm.CanDecodeSource("h264", false, "yuv420p") || norm.CanDecodeSource("hevc", false, "yuv422p") {
		t.Error("a codec never listed, or an unmeasured chroma class, stays on the CPU")
	}
	if len(preClass.Backends[0].DecodeCodecs) != 1 {
		t.Error("Normalized copies the backend list, it never edits the persisted row")
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
