package ffmpeg

import (
	"context"
	"slices"
	"testing"
)

// The container's index answers when it vouches for the track; otherwise
// ffprobe reads the packets (here a missing binary, which reads as no cues).
func TestSubtitleCueStarts_IndexFirstThenFFprobe(t *testing.T) {
	trusted := true
	d := NewIntroDetector("", "/nonexistent/ffprobe").WithSubtitleCueIndex(
		func(context.Context, string) ([]int64, bool) {
			if !trusted {
				return nil, false
			}
			return []int64{1200, 5400}, true
		})

	cues, err := d.SubtitleCueStarts(context.Background(), "/m/a.mkv")
	if err != nil || !slices.Equal(cues, []int64{1200, 5400}) {
		t.Fatalf("index answer = %v, %v", cues, err)
	}
	trusted = false
	cues, err = d.SubtitleCueStarts(context.Background(), "/m/a.mkv")
	if err != nil || cues != nil {
		t.Fatalf("ffprobe fallback = %v, %v, want the probe's empty answer", cues, err)
	}
}
