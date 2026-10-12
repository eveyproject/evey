package httpapi

import (
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
)

// serveWasmAudioMaster hands the client the ORIGINAL Dolby track to decode
// itself. On 2026-07-30 an E-AC-3 7.1 title (8 channels) was handed to a
// Chromecast on a a low-end operator box: the audio arrived in fits, the picture ran
// silent, and the box killed the receiver - twice. The ceiling below returns
// those titles to the muxed-AAC master, which every client plays from the first
// second. This pins the DECISION; the handler wiring reads the same helper.

func TestWasmAudioChannelCeiling(t *testing.T) {
	cases := []struct {
		name     string
		channels *int
		accept   bool
	}{
		{"stereo Dolby is fine", new(2), true},
		{"5.1 is the widest we hand over", new(6), true},
		{"7.1 is refused - the prod failure", new(8), false},
		{"an unprobed track is refused, not assumed small", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			streams := []*library.MediaStream{
				{StreamType: "video", Codec: "h264"},
				{StreamType: "audio", Codec: "eac3", Channels: tc.channels},
			}
			ch := apptranscoding.SourceAudioChannels(streams, 0)
			accepted := ch != 0 && ch <= wasmAudioMaxChannels
			if accepted != tc.accept {
				t.Errorf("channels=%v accepted=%v, want %v", tc.channels, accepted, tc.accept)
			}
		})
	}
}
