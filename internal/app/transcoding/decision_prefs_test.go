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
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
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
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "matroska", transcoding.DefaultWebProfile, 0, prefs)
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
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.True(t, apptranscode.TranscodeDisallowed(d, prefs))
}

func TestPref_AllowTranscodingFalse_NoTranscode_Allowed(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.AllowTranscoding = false
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
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
	client := transcoding.DefaultWebProfile
	client.VideoCodecs = []string{"h264", "hevc"}
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", client, 0, prefs)
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
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.NotContains(t, d.TranscodeReasons, "preferred_video_codec")
}

// A preferred_video_codec alias ("h265") must be compared against the
// canonicalised source codec, not the raw ffprobe spelling ("hevc") -
// otherwise a source already encoded in the codec the user wants gets
// needlessly retranscoded just because the two strings differ syntactically.
func TestApplyCodecPreferences_AliasDoesNotForceTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	// hevc-capable client, otherwise DefaultWebProfile's lack of hevc support
	// would force a transcode for an unrelated reason (codec_unsupported).
	hevcProfile := transcoding.DefaultWebProfile
	hevcProfile.VideoCodecs = []string{"hevc"}
	hevcProfile.Containers = []string{"mp4", "mkv"}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredVideoCodec = "h265" // alias of hevc
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", hevcProfile, 0, prefs)
	assert.Equal(t, "copy", d.VideoDecision, "h265 pref on an hevc source must NOT force a transcode")
	assert.NotContains(t, d.TranscodeReasons, "preferred_video_codec")
}

// When the preferred codec genuinely differs from the source, the resulting
// VideoCodec must come out canonicalised too (not the raw pref spelling),
// so downstream consumers (encoder selection) see one consistent name.
func TestApplyCodecPreferences_DifferentCodec_CanonicalisesOutput(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredVideoCodec = "h265" // alias of hevc
	client := transcoding.DefaultWebProfile
	client.VideoCodecs = []string{"h264", "hevc"}
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", client, 0, prefs)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "hevc", d.VideoCodec, "VideoCodec must be canonicalised, not the raw pref spelling")
	assert.Contains(t, d.TranscodeReasons, "preferred_video_codec")
}

func TestPref_PreferredVideoCodec_Auto_Noop(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredVideoCodec = "auto"
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
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
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
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
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
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
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision)
}

// --- subtitle_burn_in pref ---

func TestPref_SubtitleBurnIn_Always(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle", IsDefault: true}, // image sub
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SubtitleBurnIn = "always"
	prefs.SubtitleBurnInTrack = 0 // "always" burns the SELECTED image track
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "burn", d.SubtitleDecision)
}

// "always" cannot burn a TEXT subtitle - the ffmpeg overlay only handles image
// subs - so a selected text sub is delivered as a sidecar (external), NOT burnt
// into a needlessly-transcoded video.
func TestPref_SubtitleBurnIn_Always_SelectedTextIsExternal(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "subrip", IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SubtitleBurnIn = "always"
	prefs.SubtitleBurnInTrack = 0
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "external", d.SubtitleDecision, "text sub under always → sidecar, not burn")
	assert.Equal(t, "copy", d.VideoDecision, "must NOT transcode video to burn a text sub it cannot overlay")
}

func TestPref_SubtitleBurnIn_Never(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(1920), Height: intPtr(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle", IsDefault: true}, // bitmap sub normally "burn"
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SubtitleBurnIn = "never"
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
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
	prefs.SubtitleBurnInTrack = 0 // selected bitmap track → burn
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "burn", d.SubtitleDecision)
}

func fourKStreams() []*library.MediaStream {
	return []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: intPtr(3840), Height: intPtr(2160), Bitrate: int64Ptr(40_000_000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: intPtr(2), Bitrate: int64Ptr(192_000), IsDefault: true},
	}
}

func TestEffectiveMaxBitrateKbps_WifiFallsBackToGeneral(t *testing.T) {
	p := apptranscode.TranscodingPreferences{
		MaxBitrateKbps:         10_000,
		MaxBitrateCellularKbps: 1_500,
		NetworkType:            "wifi",
	}
	assert.Equal(t, 10_000, p.EffectiveMaxBitrateKbps())
}

func TestEffectiveMaxBitrateKbps_CellularUsesCellularCap(t *testing.T) {
	p := apptranscode.TranscodingPreferences{
		MaxBitrateKbps:         10_000,
		MaxBitrateCellularKbps: 1_500,
		NetworkType:            "cellular",
	}
	assert.Equal(t, 1_500, p.EffectiveMaxBitrateKbps())
}

