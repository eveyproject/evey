package transcoding_test

import (
	"testing"

	apptranscode "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

func TestEmptyStreams_DirectPlay(t *testing.T) {
	d := apptranscode.MakePlaybackDecision(t.Context(), nil, "mp4", transcoding.DefaultWebProfile, 0)
	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Empty(t, d.TranscodeReasons)
}

func TestNonDefaultAudio_Ignored(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: intPtr(6), IsDefault: false},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)
	assert.Equal(t, "copy", d.AudioDecision)
}

func TestUserBitrateLowerThanClient(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(10000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), Bitrate: int64Ptr(192000), IsDefault: true},
	}
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxBitrate:       20000,
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 8,
	}
	// user cap 5000 kbps < client 20000 kbps -> user wins
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 5000)
	assert.Contains(t, d.TranscodeReasons, "bitrate_exceeded")
	assert.Equal(t, int64(5000), d.VideoBitrate)
}

func TestClientBitrateLowerThanUser(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(10000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), Bitrate: int64Ptr(192000), IsDefault: true},
	}
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxBitrate:       3000,
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 8,
	}
	// user cap 20000 > client 3000 -> client wins
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 20000)
	assert.Contains(t, d.TranscodeReasons, "bitrate_exceeded")
	assert.Equal(t, int64(3000), d.VideoBitrate)
}

func TestDefaultBitrates_Applied(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: intPtr(1920), Height: intPtr(1080), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: intPtr(6), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Equal(t, int64(0), d.VideoBitrate, "no cap: the video bitrate follows the source (the ladder raises to it)")
	assert.Equal(t, int64(192), d.AudioBitrate)
}

// When the SELECTED track is the bitmap one, burn must win over a text track's
// "external" regardless of stream order (here the text sub at index 0 is
// evaluated first and sets "external", then the selected PGS at index 1 forces
// burn).
func TestMixedSubtitles_SelectedBitmapWins(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "subrip", IsDefault: false},            // subtitle index 0
		{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle", IsDefault: false}, // subtitle index 1
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SubtitleBurnInTrack = 1 // select the PGS
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "burn", d.SubtitleDecision)
	assert.Equal(t, "transcode", d.VideoDecision)
}

func TestOnlyHeightExceeded(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxWidth:         3840,
		MaxHeight:        720,
		MaxAudioChannels: 8,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "resolution_exceeded")
	assert.Equal(t, 720, d.MaxHeight)
	// Only the height cap binds (1920 < MaxWidth 3840): width must stay unset so
	// the encoder preserves aspect ratio. Without this the "OnlyHeightExceeded"
	// name would pass even if a spurious MaxWidth over-constrained the encode.
	assert.Equal(t, 0, d.MaxWidth)
}

func TestNoBitrate_NoExceed(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 8000)
	assert.Equal(t, "direct_play", d.PlayMethod)
}
