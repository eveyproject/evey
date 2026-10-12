package httpapi

import (
	"context"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

func TestStreamIsTenBit(t *testing.T) {
	bd10, bd8 := 10, 8
	video := domainlibrary.StreamTypeVideo
	cases := []struct {
		name string
		s    *domainlibrary.MediaStream
		want bool
	}{
		{"nil", nil, false},
		{"Main 10 profile", &domainlibrary.MediaStream{StreamType: video, Profile: "Main 10"}, true},
		{"Main profile (8-bit)", &domainlibrary.MediaStream{StreamType: video, Profile: "Main"}, false},
		{"bit_depth 10", &domainlibrary.MediaStream{StreamType: video, BitDepth: &bd10}, true},
		{"bit_depth 8", &domainlibrary.MediaStream{StreamType: video, BitDepth: &bd8}, false},
	}
	for _, c := range cases {
		if got := streamIsTenBit(c.s); got != c.want {
			t.Errorf("%s: streamIsTenBit=%v want %v", c.name, got, c.want)
		}
	}

	// sourceVideoTenBit must pick the video stream, ignoring audio.
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeAudio, Profile: "LC"},
		{StreamType: video, Profile: "Main 10"},
	}
	if !sourceVideoTenBit(streams) {
		t.Error("sourceVideoTenBit must detect the Main 10 video stream")
	}
	if sourceVideoTenBit([]*domainlibrary.MediaStream{{StreamType: video, Profile: "Main"}}) {
		t.Error("8-bit Main video stream must not be flagged 10-bit")
	}
}

// The rendition serves the copy rung and, when the encoder's reorder delay can
// be compensated, the transcoded ladder too (one AAC encode for every rung).
// Inverse telecine changes the frame rate, so it stays muxed.
func TestAudioRenditionsForCopyAndTranscode(t *testing.T) {
	probes := func(h *hlsVODServe) *hlsVODServe {
		h.frameRateProbe = func(context.Context, string) float64 { return 24 }
		h.encoderReorderProbe = func(context.Context, string, string, string, []string) (int, bool) { return 2, true }
		return h
	}
	h := probes(&hlsVODServe{audioRenditionEnabled: true})
	copyDec := domaintranscoding.PlaybackDecision{VideoDecision: "copy", AudioDecision: "transcode"}
	transcodeDec := domaintranscoding.PlaybackDecision{VideoDecision: "transcode", AudioDecision: "transcode"}
	if !h.audioRenditionsFor(copyDec, nil) {
		t.Error("video COPY + audio transcode is the hybrid rung: renditions must be on")
	}
	if !h.audioRenditionsFor(transcodeDec, nil) {
		t.Error("a transcoded ladder shares one audio rendition when the encoder delay is compensated")
	}
	tru := true
	telecine := []*domainlibrary.MediaStream{{StreamType: "video", Codec: "h264", IdetTelecine: &tru}}
	if h.audioRenditionsFor(transcodeDec, telecine) {
		t.Error("inverse telecine changes the frame rate: keep the audio muxed")
	}
	if (&hlsVODServe{audioRenditionEnabled: true}).audioRenditionsFor(transcodeDec, nil) {
		t.Error("without the encoder probes a transcode keeps its audio muxed")
	}
	if probes(&hlsVODServe{audioRenditionEnabled: false}).audioRenditionsFor(copyDec, nil) {
		t.Error("the operator opt-out must still win")
	}
}
