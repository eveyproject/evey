package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// The argv IS the behavior: per-stream override order ("-c copy" before
// "-c:a aac") is what keeps video untouched while audio re-encodes, and
// "-ignore_unknown" is what keeps an exotic data track from failing the
// whole production. A reorder here is a product change, not a refactor.
// The "-af" fold guard matters most here: this file is written once and
// kept, so a clipped fold would be permanent (see audio_downmix.go).
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
		// Derived, not literal: the policy is a measured product decision that
		// has moved twice already (see audio_downmix.go). This test guards the
		// argv ORDER, not the value.
		"-af", transcoding.AudioFoldFilter(0, 2),
		"-f", "matroska",
		"/cache/compat/out.mkv.tmp",
	}, args)
}
