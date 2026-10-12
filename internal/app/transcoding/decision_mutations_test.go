package transcoding_test

import (
	"testing"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

// Targets decision.go mutations: boundary, negation, arithmetic.

// line 56:17 boundary `userCapKbps > 0` : userCap=0 means "no user cap"; client cap must apply.
func TestResolveMaxBitrate_UserCapZero_ClientCapApplies(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxBitrate:       5000,
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 8,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(10000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	assert.Contains(t, d.TranscodeReasons, "bitrate_exceeded")
	assert.Equal(t, int64(5000), d.VideoBitrate)
}

// line 79:39 boundary `MaxWidth > 0` : MaxWidth=0 means unlimited, width>0 must not trigger.
func TestResolution_MaxWidthZero_NoTranscode(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxWidth:         0,
		MaxHeight:        0,
		MaxAudioChannels: 8,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(7680), Height: intPtr(4320), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	assert.NotContains(t, d.TranscodeReasons, "resolution_exceeded")
	assert.Equal(t, "copy", d.VideoDecision)
}

// line 84:41 boundary `MaxHeight > 0` already covered by width=0 case above for height=0 too.

// line 97:21 negation `VideoDecision == "transcode"` : direct_play must NOT assign VideoCodec.
func TestDirectPlay_VideoCodecEmpty(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)
	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Equal(t, "", d.VideoCodec)
}

// line 97:52 negation `VideoCodec == ""` : HDR-only transcode must default VideoCodec to h264.
func TestHDRTranscode_DefaultsToH264(t *testing.T) {
	hdr := "HDR10"
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), HDRType: &hdr, Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
	assert.Equal(t, "h264", d.VideoCodec)
}

// line 111:50 boundary `MaxAudioChannels > 0` : unlimited (0) must not trigger channel exceed.
func TestAudio_MaxChannelsZero_NoTranscode(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 0,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(8), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	assert.NotContains(t, d.TranscodeReasons, "audio_channels_exceeded")
	assert.Equal(t, "copy", d.AudioDecision)
}

// line 111:69 boundary `*s.Channels > MaxAudioChannels` : equal must NOT transcode.
func TestAudio_ChannelsEqualMax_NoTranscode(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 2,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	assert.NotContains(t, d.TranscodeReasons, "audio_channels_exceeded")
	assert.Equal(t, "copy", d.AudioDecision)
}

// line 129:35 arithmetic `totalBitrate / 1000` : bit-to-kbps conversion.
// Stream bitrate 5 Mbps = 5_000_000 bits. Cap 8000 kbps → direct_play.
// Mutation `* 1000` makes 5e9, always over cap.
func TestBitrate_UnderCap_DirectPlay(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), Bitrate: int64Ptr(192000), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 8000)
	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.NotContains(t, d.TranscodeReasons, "bitrate_exceeded")
}

// line 130:49 boundary `totalBitrateKbps > effectiveMaxBitrate` : equal must NOT exceed.
func TestBitrate_ExactlyAtCap_DirectPlay(t *testing.T) {
	// 8_000_000 bits / 1000 = 8000 kbps == cap 8000
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(8000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 8000)
	assert.NotContains(t, d.TranscodeReasons, "bitrate_exceeded")
	assert.Equal(t, "direct_play", d.PlayMethod)
}

// line 134:19 negation `VideoCodec == ""` : bitrate-only transcode must set VideoCodec=h264.
func TestBitrateOnlyTranscode_VideoCodecDefault(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(40000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 8000)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "h264", d.VideoCodec)
	assert.Equal(t, int64(8000), d.VideoBitrate)
}

// line 159:18 negation `Container == ""` : transcode path must set Container.
func TestTranscode_SetsContainer(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)
	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "mp4", d.Container)
}

// line 197:21 boundary+negation `len(containers) > 0` : empty list returns "mp4".
func TestPreferredContainer_EmptyFallsBackToMP4(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{}, // empty
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 8,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "mp4", d.Container)
}
