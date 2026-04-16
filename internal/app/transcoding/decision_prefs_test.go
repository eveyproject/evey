package transcoding_test

import (
	"testing"

	apptranscode "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

// --- ParseTranscodingPreferences ---

func TestParseTranscodingPreferences_Empty(t *testing.T) {
	tp := apptranscode.ParseTranscodingPreferences(nil)
	assert.True(t, tp.AllowDirectPlay)
	assert.True(t, tp.AllowDirectStream)
	assert.True(t, tp.AllowTranscoding)
	assert.Equal(t, "", tp.PreferredVideoCodec)
	assert.Equal(t, 0, tp.MaxAudioChannels)
}

func TestParseTranscodingPreferences_Full(t *testing.T) {
	m := map[string]string{
		"preferred_video_codec": "HEVC",
		"preferred_audio_codec": "Opus",
		"max_audio_channels":    "2",
		"subtitle_burn_in":      "Always",
		"allow_direct_play":     "false",
		"allow_direct_stream":   "true",
		"allow_transcoding":     "false",
	}
	tp := apptranscode.ParseTranscodingPreferences(m)
	assert.Equal(t, "hevc", tp.PreferredVideoCodec)
	assert.Equal(t, "opus", tp.PreferredAudioCodec)
	assert.Equal(t, 2, tp.MaxAudioChannels)
	assert.Equal(t, "always", tp.SubtitleBurnIn)
	assert.False(t, tp.AllowDirectPlay)
	assert.True(t, tp.AllowDirectStream)
	assert.False(t, tp.AllowTranscoding)
}

func TestParseTranscodingPreferences_BadChannels_Ignored(t *testing.T) {
	tp := apptranscode.ParseTranscodingPreferences(map[string]string{"max_audio_channels": "lolno"})
	assert.Equal(t, 0, tp.MaxAudioChannels)
}

// --- allow_direct_play = false forces transcode ---

func TestPref_AllowDirectPlayFalse_ForcesTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.AllowDirectPlay = false
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Contains(t, d.TranscodeReasons, "direct_play_disabled")
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
}

// --- allow_direct_stream = false: remux case upgrades to transcode ---

func TestPref_AllowDirectStreamFalse_ForcesTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.AllowDirectStream = false
	// matroska container not supported by default web profile -> normally direct_stream
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "matroska", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Contains(t, d.TranscodeReasons, "direct_stream_disabled")
}

// --- allow_transcoding = false surfaces via TranscodeDisallowed ---

func TestPref_AllowTranscodingFalse_Disallowed(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.AllowTranscoding = false
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.True(t, apptranscode.TranscodeDisallowed(d, prefs))
}

func TestPref_AllowTranscodingFalse_NoTranscode_Allowed(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.AllowTranscoding = false
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.False(t, apptranscode.TranscodeDisallowed(d, prefs))
}

// --- preferred_video_codec ---

func TestPref_PreferredVideoCodec_ForcesTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredVideoCodec = "hevc"
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "hevc", d.VideoCodec)
	assert.Contains(t, d.TranscodeReasons, "preferred_video_codec")
}

func TestPref_PreferredVideoCodec_MatchesSource_NoTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredVideoCodec = "h264"
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.NotContains(t, d.TranscodeReasons, "preferred_video_codec")
}

func TestPref_PreferredVideoCodec_Auto_Noop(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredVideoCodec = "auto"
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "direct_play", d.PlayMethod)
}

// --- preferred_audio_codec ---

func TestPref_PreferredAudioCodec_ForcesTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredAudioCodec = "opus"
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Equal(t, "opus", d.AudioCodec)
	assert.Contains(t, d.TranscodeReasons, "preferred_audio_codec")
}

// --- max_audio_channels pref ---

func TestPref_MaxAudioChannels_ForcesDownmix(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(6), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.MaxAudioChannels = 2
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_channels_exceeded")
	assert.Equal(t, "aac", d.AudioCodec)
}

func TestPref_MaxAudioChannels_WithinLimit_NoTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.MaxAudioChannels = 2
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision)
}

// --- subtitle_burn_in pref ---

func TestPref_SubtitleBurnIn_Always(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "subrip", IsDefault: true}, // text sub normally "external"
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SubtitleBurnIn = "always"
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "burn", d.SubtitleDecision)
}

func TestPref_SubtitleBurnIn_Never(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle", IsDefault: true}, // bitmap sub normally "burn"
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SubtitleBurnIn = "never"
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "external", d.SubtitleDecision)
}

func TestPref_SubtitleBurnIn_Auto_Default(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle", IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SubtitleBurnIn = "auto"
	d := apptranscode.MakePlaybackDecisionWithPrefs(streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "burn", d.SubtitleDecision)
}
