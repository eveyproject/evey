package transcoding

// How an AAC track is sized: a rung's audio bitrate is quoted for stereo, and
// every channel beyond that costs ~64 kbps (5.1 -> 384k), capped at 512k.
//
// A product rule, not an ffmpeg detail - the encoder and the stream-info panel
// must agree on it to the kbps, or the panel quotes 192 while we encode 384.
const (
	stereoChannels      = 2
	audioKbpsPerChannel = 64
	maxAudioBitrateKbps = 512

	// AudioRenditionBaseKbps is the stereo base of the standalone AAC rendition
	// served next to a COPIED video track. Independent of any ladder rung - on
	// that path there is no ladder, the video passes through untouched.
	AudioRenditionBaseKbps = 128
)

// ServedAudioBitrateKbps is the bitrate an AAC track is actually encoded at.
// Deterministic in (base, channels) - both hashed into the fingerprint, so a
// given fingerprint always yields the same audio bytes.
func ServedAudioBitrateKbps(baseKbps int64, channels int) int64 {
	if channels <= stereoChannels {
		return baseKbps
	}
	scaled := int64(channels) * audioKbpsPerChannel
	if scaled < baseKbps {
		return baseKbps
	}
	if scaled > maxAudioBitrateKbps {
		return maxAudioBitrateKbps
	}
	return scaled
}
