package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRaiseLadderToSource(t *testing.T) {
	in := []HLSProfile{
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
		{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 15000, AudioBitrate: 256},
	}
	out := RaiseLadderToSource(in, 70000, 3840, 2160, 0)
	assert.Equal(t, int64(70000), out[1].VideoBitrate, "the source-resolution rung gets the source bitrate")
	assert.Equal(t, int64(17500), out[0].VideoBitrate, "lower rungs scale by pixel count")
	assert.Equal(t, int64(15000), in[1].VideoBitrate, "the input ladder is not mutated")

	capped := RaiseLadderToSource(in, 70000, 3840, 2160, 40000)
	assert.Equal(t, int64(40000), capped[1].VideoBitrate, "the ceiling wins")
	assert.Equal(t, int64(17500), capped[0].VideoBitrate)

	low := RaiseLadderToSource(in, 8000, 3840, 2160, 0)
	assert.Equal(t, in, low, "a thin source never lowers a rung")
	assert.Equal(t, in, RaiseLadderToSource(in, 0, 3840, 2160, 0), "unknown source bitrate is a no-op")
}
