package httpapi

import (
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

func audioStreams(n int) []*domainlibrary.MediaStream {
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video"},
	}
	for range n {
		streams = append(streams, &domainlibrary.MediaStream{StreamType: "audio"})
	}
	streams = append(streams, &domainlibrary.MediaStream{StreamType: "subtitle"})
	return streams
}

func TestClampAudioTrack(t *testing.T) {
	tests := []struct {
		name       string
		idx        int
		audioCount int
		want       int
	}{
		{"in range first", 0, 3, 0},
		{"in range middle", 1, 3, 1},
		{"in range last", 2, 3, 2},
		{"out of range high clamps to 0", 3, 3, 0},
		{"way out of range clamps to 0", 999, 2, 0},
		{"negative clamps to 0", -1, 3, 0},
		{"no audio streams clamps to 0", 2, 0, 0},
		{"single audio in range", 0, 1, 0},
		{"single audio out of range", 1, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := clampAudioTrack(tc.idx, audioStreams(tc.audioCount))
			if got != tc.want {
				t.Errorf("clampAudioTrack(%d, %d audio) = %d, want %d", tc.idx, tc.audioCount, got, tc.want)
			}
		})
	}
}

// A clamped (default) audio track must produce a valid "-map 0:a:0", never the
// out-of-range index that would make ffmpeg abort.
func TestClampAudioTrack_FeedsValidFFmpegMap(t *testing.T) {
	h := &transcodeServe{}
	streams := audioStreams(2) // valid indices: 0, 1
	clamped := clampAudioTrack(999, streams)
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{audioTrack: clamped, subtitleTrack: -1}, transcodeAnalysis{})

	var mapped string
	for i, a := range args {
		if a == "-map" && i+1 < len(args) && len(args[i+1]) > 4 && args[i+1][:4] == "0:a:" {
			mapped = args[i+1]
		}
	}
	if mapped != "0:a:0" {
		t.Fatalf("expected audio map 0:a:0 after clamp, got %q", mapped)
	}
}
