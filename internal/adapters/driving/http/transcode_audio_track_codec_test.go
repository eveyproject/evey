package httpapi

import (
	"context"
	"errors"
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// The passthrough gate must read the SAME track numbering the renditions
// advertise (`-map 0:a:N`): audio streams counted alone, video and subtitles
// skipped, whatever order the prober stored them in.
func TestSourceAudioCodecForTrack_CountsAudioStreamsOnly(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.Video, Codec: "hevc"},
		{StreamType: domainlibrary.Audio, Codec: "eac3", Language: "fre"},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip"},
		nil,
		{StreamType: domainlibrary.Audio, Codec: "mp3", Language: "eng"},
	}
	h := &audioServe{libSvc: &fakeMediaStreamsGetter{streams: streams}}
	ctx := context.Background()
	if got := h.sourceAudioCodecForTrack(ctx, "m1", 0); got != "eac3" {
		t.Errorf("track 0 = %q, want eac3 (the video stream does not count)", got)
	}
	if got := h.sourceAudioCodecForTrack(ctx, "m1", 1); got != "mp3" {
		t.Errorf("track 1 = %q, want mp3 (subtitle and nil entries do not count)", got)
	}
	if got := h.sourceAudioCodecForTrack(ctx, "m1", 2); got != "" {
		t.Errorf("track 2 = %q, want empty past the last audio stream", got)
	}
	// Unreadable streams and an unwired service both mean "unknown", never a guess.
	if got := (&audioServe{libSvc: &fakeMediaStreamsGetter{sErr: errors.New("db")}}).sourceAudioCodecForTrack(ctx, "m1", 0); got != "" {
		t.Errorf("read error = %q, want empty", got)
	}
	if got := (&audioServe{}).sourceAudioCodecForTrack(ctx, "m1", 0); got != "" {
		t.Errorf("no service = %q, want empty", got)
	}
}

func TestAudioServe_HintFor(t *testing.T) {
	if got := (&audioServe{}).hintFor("m1"); got != 0 {
		t.Errorf("no hint source: got %d, want 0", got)
	}
	h := &audioServe{startHint: func(id string) int {
		if id == "m1" {
			return 42
		}
		return 0
	}}
	if got := h.hintFor("m1"); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
	if got := h.hintFor("other"); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}
