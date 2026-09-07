package transcoding

// The AAC encoder is a BYTE input: native ffmpeg aac and libfdk_aac at the same
// bitrate produce different audio. Measured on prod bytes (Arrival, AC-3 5.1
// folded to stereo, 2026-09-07) the difference is +2 to +3 dB of headroom above
// 1 kHz, which is exactly the band a listener reported as saturated.
//
// So the encoder MUST reach the cache key. Without that, an image swap (or a
// rollback) would leave a session's first half encoded by one and its second
// half by the other under the same hash - the class of bug the downmix policy
// already had to be forked for in September.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func decisionFPWithEncoder(enc string) DecisionFingerprint {
	return DecisionFingerprint{
		MediaID:      "m1",
		SourcePath:   "/media/x.mkv",
		VideoCodec:   "h264",
		AudioEncoder: enc,
	}
}

func audioFPWithEncoder(enc string) AudioFingerprint {
	return AudioFingerprint{
		MediaID:      "m1",
		SourcePath:   "/media/x.mkv",
		AudioCodec:   "aac",
		AudioBitrate: 192,
		AudioEncoder: enc,
	}
}

func TestDecisionFingerprint_EncoderForksTheCache(t *testing.T) {
	native := decisionFPWithEncoder("").Hash()
	libfdk := decisionFPWithEncoder("libfdk_aac").Hash()
	assert.NotEqual(t, native, libfdk, "two encoders must never share a cache entry")
}

func TestAudioFingerprint_EncoderForksTheCache(t *testing.T) {
	native := audioFPWithEncoder("").Hash()
	libfdk := audioFPWithEncoder("libfdk_aac").Hash()
	assert.NotEqual(t, native, libfdk, "an audio rendition follows the same rule as the muxed ladder")
}

// The other half of the contract, and the reason the hash is conditional: a box
// that never had libfdk keeps every segment it already made. Naming the native
// encoder explicitly must hash exactly like naming nothing, because the bytes
// are the same - otherwise shipping this feature would silently invalidate the
// transcode cache of every installation on earth.
func TestFingerprints_NativeEncoderDoesNotInvalidateExistingCaches(t *testing.T) {
	assert.Equal(t, decisionFPWithEncoder("").Hash(), decisionFPWithEncoder("aac").Hash())
	assert.Equal(t, audioFPWithEncoder("").Hash(), audioFPWithEncoder("aac").Hash())
}

func TestNonNativeAudioEncoder(t *testing.T) {
	assert.Empty(t, nonNativeAudioEncoder(""), "unset is the native encoder")
	assert.Empty(t, nonNativeAudioEncoder("aac"), "the native encoder hashes to nothing")
	assert.Equal(t, "libfdk_aac", nonNativeAudioEncoder("libfdk_aac"))
}

// An unwired resolver must behave exactly like the code did before this
// feature: native encoder, and therefore the historical hash.
func TestPlanResolver_AudioEncoderIDIsNilSafe(t *testing.T) {
	assert.Empty(t, (&PlanResolver{}).audioEncoderID())
	pr := &PlanResolver{AudioEncoderID: func() string { return "libfdk_aac" }}
	assert.Equal(t, "libfdk_aac", pr.audioEncoderID())
}
