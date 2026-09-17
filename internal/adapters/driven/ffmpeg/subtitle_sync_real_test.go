package ffmpeg

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Gated on a real file: EVEY_SUBSYNC_FILE=<mkv with text subtitles>, and
// EVEY_SUBSYNC_EXPECT=<lag seconds> to pin the reading.
func TestSubtitleSync_RealFile(t *testing.T) {
	file := os.Getenv("EVEY_SUBSYNC_FILE")
	if file == "" {
		t.Skip("EVEY_SUBSYNC_FILE not set")
	}
	lags, err := NewSubtitleSync("ffmpeg", "ffprobe", "").ProbeAudioPictureLag(context.Background(), file)
	require.NoError(t, err)
	require.NotEmpty(t, lags, "a measurement was expected")
	t.Logf("lags: %+v", lags)
	if want := os.Getenv("EVEY_SUBSYNC_EXPECT"); want != "" {
		w, _ := strconv.ParseFloat(want, 64)
		for _, l := range lags {
			assert.InDelta(t, w, l.LagSec, 0.25, "track %d", l.Track)
		}
	}
}
