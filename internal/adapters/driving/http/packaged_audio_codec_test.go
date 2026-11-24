package httpapi

import (
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

// A trimmed packager presentation carries one audio track: the session is
// labelled with that track, not the file's first (AC-3 5.1 before the stereo
// AAC the viewer plays). One that carries every track leaves the label alone.
func TestPackagedAudioCodec_NamesTheTrackServed(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		{StreamIndex: 0, StreamType: "video", Codec: "h264"},
		{StreamIndex: 1, StreamType: "audio", Codec: "ac3"},
		{StreamIndex: 2, StreamType: "audio", Codec: "AAC"},
	}
	trimmed := &requestPlan{AudioTrack: 1}
	trimmed.Plan.Delivery.AudioTrimmed = true
	if got := packagedAudioCodec(trimmed, streams); got != "aac" {
		t.Errorf("trimmed to the second audio track: got %q, want aac", got)
	}
	every := &requestPlan{AudioTrack: 1, Plan: apptranscoding.PlaybackPlan{}}
	if got := packagedAudioCodec(every, streams); got != "" {
		t.Errorf("every track carried: got %q, want no label", got)
	}
	if got := packagedAudioCodec(nil, streams); got != "" {
		t.Errorf("no plan: got %q", got)
	}
}
