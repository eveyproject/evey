package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func TestAvcCodecString(t *testing.T) {
	cases := []struct {
		profile string
		level   int
		want    string
	}{
		{"High", 40, "avc1.640028"},     // == the legacy hardcoded default
		{"Main", 31, "avc1.4d001f"},     // Main @ L3.1
		{"Baseline", 30, "avc1.42001e"}, // Baseline @ L3.0
		{"Constrained Baseline", 30, "avc1.42001e"},
		{"High", 51, "avc1.640033"},                 // High @ L5.1 (4K-capable)
		{"weird-future-profile", 40, "avc1.640028"}, // unknown → High default
	}
	for _, c := range cases {
		assert.Equal(t, c.want, avcCodecString(c.profile, c.level),
			"profile=%q level=%d", c.profile, c.level)
	}
}

// The master must advertise the SOURCE avc1.PPCCLL for the copy rung (so Safari/
// hls.js accept its real level) while transcoded rungs keep the codec default.
func TestMaster_PerRungCopyCodec(t *testing.T) {
	g := NewManifestGenerator()
	in := ManifestInput{
		MediaID:          "m1",
		DurationSec:      100,
		StreamToken:      "tok",
		ChosenVideoCodec: "h264",
		ChosenAudioCodec: "aac",
		CopyVideoCodec:   "avc1.640033", // source High @ L5.1
		Profiles: []transcoding.HLSProfile{
			{Name: "1080p-copy", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 192, Copy: true},
			{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 128, Copy: false},
		},
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `CODECS="avc1.640033,mp4a.40.2"`, "copy rung advertises the source avc1.PPCCLL")
	assert.Contains(t, out, `CODECS="avc1.640020,mp4a.40.2"`, "a transcode rung declares its encode: 720p at an unknown rate (60) is High 3.2")
}

func TestHevcCodecString(t *testing.T) {
	cases := []struct {
		profile string
		level   int
		want    string
	}{
		{"Main 10", 153, "hvc1.2.4.L153.B0"}, // Main 10 @ L5.1 (the 4K HDR shape)
		{"Main 10", 120, "hvc1.2.4.L120.B0"}, // Main 10 @ L4.0
		{"Main", 123, "hvc1.1.6.L123.B0"},    // Main @ L4.1
		{"main 10", 150, "hvc1.2.4.L150.B0"}, // case-insensitive
		{"Rext", 153, ""},                    // unknown profile → no string (gated)
	}
	for _, c := range cases {
		assert.Equal(t, c.want, hevcCodecString(c.profile, c.level),
			"profile=%q level=%d", c.profile, c.level)
	}
}

// Phase H1: an HEVC HDR copy rung must advertise the source hvc1 string AND
// the VIDEO-RANGE attribute - Apple's authoring spec requires it on HDR
// variants, and a PQ stream without it renders washed-out on some Safari/tvOS.
func TestMaster_HevcCopyRung_CodecAndVideoRange(t *testing.T) {
	g := NewManifestGenerator()
	in := ManifestInput{
		MediaID:          "m1",
		DurationSec:      100,
		StreamToken:      "tok",
		ChosenVideoCodec: "hevc",
		ChosenAudioCodec: "eac3",
		CopyVideoCodec:   "hvc1.2.4.L153.B0",
		CopyVideoRange:   "PQ",
		Profiles: []transcoding.HLSProfile{
			{Name: "2160p-copy", Width: 3840, Height: 2160, VideoBitrate: 25000, AudioBitrate: 640, Copy: true},
		},
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `CODECS="hvc1.2.4.L153.B0,ec-3"`, "copy rung advertises the source hvc1 string")
	assert.Contains(t, out, `,VIDEO-RANGE=PQ`, "HDR copy rung carries VIDEO-RANGE")
}

// SDR copy (CopyVideoRange empty) and transcoded rungs must NOT carry
// VIDEO-RANGE: transcoded HDR is tonemapped to SDR.
func TestMaster_NoVideoRangeOnSDROrTranscode(t *testing.T) {
	g := NewManifestGenerator()
	in := ManifestInput{
		MediaID:          "m1",
		DurationSec:      100,
		ChosenVideoCodec: "h264",
		ChosenAudioCodec: "aac",
		CopyVideoCodec:   "avc1.640028",
		Profiles: []transcoding.HLSProfile{
			{Name: "1080p-copy", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 192, Copy: true},
			{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 128},
		},
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	assert.NotContains(t, out, "VIDEO-RANGE", "no VIDEO-RANGE on SDR copy or transcoded rungs")

	// Even with a range set, a TRANSCODED rung never carries it (tonemapped SDR).
	in.CopyVideoRange = "PQ"
	in.Profiles = []transcoding.HLSProfile{
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 128},
	}
	out, err = g.Master(in)
	require.NoError(t, err)
	assert.NotContains(t, out, "VIDEO-RANGE", "transcoded rungs are SDR - never VIDEO-RANGE")
}

// Phase A2: with audio passthrough the segments carry the SOURCE audio codec,
// so the master must advertise ac-3/ec-3 - a mp4a claim over ac-3 bytes (or
// the reverse) is refused by Safari's strict CODECS validation.
func TestMaster_AudioPassthroughCodec(t *testing.T) {
	g := NewManifestGenerator()
	in := ManifestInput{
		MediaID:          "m1",
		DurationSec:      100,
		StreamToken:      "tok",
		ChosenVideoCodec: "h264",
		ChosenAudioCodec: "eac3", // servedAudioCodec on an E-AC-3 passthrough
		CopyVideoCodec:   "avc1.640029",
		Profiles: []transcoding.HLSProfile{
			{Name: "1080p-copy", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 192, Copy: true},
		},
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `CODECS="avc1.640029,ec-3"`, "passthrough rung advertises the real audio codec")
	assert.NotContains(t, out, "mp4a.40.2", "no stale AAC claim anywhere in a passthrough master")
}

// An MP3 passthrough rung must advertise mp4a.6B: that is the
// objectTypeIndication ffmpeg writes into the esds when it copies MP3 into
// fMP4 (0x6B, MPEG-1 Audio - read off the produced bytes, 2026-09-06). The
// codec-attr default would have claimed mp4a.40.2 (AAC-LC) over MP3 samples.
func TestMaster_MP3PassthroughCodec(t *testing.T) {
	g := NewManifestGenerator()
	in := ManifestInput{
		MediaID:          "m1",
		DurationSec:      100,
		StreamToken:      "tok",
		ChosenVideoCodec: "h264",
		ChosenAudioCodec: "mp3", // servedAudioCodec on an MP3 passthrough
		CopyVideoCodec:   "avc1.640029",
		Profiles: []transcoding.HLSProfile{
			{Name: "1080p-copy", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 128, Copy: true},
		},
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `CODECS="avc1.640029,mp4a.6B"`)
	assert.NotContains(t, out, "mp4a.40.2", "no stale AAC claim over MP3 bytes")
}
