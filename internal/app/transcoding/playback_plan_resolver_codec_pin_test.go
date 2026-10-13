package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// The vc= pin: the copy master's fallback rung keeps the copy rung's codec.
// Without it an HEVC-capable client's transcode was upgraded to a GPU HEVC
// (outputCodecFor) next to an H.264 copy variant - the player's emergency drop
// became an H.264->HEVC decoder swap under starvation (prod, 2026-09-15).
func TestResolver_PinnedVideoCodec_StopsTheHEVCUpgrade(t *testing.T) {
	pr := fallbackResolver(probedNVENC())
	req := PlaybackRequest{
		MediaID: "m1", Streams: sdrStreams("h264", "yuv420p", false),
		Client:              transcoding.ClientProfile{VideoCodecs: []string{"h264", "hevc"}, AudioCodecs: []string{"aac"}, Containers: []string{"mp4"}},
		Prefs:               DefaultTranscodingPreferences(),
		PinnedVideoDecision: "transcode",
	}

	unpinned, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "hevc", unpinned.VideoCodec, "premise: the HEVC-capable client's transcode is upgraded to HEVC")

	req.PinnedVideoCodec = "h264"
	pinned, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "h264", pinned.VideoCodec, "the pin holds the copy rung's codec")
	assert.Equal(t, "h264", pinned.Fingerprint.VideoCodec, "the fingerprint (the encoder's codec) follows the pin")
	assert.Equal(t, "nvenc", pinned.HWAccel, "the pin changes the codec, not the route")
	assert.NotEqual(t, unpinned.FingerprintHash, pinned.FingerprintHash, "different bytes, different cache key")
}

// A pin the client cannot decode is ignored (never forced), and a pin on a copy
// decision changes nothing: copied bytes carry the source codec.
func TestResolver_PinnedVideoCodec_IgnoredWhenUndecodableOrCopy(t *testing.T) {
	pr := fallbackResolver(probedNVENC())
	req := PlaybackRequest{
		MediaID: "m1", Streams: sdrStreams("h264", "yuv420p", false),
		Client:              transcoding.ClientProfile{VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}, Containers: []string{"mp4"}},
		Prefs:               DefaultTranscodingPreferences(),
		PinnedVideoDecision: "transcode",
		PinnedVideoCodec:    "hevc",
	}
	plan, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "h264", plan.VideoCodec, "a codec the client does not decode is never forced")
	assert.NotContains(t, plan.Decision.TranscodeReasons, "video_codec_pinned")

	req.PinnedVideoDecision = ""
	req.PinnedVideoCodec = "hevc"
	req.Client.VideoCodecs = []string{"h264", "hevc"}
	d := pr.Decide(context.Background(), req)
	if d.VideoDecision == "copy" {
		assert.NotContains(t, d.TranscodeReasons, "video_codec_pinned", "a copy is not re-encoded to honour a pin")
	}
}

// The resolved plan carries the file's size and mtime into its cache key: a
// replaced file at the same path resolves to another hash.
func TestResolver_SourceFingerprintForksTheCacheKey(t *testing.T) {
	pr := fallbackResolver(probedNVENC())
	req := PlaybackRequest{
		MediaID: "m1", SourcePath: "/m.mkv", SourceFingerprint: "1000-1",
		Streams:             sdrStreams("h264", "yuv420p", false),
		Client:              transcoding.ClientProfile{VideoCodecs: []string{"h264"}, AudioCodecs: []string{"aac"}, Containers: []string{"mp4"}},
		Prefs:               DefaultTranscodingPreferences(),
		PinnedVideoDecision: "transcode",
	}
	before, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "1000-1", before.Fingerprint.SourceFingerprint)

	req.SourceFingerprint = "1000-2"
	after, err := pr.Resolve(context.Background(), req)
	require.NoError(t, err)
	assert.NotEqual(t, before.FingerprintHash, after.FingerprintHash)
}
