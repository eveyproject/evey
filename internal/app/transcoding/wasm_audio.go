package transcoding

import (
	"strings"

	"github.com/evey-project/evey/internal/domain/library"
)

// The wasm-audio path serves the AC-3/E-AC-3 track untouched as an out-of-band
// rendition the client decodes itself. Asked by the master handler, the segment
// fingerprint and the playback decision, which must agree: when the first two
// disagreed (2026-07-29) the master advertised muxed audio over audio-free
// segments and the Chromecast answered 104 then 905.

// WasmAudioMaxChannels: 7.1 decoded then folded client-side killed the receiver
// on a low-end operator box (2026-07-30).
const WasmAudioMaxChannels = 6

func WasmAudioCodecSupported(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "ac3", "eac3":
		return true
	}
	return false
}

// Unknown (0) is not treated as small: an unprobed track is exactly the case
// where we cannot promise the device copes.
func WasmAudioChannelsOK(channels int) bool {
	return channels > 0 && channels <= WasmAudioMaxChannels
}

// WasmAudioApplies answers for the track the client will hear (ffmpeg's 0:a:N).
func WasmAudioApplies(streams []*library.MediaStream, audioTrack int) bool {
	return WasmAudioCodecSupported(SourceAudioCodec(streams, audioTrack)) &&
		WasmAudioChannelsOK(SourceAudioChannels(streams, audioTrack))
}