func TestEffectiveMaxBitrateKbps_CellularWithoutCellularCapFallsBack(t *testing.T) {
	p := apptranscode.TranscodingPreferences{
		MaxBitrateKbps: 10_000,
		NetworkType:    "cellular",
	}
	assert.Equal(t, 10_000, p.EffectiveMaxBitrateKbps())
}

func TestEffectiveMaxBitrateKbps_CellularCapNotLowerThanGeneralIgnored(t *testing.T) {
	// If cellular cap is >= general cap, the general cap (smaller-or-equal) still wins.
	p := apptranscode.TranscodingPreferences{
		MaxBitrateKbps:         5_000,
		MaxBitrateCellularKbps: 8_000,
		NetworkType:            "cellular",
	}
	assert.Equal(t, 5_000, p.EffectiveMaxBitrateKbps())
}

func TestEffectiveMaxBitrateKbps_CellularCapUsedWhenNoGeneralCap(t *testing.T) {
	p := apptranscode.TranscodingPreferences{
		MaxBitrateCellularKbps: 1_500,
		NetworkType:            "cellular",
	}
	assert.Equal(t, 1_500, p.EffectiveMaxBitrateKbps())
}

func TestEffectiveMaxBitrateKbps_EmptyNetworkTypeTreatedAsWifi(t *testing.T) {
	p := apptranscode.TranscodingPreferences{
		MaxBitrateKbps:         10_000,
		MaxBitrateCellularKbps: 1_500,
	}
	assert.Equal(t, 10_000, p.EffectiveMaxBitrateKbps())
}

func TestMakePlaybackDecisionWithPrefs_CellularCapTriggersTranscode(t *testing.T) {
	prefs := apptranscode.TranscodingPreferences{
		MaxBitrateKbps:         50_000,
		MaxBitrateCellularKbps: 1_500,
		NetworkType:            "cellular",
	}
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), fourKStreams(), "mp4", transcoding.DefaultWebProfile, prefs.MaxBitrateKbps, prefs)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "bitrate_exceeded")
	assert.Equal(t, int64(1_500), d.VideoBitrate)
}

func TestMakePlaybackDecisionWithPrefs_WifiUsesGeneralCap(t *testing.T) {
	prefs := apptranscode.TranscodingPreferences{
		MaxBitrateKbps:         50_000,
		MaxBitrateCellularKbps: 1_500,
		NetworkType:            "wifi",
	}
	// source total ~40 Mbps, general cap 50 Mbps → no bitrate_exceeded.
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), fourKStreams(), "mp4", transcoding.DefaultWebProfile, prefs.MaxBitrateKbps, prefs)

	assert.NotContains(t, d.TranscodeReasons, "bitrate_exceeded")
}

func TestMakePlaybackDecisionWithPrefs_CellularButNoCellularCapUsesGeneral(t *testing.T) {
	prefs := apptranscode.TranscodingPreferences{
		MaxBitrateKbps: 8_000,
		NetworkType:    "cellular",
	}
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), fourKStreams(), "mp4", transcoding.DefaultWebProfile, prefs.MaxBitrateKbps, prefs)

	assert.Contains(t, d.TranscodeReasons, "bitrate_exceeded")
	assert.Equal(t, int64(8_000), d.VideoBitrate)
}

// A preference the CLIENT cannot decode must not force a transcode into a codec
// it will refuse: the pref is ignored, not enforced.
func TestPref_PreferredVideoCodec_UnsupportedByClientIsIgnored(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredVideoCodec = "hevc" // DefaultWebProfile decodes h264/vp9 only
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.NotContains(t, d.TranscodeReasons, "preferred_video_codec")
	assert.NotEqual(t, "hevc", d.VideoCodec)
}

func TestPref_PreferredAudioCodec_UnsupportedByClientIsIgnored(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredAudioCodec = "eac3" // not in DefaultWebProfile.AudioCodecs
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.NotContains(t, d.TranscodeReasons, "preferred_audio_codec")
}

// The audio preference is judged on the track the client will HEAR, not the
// container's default track, and on canonical codec names.
func TestPref_PreferredAudioCodec_JudgesTheSelectedTrack(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true},
		{StreamType: "audio", Codec: "eac3", Channels: new(6)},
	}
	client := transcoding.DefaultWebProfile
	client.AudioCodecs = []string{"aac", "e-ac-3"}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.PreferredAudioCodec = "e-ac-3"
	prefs.AudioTrack = 1
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", client, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision, "the selected track already is the preferred codec")
	assert.NotContains(t, d.TranscodeReasons, "preferred_audio_codec")

	prefs.AudioTrack = 0
	d = apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", client, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Equal(t, "eac3", d.AudioCodec, "the encoder gets the canonical name")
}
