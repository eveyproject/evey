package transcoding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
)

// IsImageSubtitle is the single bitmap-detection source of truth for the
// application layer (the playback decision gates burn-in on it). It must match
// both the ffprobe canonical codec_name and the ffmpeg encoder aliases - in
// particular dvb_subtitle (the canonical name the prober stores), whose absence
// would silently route a DVB bitmap to the sidecar path that can't render it.
func TestIsImageSubtitle(t *testing.T) {
	image := []string{
		"hdmv_pgs_subtitle", "pgssub",
		"dvd_subtitle", "dvdsub",
		"dvb_subtitle", "dvbsub",
		"xsub",
		" DVB_SUBTITLE ", // trimmed + case-insensitive
	}
	for _, c := range image {
		assert.True(t, apptranscode.IsImageSubtitle(c), "expected %q to be an image subtitle", c)
	}
	text := []string{"subrip", "srt", "ass", "ssa", "mov_text", "webvtt", "", "hdmv_text_subtitle"}
	for _, c := range text {
		assert.False(t, apptranscode.IsImageSubtitle(c), "expected %q NOT to be an image subtitle", c)
	}
}
