package transcoding_test

import (
	"testing"

	apptranscode "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }
func strPtr(v string) *string { return &v }

func TestDirectPlay_H264_AAC_MP4(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Empty(t, d.TranscodeReasons)
}

func TestVideoTranscode_HEVC(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "video_codec_unsupported")
	assert.Equal(t, "h264", d.VideoCodec)
}

func TestAudioTranscode_DTS(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: intPtr(6), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.Equal(t, "aac", d.AudioCodec)
}

func TestBitrateCap(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(3840), Height: intPtr(2160), Bitrate: int64Ptr(40000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), Bitrate: int64Ptr(192000), IsDefault: true},
	}
	// user cap 8 Mbps
	d := apptranscode.MakePlaybackDecision(streams, "mp4", transcoding.DefaultWebProfile, 8000)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "bitrate_exceeded")
	assert.Equal(t, int64(8000), d.VideoBitrate)
}

func TestResolutionCap(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264", "vp9"},
		AudioCodecs:      []string{"aac", "mp3", "opus", "flac"},
		Containers:       []string{"mp4", "webm"},
		MaxWidth:         1920,
		MaxHeight:        1080,
		SupportsHDR:      false,
		MaxAudioChannels: 8,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(3840), Height: intPtr(2160), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", client, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "resolution_exceeded")
	assert.Equal(t, 1920, d.MaxWidth)
	assert.Equal(t, 1080, d.MaxHeight)
}

func TestDirectStream_MKV_Container(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "matroska", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "direct_stream", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "container_incompatible")
	assert.Equal(t, "mp4", d.Container)
}

func TestHDR_NonHDRClient(t *testing.T) {
	hdrType := "HDR10"
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(3840), Height: intPtr(2160), HDRType: &hdrType, Bitrate: int64Ptr(20000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
}

func TestSubtitleBurnIn_PGS(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle", IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "burn", d.SubtitleDecision)
}

func TestSubtitleBurnIn_DVDSub(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "dvd_subtitle", IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "burn", d.SubtitleDecision)
}

func TestTextSubtitle_External(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "subrip", IsDefault: true, IsExternal: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "external", d.SubtitleDecision)
}

func TestMultiReason_Codec_Bitrate_Resolution(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxWidth:         1920,
		MaxHeight:        1080,
		SupportsHDR:      false,
		MaxAudioChannels: 2,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: intPtr(3840), Height: intPtr(2160), Bitrate: int64Ptr(40000000), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: intPtr(6), Bitrate: int64Ptr(1536000), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mkv", client, 8000)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "video_codec_unsupported")
	assert.Contains(t, d.TranscodeReasons, "resolution_exceeded")
	assert.Contains(t, d.TranscodeReasons, "bitrate_exceeded")
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.Contains(t, d.TranscodeReasons, "audio_channels_exceeded")
	assert.Contains(t, d.TranscodeReasons, "container_incompatible")
}

func TestAudioChannelsExceeded(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxWidth:         3840,
		MaxHeight:        2160,
		SupportsHDR:      false,
		MaxAudioChannels: 2,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(6), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", client, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_channels_exceeded")
	assert.Equal(t, "aac", d.AudioCodec)
}

func TestHDRClient_DirectPlay(t *testing.T) {
	hdrType := "HDR10"
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264", "hevc"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxWidth:         3840,
		MaxHeight:        2160,
		SupportsHDR:      true,
		MaxAudioChannels: 8,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: intPtr(3840), Height: intPtr(2160), HDRType: &hdrType, Bitrate: int64Ptr(20000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(streams, "mp4", client, 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Empty(t, d.TranscodeReasons)
}
