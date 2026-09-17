package ffmpeg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// One demuxer per stream type and no interleave wait: the two properties
// that keep the remux memory-bounded and the tracks side by side.
func TestBuildReinterleaveArgs(t *testing.T) {
	args := strings.Join(buildReinterleaveArgs("/m/a.mkv", "/m/a.mkv.tmp"), " ")
	assert.Equal(t, 3, strings.Count(args, "-i /m/a.mkv"), "three demuxers of the same file")
	assert.Contains(t, args, "-map 0:v? -map 1:a? -map 2:s?")
	assert.Contains(t, args, "-max_interleave_delta 0")
	assert.Contains(t, args, "-c copy")
	assert.True(t, strings.HasSuffix(args, "-f matroska /m/a.mkv.tmp"))
}

// The remux reorders streams (video, audio, subtitles); what must not change
// is each stream's packet count.
func TestSamePacketCounts(t *testing.T) {
	assert.True(t, samePacketCounts(map[int]int{0: 100, 1: 40, 2: 5}, map[int]int{0: 100, 2: 40, 1: 5}))
	assert.False(t, samePacketCounts(map[int]int{0: 100, 1: 40}, map[int]int{0: 100, 1: 39}))
	assert.False(t, samePacketCounts(map[int]int{0: 100, 1: 40}, map[int]int{0: 100}))
}
