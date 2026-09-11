package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

func hdrHevcStreams() []*library.MediaStream {
	return []*library.MediaStream{videoStream(func(s *library.MediaStream) {
		s.Codec = "hevc"
		s.ColorTransfer = new("smpte2084")
		s.BitDepth = new(10)
	})}
}

func hevcCapable() transcoding.EncodeCapability {
	c := vaapiCapable()
	c.HWHDR10Encode = true
	return c
}

func transcodeFor(clientCodecs ...string) transcoding.PlaybackDecision {
	return transcoding.PlaybackDecision{
		VideoDecision:     "transcode",
		VideoCodec:        "h264",
		TranscodeReasons:  []string{"bitrate_exceeded"},
		ClientVideoCodecs: clientCodecs,
	}
}

func TestEffectiveVideoCodec_AutoHEVC(t *testing.T) {
	sdr := []*library.MediaStream{videoStream(func(s *library.MediaStream) { s.Codec = "h264" })}

	t.Run("client decodes HEVC and the GPU encodes it", func(t *testing.T) {
		codec, accel, _, _, usable, _ := hwResolver(vaapiCapable()).EffectiveVideoCodec(transcodeFor("h264", "hevc"), sdr)
		assert.Equal(t, "hevc", codec)
		assert.True(t, usable)
		assert.Equal(t, "vaapi", accel)
	})
	t.Run("client without HEVC stays on H.264", func(t *testing.T) {
		codec, _, _, _, _, _ := hwResolver(vaapiCapable()).EffectiveVideoCodec(transcodeFor("h264"), sdr)
		assert.Equal(t, "h264", codec)
	})
	t.Run("GPU without HEVC stays on H.264", func(t *testing.T) {
		c := vaapiCapable()
		c.HWCodecs = []string{"h264"}
		codec, _, _, _, _, _ := hwResolver(c).EffectiveVideoCodec(transcodeFor("h264", "hevc"), sdr)
		assert.Equal(t, "h264", codec)
	})
	t.Run("HW accel off stays on H.264 - no libx265 for live HLS", func(t *testing.T) {
		pr := hwResolver(vaapiCapable())
		pr.HWAccelEnabled = func() bool { return false }
		codec, _, _, _, usable, _ := pr.EffectiveVideoCodec(transcodeFor("h264", "hevc"), sdr)
		assert.Equal(t, "h264", codec)
		assert.False(t, usable)
	})
	t.Run("an explicit preference is never overridden", func(t *testing.T) {
		d := transcodeFor("h264", "hevc")
		d.VideoCodec = "h264"
		d.TranscodeReasons = append(d.TranscodeReasons, "preferred_video_codec")
		codec, _, _, _, _, _ := hwResolver(vaapiCapable()).EffectiveVideoCodec(d, sdr)
		assert.Equal(t, "h264", codec)
	})
	t.Run("the route agrees with the codec", func(t *testing.T) {
		assert.True(t, hwResolver(vaapiCapable()).HWRouteUsable(transcodeFor("h264", "hevc"), SourceDims{Codec: "h264"}))
	})
}

func TestHDRPassthroughWanted(t *testing.T) {
	d := transcodeFor("h264", "hevc") // an HDR client: no hdr_tonemapping_needed reason

	assert.True(t, hwResolver(hevcCapable()).HDRPassthroughWanted(d, hdrHevcStreams()))

	t.Run("needs the probed Main10 encoder", func(t *testing.T) {
		assert.False(t, hwResolver(vaapiCapable()).HDRPassthroughWanted(d, hdrHevcStreams()))
	})
	t.Run("needs an HEVC client", func(t *testing.T) {
		assert.False(t, hwResolver(hevcCapable()).HDRPassthroughWanted(transcodeFor("h264"), hdrHevcStreams()))
	})
	t.Run("an SDR client tone-maps as before", func(t *testing.T) {
		sdrClient := transcodeFor("h264", "hevc")
		sdrClient.TranscodeReasons = []string{"hdr_tonemapping_needed"}
		assert.False(t, hwResolver(hevcCapable()).HDRPassthroughWanted(sdrClient, hdrHevcStreams()))
	})
	t.Run("an SDR source has nothing to pass through", func(t *testing.T) {
		sdr := []*library.MediaStream{videoStream(func(s *library.MediaStream) { s.ColorTransfer = new("bt709") })}
		assert.False(t, hwResolver(hevcCapable()).HDRPassthroughWanted(d, sdr))
	})
	t.Run("a copy needs no transcode at all", func(t *testing.T) {
		copyD := d
		copyD.VideoDecision = "copy"
		assert.False(t, hwResolver(hevcCapable()).HDRPassthroughWanted(copyD, hdrHevcStreams()))
	})
}

