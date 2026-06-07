package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func TestAvcCodecString(t *testing.T) {
	cases := []struct {
		profile string
		level   int
		want    string
	}{
		{"High", 40, "avc1.640028"},              // == the legacy hardcoded default
		{"Main", 31, "avc1.4d001f"},              // Main @ L3.1
		{"Baseline", 30, "avc1.42001e"},          // Baseline @ L3.0
		{"Constrained Baseline", 30, "avc1.42001e"},
		{"High", 51, "avc1.640033"},              // High @ L5.1 (4K-capable)
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
	assert.Contains(t, out, `CODECS="avc1.640028,mp4a.40.2"`, "transcode rung keeps the codec default")
}

