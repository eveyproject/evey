package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The argv IS the behavior: per-stream override order ("-c copy" before
// "-c:a aac") is what keeps video untouched while audio re-encodes, and
// "-ignore_unknown" is what keeps an exotic data track from failing the
// whole production. A reorder here is a product change, not a refactor.
// "-rematrix_maxval" is the stereo fold's headroom: this file is written once
// and kept, so a clipped fold here is permanent (see audio_downmix.go).
func TestBuildCompatArgs(t *testing.T) {
	args := BuildCompatArgs("/media/film.mkv", "/cache/compat/out.mkv.tmp")
	assert.Equal(t, []string{
		"-hide_banner", "-v", "error", "-nostdin", "-y",
		"-i", "/media/film.mkv",
		"-map", "0", "-ignore_unknown",
		"-map_metadata", "0",
		"-map_chapters", "0",
		"-c", "copy",
		"-c:a", "aac", "-ac", "2", "-b:a", "192k",
		"-rematrix_maxval", "1.0000",
		"-f", "matroska",
		"/cache/compat/out.mkv.tmp",
	}, args)
}
