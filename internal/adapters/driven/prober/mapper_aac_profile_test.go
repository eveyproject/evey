package prober

import (
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
)

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

// The sample rate follows the same signal as the profile: an LC config with a
// Matroska OutputSamplingFrequency (implicit SBR, CELLAR test5) is the core
// rate, as ffprobe and ffmpeg's decoder say; explicit SBR is the doubled one.
func TestMapAACSampleRateFollowsExplicitSBR(t *testing.T) {
	for name, tc := range map[string]struct {
		asc  []byte
		want int
	}{
		"implicit SBR (LC config)": {[]byte{0x13, 0x90}, 22050},
		"explicit SBR (AOT 5)":     {[]byte{0x2B, 0x8A, 0x08, 0x00}, 44100},
	} {
		c := &extmkv.Container{Tracks: []extmkv.Track{{
			Type: extmkv.AudioTrack, Codec: "aac", CodecPrivate: tc.asc,
			Channels: new(uint8(2)), SampleRate: new(22050.0), OutputSampleRate: new(44100.0),
		}}}
		res, ok := mapContainer(c, true)
		if !ok || len(res.Streams) != 1 || res.Streams[0].SampleRate == nil {
			t.Fatalf("%s: not mapped", name)
		}
		if got := *res.Streams[0].SampleRate; got != tc.want {
			t.Errorf("%s: sample rate %d, want %d", name, got, tc.want)
		}
	}
}
