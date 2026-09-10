package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func TestHevcTranscodeCodecString(t *testing.T) {
	assert.Equal(t, "hvc1.1.6.L93.B0", hevcTranscodeCodecString(0, false), "no height keeps the old permissive string")
	assert.Equal(t, "hvc1.1.6.L93.B0", hevcTranscodeCodecString(720, false))
	assert.Equal(t, "hvc1.1.6.L123.B0", hevcTranscodeCodecString(1080, false))
	assert.Equal(t, "hvc1.1.6.L150.B0", hevcTranscodeCodecString(1440, false))
	assert.Equal(t, "hvc1.2.4.L153.B0", hevcTranscodeCodecString(2160, true), "Main10 for the HDR passthrough")
	assert.Equal(t, "avc1.640028", videoCodecAttrFor("h264", 2160, true), "only HEVC carries a profile/level")
}

// A transcoded HEVC ladder advertises each rung's own level; the HDR
// passthrough adds Main10 and VIDEO-RANGE on every transcoded rung.
func TestMaster_HevcTranscodeRungs_CodecsAndVideoRange(t *testing.T) {
	g := NewManifestGenerator()
	in := ManifestInput{
		MediaID:          "m1",
		DurationSec:      100,
		ChosenVideoCodec: "hevc",
		ChosenAudioCodec: "aac",
		Profiles: []transcoding.HLSProfile{
			{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 3300, AudioBitrate: 192},
			{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 10000, AudioBitrate: 192},
		},
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `CODECS="hvc1.1.6.L123.B0,mp4a.40.2"`)
	assert.Contains(t, out, `CODECS="hvc1.1.6.L153.B0,mp4a.40.2"`)
	assert.NotContains(t, out, "VIDEO-RANGE", "an SDR transcode carries no range")

	in.TranscodeVideoRange = "PQ"
	out, err = g.Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `CODECS="hvc1.2.4.L153.B0,mp4a.40.2",VIDEO-RANGE=PQ`)
	assert.Contains(t, out, `CODECS="hvc1.2.4.L123.B0,mp4a.40.2",VIDEO-RANGE=PQ`)
}
