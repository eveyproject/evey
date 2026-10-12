package transcoding_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func TestEncodeCapability_HWSupports(t *testing.T) {
	type tc struct {
		name  string
		cap   transcoding.EncodeCapability
		codec string
		want  bool
	}
	cases := []tc{
		{"no backend", transcoding.EncodeCapability{}, "h264", false},
		{"safe fallback", transcoding.SafeFallback(), "h264", false},
		{"nvenc h264", transcoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc", "av1"}}, "h264", true},
		{"nvenc hevc", transcoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc", "av1"}}, "hevc", true},
		{"nvenc av1", transcoding.EncodeCapability{HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc", "av1"}}, "av1", true},
		// Old QSV: H.264 only - HEVC/AV1 must fall back to software.
		{"qsv old-igpu h264", transcoding.EncodeCapability{HWBackend: "qsv", HWCodecs: []string{"h264"}}, "h264", true},
		{"qsv old-igpu hevc", transcoding.EncodeCapability{HWBackend: "qsv", HWCodecs: []string{"h264"}}, "hevc", false},
		{"qsv old-igpu av1", transcoding.EncodeCapability{HWBackend: "qsv", HWCodecs: []string{"h264"}}, "av1", false},
		// Empty codec defaults to h264.
		{"empty codec → h264", transcoding.EncodeCapability{HWBackend: "vaapi", HWCodecs: []string{"h264"}}, "", true},
		{"empty codec, no h264", transcoding.EncodeCapability{HWBackend: "vaapi", HWCodecs: []string{"hevc"}}, "", false},
		// Case/space tolerance.
		{"case insensitive", transcoding.EncodeCapability{HWBackend: "vaapi", HWCodecs: []string{"hevc"}}, " HEVC ", true},
		// Backend set but empty codec list (degenerate) → no support.
		{"backend, empty codecs", transcoding.EncodeCapability{HWBackend: "nvenc"}, "h264", false},
	}
	for _, c := range cases {
		if got := c.cap.HWSupports(c.codec); got != c.want {
			t.Errorf("%s: HWSupports(%q) = %v, want %v", c.name, c.codec, got, c.want)
		}
	}
}
