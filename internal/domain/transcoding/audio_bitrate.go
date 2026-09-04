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
	//
	// 192, not 128, since 2026-09-04. The per-channel rule below is right for a
	// layout-PRESERVING encode, where each channel carries its own share of the
	// mix, and wrong for a FOLD: a stereo rendition made from a 5.1 bed carries
	// dialogue, music, effects and both surrounds on two channels, so it needs
	// more bits per channel than the 5.1 it came from, not fewer. The client that
	// declares a stereo output was getting 128k for the whole mix while a browser
	// declaring 5.1 got 384k for the same title. Measured on an AC-3 5.1 film,
	// coding noise under the signal on a quiet ambience passage (the case that
	// suffers first, -38 dBFS): 26.9 dB at 128k, 34.4 dB at 192k. The extra
	// 64 kbps sit next to a 1500-7000 kbps video rung.
	AudioRenditionBaseKbps = 192
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
