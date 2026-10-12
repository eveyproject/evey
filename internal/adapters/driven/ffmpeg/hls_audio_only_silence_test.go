package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// argValue returns the argument following flag, or "" when flag is absent.
func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// A silence-tail session generates bounded silence INSTEAD of reading the
// source: an audio track that ends before the container's declared duration
// (truncated tail) leaves the VOD playlist promising segments no ffmpeg -ss on
// the source can ever produce. The generator keeps the promise with silent AAC
// segments carrying the same names/numbering, so the player just plays through.
func TestBuildFFmpegAudioOnlyArgs_SilenceTail(t *testing.T) {
	opts := transcoding.AudioProcessOpts{
		OutputDir:          t.TempDir(),
		SourceTrackIdx:     0,
		AudioCodec:         "aac",
		AudioBitrate:       128,
		MaxAudioChannels:   6,
		StartSec:           5742.0,
		StartNumber:        1914,
		SilenceDurationSec: 249.611,
		// FilePath deliberately set: the builder must IGNORE the source in
		// silence mode (the whole point is that reading it there yields nothing).
		FilePath: "/media/films/some.mkv",
		ReadRate: 8,
	}
	args := BuildFFmpegAudioOnlyArgs(opts)

	require.Equal(t, "lavfi", argValue(args, "-f"), "silence input is an anullsrc lavfi graph")
	assert.Contains(t, argValue(args, "-i"), "anullsrc=", "input must be the silence generator")
	assert.Contains(t, argValue(args, "-i"), "r=48000", "silence must match the rendition sample rate")
	assert.Contains(t, argValue(args, "-i"), "cl=5.1", "silence must match the rendition channel layout")
	assert.Equal(t, "249.611", argValue(args, "-t"), "silence must be bounded to the missing tail, never infinite")
	assert.NotContains(t, args, opts.FilePath, "silence mode must not read the source")
	assert.NotContains(t, args, "-ss", "the timeline lands via the rebase pump, not an input seek on a t=0 graph")
	assert.NotContains(t, args, "-readrate", "generating a few minutes of silence is trivial - no pacing")
	assert.Equal(t, "1914", argValue(args, "-start_number"), "segments must keep the absolute VOD numbering")
	assert.Equal(t, "aac", argValue(args, "-c:a"))

	// The seeked-session rebase contract applies: fragments are written t=0
	// session-relative and shifted to absolute time by the pump.
	assert.True(t, needsAudioFMP4Rebase(opts), "a mid-timeline silence tail must be rebased to absolute time")
}

// Channel layouts map to anullsrc's cl= names; unknown counts fall back to
// stereo (matching resolveAudioChannels' conservative default).
func TestBuildFFmpegAudioOnlyArgs_SilenceLayouts(t *testing.T) {
	cases := []struct {
		channels int
		want     string
	}{
		{0, "cl=stereo"},
		{1, "cl=mono"},
		{2, "cl=stereo"},
		{6, "cl=5.1"},
		{8, "cl=7.1"},
		{3, "cl=stereo"},
	}
	for _, c := range cases {
		opts := transcoding.AudioProcessOpts{
			OutputDir:          t.TempDir(),
			AudioCodec:         "aac",
			MaxAudioChannels:   c.channels,
			SilenceDurationSec: 3,
		}
		args := BuildFFmpegAudioOnlyArgs(opts)
		assert.Contains(t, argValue(args, "-i"), c.want, "channels=%d", c.channels)
	}
}
