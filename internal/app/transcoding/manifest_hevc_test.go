package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// The codec string of a rung is the level its encode is pinned at: the
// rung's CodecLevel, set by the plan; a rung the plan did not level is
// levelled as the encoder levels it (static software ceiling, 60 fps when
// the rate is unknown). The HEVC constraint byte is the encoders' (0x90).
func TestVideoCodecAttrFor_TheEncodeContract(t *testing.T) {
	levelled := func(w, h, lvl int) transcoding.HLSProfile {
		return transcoding.HLSProfile{Width: w, Height: h, CodecLevel: lvl}
	}
	assert.Equal(t, "avc1.64002a", videoCodecAttrFor("h264", levelled(1920, 1080, 42), false, 60))
	assert.Equal(t, "hvc1.1.6.L156.90", videoCodecAttrFor("hevc", levelled(1920, 1080, 156), false, 24))
	assert.Equal(t, "hvc1.2.4.L153.90", videoCodecAttrFor("hevc", levelled(3840, 2160, 153), true, 24), "Main10 for the HDR passthrough")
	unlevelled := transcoding.HLSProfile{Width: 1920, Height: 1080, VideoBitrate: 5000}
	assert.Equal(t, "avc1.640028", videoCodecAttrFor("h264", unlevelled, false, 25), "1080p25 at 8000/16000: 4.0")
	assert.Equal(t, "avc1.64002a", videoCodecAttrFor("h264", unlevelled, false, 0), "unknown rate counts as 60: 4.2")
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
	assert.Contains(t, out, `CODECS="hvc1.1.6.L123.90,mp4a.40.2"`)
	assert.Contains(t, out, `CODECS="hvc1.1.6.L153.90,mp4a.40.2"`)
	assert.NotContains(t, out, "VIDEO-RANGE", "an SDR transcode carries no range")

	in.TranscodeVideoRange = "PQ"
	out, err = g.Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `CODECS="hvc1.2.4.L153.90,mp4a.40.2",VIDEO-RANGE=PQ`)
	assert.Contains(t, out, `CODECS="hvc1.2.4.L123.90,mp4a.40.2",VIDEO-RANGE=PQ`)
}

// Every codec spelling the plan can carry maps to the CODECS string the
// player probes against: the HEVC aliases share one string, AV1 always
// declares 8-bit (a 10-bit declaration made MSE open a buffer the 8-bit
// segments never matched), VP9 is fixed, and anything unknown is H.264.
func TestVideoCodecAttrFor_EveryCodecSpelling(t *testing.T) {
	hd := transcoding.HLSProfile{Width: 1920, Height: 1080, CodecLevel: 41}
	uhd := transcoding.HLSProfile{Width: 3840, Height: 2160, CodecLevel: 51}
	hdHEVC := transcoding.HLSProfile{Width: 1920, Height: 1080, CodecLevel: 123}
	uhdHEVC := transcoding.HLSProfile{Width: 3840, Height: 2160, CodecLevel: 153}
	for _, c := range []string{"hevc", "h265", "x265"} {
		assert.Equal(t, "hvc1.1.6.L123.90", videoCodecAttrFor(c, hdHEVC, false, 24), c)
		assert.Equal(t, "hvc1.2.4.L153.90", videoCodecAttrFor(c, uhdHEVC, true, 24), c)
	}
	for _, c := range []string{"av1", "av01"} {
		assert.Equal(t, "av01.0.04M.08", videoCodecAttrFor(c, uhd, true, 24), "AV1 declares 8-bit whatever the source depth")
	}
	assert.Equal(t, "vp09.00.10.08", videoCodecAttrFor("vp9", hd, false, 24))
	for _, c := range []string{"", "h264", "avc", "libx264", "mpeg2video", "something-new"} {
		assert.Equal(t, "avc1.640029", videoCodecAttrFor(c, hd, false, 24), c)
		assert.Equal(t, "avc1.640033", videoCodecAttrFor(c, uhd, false, 24), c)
	}
}
