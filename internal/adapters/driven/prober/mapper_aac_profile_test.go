package prober

import "testing"

// The AAC profile is the AudioSpecificConfig's first object type, named as
// ffprobe names it: explicit SBR / PS signalling makes HE-AAC / HE-AACv2, an
// implicit one stays LC (the frames alone carry it, and LC is what the
// copied track's esds declares). The 31 escape reaches the types past 31.
func TestAACProfileFromASC(t *testing.T) {
	for name, tc := range map[string]struct {
		asc  []byte
		want string
	}{
		"LC 48 kHz stereo":             {[]byte{0x11, 0x90}, "LC"},
		"HE-AAC explicit (AOT 5)":      {[]byte{0x2B, 0x11, 0x88, 0x00}, "HE-AAC"},
		"HE-AACv2 explicit (AOT 29)":   {[]byte{0xEB, 0x09, 0x88, 0x00}, "HE-AACv2"},
		"ELD through the escape (39)":  {[]byte{0xF8, 0xE0}, "ELD"},
		"no config":                    {nil, ""},
		"escape cut short":             {[]byte{0xF8}, ""},
		"an object type ffprobe lacks": {[]byte{0x30, 0x00}, ""}, // AOT 6
	} {
		if got := aacProfileFromASC(tc.asc); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}
