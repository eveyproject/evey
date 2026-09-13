package transcoding

import "testing"

func TestSourceClassFor(t *testing.T) {
	cases := []struct {
		name   string
		codec  string
		height int
		want   string
	}{
		{"hevc_4k_exact", "hevc", 2160, "hevc/2160p"},
		{"hevc_4k_letterboxed", "hevc", 1600, "hevc/1440p"}, // 3840x1600 source
		{"h264_qhd", "h264", 1440, "h264/1440p"},
		{"h264_fhd_exact", "h264", 1080, "h264/1080p"},
		{"av1_fhd", "av1", 1080, "av1/1080p"},
		{"empty_codec_fallback", "", 2160, "any/2160p"},
		{"unknown_codec_fallback", "weirdcodec", 2160, "any/2160p"},
		{"avc1_normalises_to_h264", "avc1", 1080, "h264/1080p"},
		{"hev1_normalises_to_hevc", "hev1", 2160, "hevc/2160p"},
		{"h265_alias", "h265", 2160, "hevc/2160p"},
		{"vp9_kept", "vp9", 1080, "vp9/1080p"},
		{"hd_720_below_floor", "hevc", 720, ""},
		{"sd_480_below_floor", "h264", 480, ""},
		{"zero_height", "hevc", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SourceClassFor(c.codec, c.height); got != c.want {
				t.Errorf("(%q, %d) → %q; want %q", c.codec, c.height, got, c.want)
			}
		})
	}
}

func TestSourceClassMatchesLegacy(t *testing.T) {
	for _, in := range []string{"1080p", "1440p", "2160p"} {
		if !SourceClassMatchesLegacy(in) {
			t.Errorf("legacy class %q should match", in)
		}
	}
	for _, in := range []string{"hevc/2160p", "h264/1080p", "any/1440p", "720p", ""} {
		if SourceClassMatchesLegacy(in) {
			t.Errorf("class %q should NOT be considered legacy", in)
		}
	}
}

func TestNormaliseSourceCodec(t *testing.T) {
	// Canonicalises codec aliases into the buckets HWCanDecode / SourceClassFor
	// / cap overrides all key on. Unknown and empty collapse to "any" so a typo'd
	// codec can't fork the bucket space.
	cases := map[string]string{
		"h264": "h264", "avc": "h264", "avc1": "h264", "AVC1": "h264", " h264 ": "h264",
		"hevc": "hevc", "h265": "hevc", "hev1": "hevc", "hvc1": "hevc", "HEVC": "hevc",
		"av1": "av1", "av01": "av1",
		"vp9":        "vp9",
		"":           "any",
		"   ":        "any",
		"mpeg2video": "mpeg2video", "vc1": "vc1", "vp8": "vp8",
		"theora": "any", "dirac": "any",
	}
	for in, want := range cases {
		if got := normaliseSourceCodec(in); got != want {
			t.Errorf("normaliseSourceCodec(%q) = %q; want %q", in, got, want)
		}
	}
}
