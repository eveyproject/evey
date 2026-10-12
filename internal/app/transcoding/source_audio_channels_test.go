package transcoding_test

import (
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
)

// The wasm-audio path hands the browser/receiver the ORIGINAL Dolby track to
// decode itself. On 2026-07-30 a Chromecast on a low-end operator box was given an
// E-AC-3 7.1 track: 8 channels to decode and fold, which the box could not
// sustain - silent picture, then the receiver killed by the box. The server
// needs the channel count to refuse that hand-off, so this is the fact it
// reads.

func TestSourceAudioChannels(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "eac3", Channels: new(8)},
		{StreamType: "audio", Codec: "ac3", Channels: new(6)},
		{StreamType: "subtitle", Codec: "subrip"},
		{StreamType: "audio", Codec: "aac"}, // channel count unknown
	}

	t.Run("reads the channels of the SELECTED audio track", func(t *testing.T) {
		// The index is audio-relative: subtitle and video streams must not shift it.
		if got := apptranscoding.SourceAudioChannels(streams, 0); got != 8 {
			t.Errorf("track 0 = %d, want 8", got)
		}
		if got := apptranscoding.SourceAudioChannels(streams, 1); got != 6 {
			t.Errorf("track 1 = %d, want 6", got)
		}
	})

	t.Run("unknown channel count reads 0, never a guess", func(t *testing.T) {
		// 0 must NOT be mistaken for "few channels": callers gate on a maximum,
		// and inventing a number here would silently re-enable the failing path.
		if got := apptranscoding.SourceAudioChannels(streams, 2); got != 0 {
			t.Errorf("unprobed track = %d, want 0", got)
		}
	})

	t.Run("out-of-range and empty inputs are 0", func(t *testing.T) {
		if got := apptranscoding.SourceAudioChannels(streams, 9); got != 0 {
			t.Errorf("out of range = %d, want 0", got)
		}
		if got := apptranscoding.SourceAudioChannels(nil, 0); got != 0 {
			t.Errorf("nil streams = %d, want 0", got)
		}
	})

	t.Run("a nil stream in the slice does not panic", func(t *testing.T) {
		withNil := []*library.MediaStream{nil, {StreamType: "audio", Codec: "ac3", Channels: new(6)}}
		if got := apptranscoding.SourceAudioChannels(withNil, 0); got != 6 {
			t.Errorf("= %d, want 6", got)
		}
	})
}
