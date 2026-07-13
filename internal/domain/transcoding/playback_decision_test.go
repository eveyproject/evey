package transcoding

import "testing"

// The default web profile must not claim 8 audio channels: no browser decodes
// AAC 7.1 in MSE (Chrome tops out at 5.1). Claiming 8 makes the pipeline emit
// `-ac 8`, the SourceBuffer refuses the stream, and the title simply never
// starts - observed on a real Dolby Atmos 7.1 title (H.264 + TrueHD): the audio
// rendition came out `aac 8ch 7.1 @422k` and playback never began.
//
// 6 (5.1) is the real browser ceiling, and it is what the muxed ladder has always
// emitted in practice. A client that genuinely decodes more says so itself
// (MediaCaps); this is only the DEFAULT for a client that declares nothing.
func TestDefaultWebProfile_AudioChannelsCappedAtSurround(t *testing.T) {
	if DefaultWebProfile.MaxAudioChannels > 6 {
		t.Errorf("DefaultWebProfile.MaxAudioChannels = %d: no browser decodes AAC beyond 5.1 (6ch); "+
			"a 7.1 source then yields `-ac 8` and the stream never plays",
			DefaultWebProfile.MaxAudioChannels)
	}
	if DefaultWebProfile.MaxAudioChannels < 6 {
		t.Errorf("MaxAudioChannels = %d: 5.1 (6ch) IS decodable - don't downmix it away",
			DefaultWebProfile.MaxAudioChannels)
	}
}
