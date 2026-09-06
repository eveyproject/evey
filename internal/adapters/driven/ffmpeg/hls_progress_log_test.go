package ffmpeg_test

// ffmpeg's -progress stream is the only record of what an encoder ACHIEVED
// second by second; everything else this server logs (the argv, the spawn, the
// kill, the reach verdict) records what it DECIDED. Its absence on 2026-09-06
// is what left "the encoder was too far behind when the client hung up" a
// deduction instead of a measurement - the stderr ring buffer is 8 KB,
// overwritten continuously, and read only when a session FAILS.

import (
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three builder shapes, because -progress is prepended at their shared
// entry point rather than in each of them: a diagnostic that silently misses
// one code path is worse than no diagnostic.
func progressShapes() map[string]transcoding.HLSProcessOpts {
	return map[string]transcoding.HLSProcessOpts{
		"video copy": {
			OutputDir: "/tmp/t", FilePath: "/f.mkv",
			Profiles:      transcoding.DefaultHLSProfiles[:1],
			VideoDecision: "copy", AudioDecision: "transcode",
		},
		"single transcode": {
			OutputDir: "/tmp/t", FilePath: "/f.mkv",
			Profiles:      transcoding.DefaultHLSProfiles[:1],
			VideoDecision: "transcode", AudioDecision: "transcode",
		},
		"multi variant": {
			OutputDir: "/tmp/t", FilePath: "/f.mkv",
			Profiles:      transcoding.DefaultHLSProfiles,
			VideoDecision: "transcode", AudioDecision: "transcode",
		},
	}
}

func TestBuildFFmpegArgs_ProgressLogOffByDefault(t *testing.T) {
	for name, opts := range progressShapes() {
		t.Run(name, func(t *testing.T) {
			joined := strings.Join(ffmpeg.BuildFFmpegArgs(opts), " ")
			assert.NotContains(t, joined, "-progress",
				"an empty ProgressLogPath must leave the command byte-identical to before")
		})
	}
}

func TestBuildFFmpegArgs_ProgressLogReachesEveryBuilder(t *testing.T) {
	for name, opts := range progressShapes() {
		t.Run(name, func(t *testing.T) {
			opts.ProgressLogPath = "/data/logs/ffmpeg/s.log"
			args := ffmpeg.BuildFFmpegArgs(opts)

			// -progress is a GLOBAL option: it must precede everything the
			// builders emit, including the pre-input -ss of a seek session.
			require.GreaterOrEqual(t, len(args), 2)
			assert.Equal(t, "-progress", args[0])
			// `file:` is load-bearing, not decoration: ffmpeg parses this
			// argument as a URL, so a path carrying a colon would otherwise be
			// read as a protocol name.
			assert.Equal(t, "file:/data/logs/ffmpeg/s.log", args[1])
		})
	}
}

// A seek session opens with `-ss`; the global option still has to come first or
// ffmpeg rejects the command line.
func TestBuildFFmpegArgs_ProgressLogPrecedesTheSeekFlag(t *testing.T) {
	opts := progressShapes()["single transcode"]
	opts.StartSec = 135
	opts.StartNumber = 45
	opts.ProgressLogPath = "/data/logs/ffmpeg/seek.log"

	args := ffmpeg.BuildFFmpegArgs(opts)
	joined := strings.Join(args, " ")
	require.Contains(t, joined, "-ss")
	assert.Equal(t, "-progress", args[0])
	assert.Less(t, indexOf(args, "-progress"), indexOf(args, "-ss"))
	assert.Less(t, indexOf(args, "-progress"), indexOf(args, "-i"))
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}
