package httpapi

import "testing"

// The segment-name route regex must accept 4+ digit indices. ffmpeg's
// hls_segment_filename zero-pads to a 3-digit MINIMUM, so segment 1000+
// (≥ ~50 min at 3s/seg) is 4 digits; a \d{3} cap 404'd every deep-resume /
// long-playback segment before it ever reached the transcoder.
func TestVODSegmentNameRE_DeepSegmentsRoute(t *testing.T) {
	cases := []struct {
		file  string
		match bool
		idx   string // expected captured segment index ("" = init / n/a)
	}{
		{"stream_0_000.m4s", true, "000"},
		{"stream_0_005.m4s", true, "005"},
		{"stream_0_999.m4s", true, "999"},
		{"stream_0_1000.m4s", true, "1000"}, // the fix: 4 digits (>50 min) must route
		{"stream_0_2443.m4s", true, "2443"},
		{"stream_1_1500.ts", true, "1500"},
		{"stream_0_init.mp4", true, ""}, // init segment
		{"stream_0_1000.txt", false, ""},
		{"stream_x_100.m4s", false, ""},
	}
	for _, c := range cases {
		m := vodSegmentNameRE.FindStringSubmatch(c.file)
		if (m != nil) != c.match {
			t.Errorf("%s: matched=%v, want %v", c.file, m != nil, c.match)
			continue
		}
		if c.match && c.idx != "" && m[2] != c.idx {
			t.Errorf("%s: captured idx=%q, want %q", c.file, m[2], c.idx)
		}
	}
}

func TestAudioSegmentNameRE_DeepSegmentsRoute(t *testing.T) {
	for _, f := range []string{
		"audio_0_000.m4s", "audio_1_999.m4s",
		"audio_0_1000.m4s", "audio_2_4000.m4s", "audio_0_init.mp4",
	} {
		if !audioSegmentNameRE.MatchString(f) {
			t.Errorf("%s must route (deep audio segment)", f)
		}
	}
	if audioSegmentNameRE.MatchString("audio_0_100.txt") {
		t.Errorf("bad audio segment ext must not route")
	}
}
