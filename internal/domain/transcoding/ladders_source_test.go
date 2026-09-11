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

func TestCapLadderToScreen(t *testing.T) {
	in := []HLSProfile{{Name: "480p", Height: 480}, {Name: "720p", Height: 720}, {Name: "1080p", Height: 1080}, {Name: "2160p", Height: 2160}}
	heights := func(l []HLSProfile) []int {
		out := []int{}
		for _, p := range l {
			out = append(out, p.Height)
		}
		return out
	}
	assert.Equal(t, []int{480, 720, 1080}, heights(CapLadderToScreen(in, 768)), "a 768 px screen stops at 1080p")
	assert.Equal(t, []int{480, 720, 1080, 2160}, heights(CapLadderToScreen(in, 1440)), "1440 needs the 2160p rung")
	assert.Equal(t, []int{480, 720, 1080, 2160}, heights(CapLadderToScreen(in, 4320)), "taller than every rung: unchanged")
	assert.Equal(t, in, CapLadderToScreen(in, 0))
}

func TestCapLadderToH264Level(t *testing.T) {
	in := []HLSProfile{{Name: "480p", Height: 480}, {Name: "720p", Height: 720}, {Name: "1080p", Height: 1080}, {Name: "2160p", Height: 2160}}
	heights := func(l []HLSProfile) []int {
		out := []int{}
		for _, p := range l {
			out = append(out, p.Height)
		}
		return out
	}
	assert.Equal(t, []int{480, 720, 1080}, heights(CapLadderToH264Level(in, 41)), "level 4.1 stops at 1080p")
	assert.Equal(t, []int{480, 720, 1080}, heights(CapLadderToH264Level(in, 42)))
	assert.Equal(t, []int{480, 720}, heights(CapLadderToH264Level(in, 31)), "level 3.1 stops at 720p")
	assert.Equal(t, []int{480}, heights(CapLadderToH264Level(in, 30)))
	assert.Equal(t, []int{480, 720, 1080}, heights(CapLadderToH264Level(in, 50)), "level 5.0 cannot carry 2160p")
	assert.Equal(t, []int{480, 720, 1080, 2160}, heights(CapLadderToH264Level(in, 51)))
	assert.Equal(t, in, CapLadderToH264Level(in, 0), "no cap declared: unchanged")
	assert.Equal(t, "4.0", H264LevelString(40))
	assert.Equal(t, "5.1", H264LevelString(H264LevelIdcFor(2160)))
	assert.Equal(t, 41, H264LevelIdcFor(1080))
}

func TestCapLadderToVideoBitrate(t *testing.T) {
	in := []HLSProfile{
		{Name: "480p", Height: 480, VideoBitrate: 1500, AudioBitrate: 192},
		{Name: "720p", Height: 720, VideoBitrate: 3500, AudioBitrate: 192},
		{Name: "1080p", Height: 1080, VideoBitrate: 7000, AudioBitrate: 192},
	}
	out := CapLadderToVideoBitrate(in, 4000)
	assert.Equal(t, []HLSProfile{in[0], in[1], {Name: "1080p", Height: 1080, VideoBitrate: 4000, AudioBitrate: 192}}, out,
		"the rung above the ceiling is clamped, the others untouched")
	assert.Equal(t, int64(7000), in[2].VideoBitrate, "input not mutated")

	tight := CapLadderToVideoBitrate(in, 2000)
	assert.Equal(t, []HLSProfile{in[0], {Name: "720p", Height: 720, VideoBitrate: 2000, AudioBitrate: 192}}, tight,
		"1080p clamps onto 720p's bitrate and is dropped")

	floor := CapLadderToVideoBitrate(in, 100)
	assert.Equal(t, []HLSProfile{{Name: "480p", Height: 480, VideoBitrate: 100, AudioBitrate: 192}}, floor, "the lowest rung survives")
	assert.Equal(t, in, CapLadderToVideoBitrate(in, 0))
}
