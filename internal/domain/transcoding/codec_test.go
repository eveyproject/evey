package transcoding

import "testing"

func TestCanonicalVideoCodec(t *testing.T) {
	cases := map[string]string{
		"h264":       "h264",
		"H264":       "h264",
		"avc":        "h264",
		"avc1":       "h264",
		"x264":       "h264",
		"libx264":    "h264",
		" h264 ":     "h264",
		"hevc":       "hevc",
		"HEVC":       "hevc",
		"h265":       "hevc",
		"x265":       "hevc",
		"libx265":    "hevc",
		"hvc1":       "hevc",
		"hev1":       "hevc",
		"av1":        "av1",
		"av01":       "av1",
		"libaom-av1": "av1",
		"libsvtav1":  "av1",
		"vp9":        "vp9",
		"vp09":       "vp9",
		"":           "",
		"   ":        "",
		"vp8":        "vp8",
		"mpeg2video": "mpeg2video",
		"mpeg2":      "mpeg2video",
		"vc1":        "vc1",
		"wmv3":       "vc1",
		"theora":     "",
		"dirac":      "",
		"nonsense":   "",
	}
	for in, want := range cases {
		if got := CanonicalVideoCodec(in); got != want {
			t.Errorf("CanonicalVideoCodec(%q) = %q, want %q", in, got, want)
		}
	}
}
