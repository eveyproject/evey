package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScaleLadderVideoBitrates(t *testing.T) {
	in := []HLSProfile{
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 128},
	}
	out := ScaleLadderVideoBitrates(in, HEVCBitrateNum, HEVCBitrateDen)
	assert.Equal(t, int64(3333), out[0].VideoBitrate)
	assert.Equal(t, int64(1866), out[1].VideoBitrate)
	assert.Equal(t, int64(192), out[0].AudioBitrate, "audio is untouched")
	assert.Equal(t, int64(5000), in[0].VideoBitrate, "the input ladder is not mutated")
	assert.Equal(t, in, ScaleLadderVideoBitrates(in, 1, 1), "1/1 is the identity")
}