func TestFingerprint_HDRPassthrough(t *testing.T) {
	d := transcodeFor("h264", "hevc")
	req := PlaybackRequest{MediaID: "m1", SourcePath: "/f.mkv", Streams: hdrHevcStreams(), SubtitleBurnInTrack: -1}
	profiles := []transcoding.HLSProfile{{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 15000, AudioBitrate: 192}}

	fp := hwResolver(hevcCapable()).Fingerprint(context.Background(), req, d, profiles)
	require.True(t, fp.HDRPassthrough)
	assert.False(t, fp.HDRTonemap, "the passthrough never tone-maps")
	assert.Equal(t, "hevc", fp.VideoCodec)
	assert.Equal(t, "pq", fp.HDRTransfer)

	t.Run("no GPU: tone-map to SDR H.264 instead", func(t *testing.T) {
		pr := hwResolver(hevcCapable())
		pr.HWAccelEnabled = func() bool { return false }
		fp := pr.Fingerprint(context.Background(), req, d, profiles)
		assert.False(t, fp.HDRPassthrough)
		assert.Equal(t, "h264", fp.VideoCodec)
	})
}

func TestMakePlaybackDecision_CarriesClientVideoCodecs(t *testing.T) {
	d := MakePlaybackDecisionWithPrefs(context.Background(), hdrHevcStreams(), "mkv",
		transcoding.ClientProfile{VideoCodecs: []string{" H264", "HEVC "}, Containers: []string{"mp4"}}, 0, DefaultTranscodingPreferences())
	assert.Equal(t, []string{"h264", "hevc"}, d.ClientVideoCodecs)
}

// A copy verdict whose copy rung is unavailable (no keyframe index yet) still
// re-encodes: for an HDR client on a Main10 GPU that re-encode keeps the HDR
// instead of tone-mapping to SDR, which is what judging the passthrough on
// the copy verdict used to do.
func TestDecide_CopyBlockedHDRClientKeepsPassthrough(t *testing.T) {
	req := PlaybackRequest{
		MediaID: "m1", SourcePath: "/f.mkv", Streams: hdrHevcStreams(), Container: "mkv",
		Client: transcoding.ClientProfile{VideoCodecs: []string{"h264", "hevc"}, AudioCodecs: []string{"aac"}, Containers: []string{"mp4"}, SupportsHDR: true},
		Prefs:  DefaultTranscodingPreferences(), AudioTrack: -1, SubtitleBurnInTrack: -1,
	}
	d := hwResolver(hevcCapable()).Decide(context.Background(), req)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_copy_unavailable")
	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
	fp := hwResolver(hevcCapable()).Fingerprint(context.Background(), req, d, []transcoding.HLSProfile{{Name: "2160p", Width: 3840, Height: 2160, VideoBitrate: 15000, AudioBitrate: 192}})
	assert.True(t, fp.HDRPassthrough)

	t.Run("no Main10 encoder: tone-map as before", func(t *testing.T) {
		d := hwResolver(vaapiCapable()).Decide(context.Background(), req)
		assert.Equal(t, "transcode", d.VideoDecision)
		assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
	})
}
