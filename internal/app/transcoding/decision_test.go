package transcoding_test

import (
	"testing"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }

func TestDirectPlay_H264_AAC_MP4(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Empty(t, d.TranscodeReasons)
}

// A file stored in one-track blocks (MediaFile.NeedsRemux) plays nowhere as
// a raw file: the DECISION names it (file_needs_remux) and routes it to the
// segmented session, where the HLS handler answers the typed shed. The codecs
// are innocent: both tracks keep their copy verdicts. One decision, read by
// every client's stream-info and by the plan - no adapter rejudges the file.
func TestNeedsRemux_RoutesToTheSegmentedSessionKeepingCopy(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SourceNeedsRemux = true
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)

	assert.Equal(t, "transcode", d.PlayMethod, "never the raw file")
	assert.Contains(t, d.TranscodeReasons, "file_needs_remux")
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
}

func TestVideoTranscode_HEVC(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "video_codec_unsupported")
	assert.Equal(t, "h264", d.VideoCodec)
}

// A client advertising HEVC under the "h265" alias must match an "hevc" source
// (same codec) and copy, not needlessly transcode.
func TestVideoCopy_H265AliasMatchesHevc(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	client := transcoding.DefaultWebProfile
	client.VideoCodecs = []string{"h264", "h265", "vp9"} // "h265" alias, no literal "hevc"
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)

	assert.Equal(t, "copy", d.VideoDecision, "h265 alias must match hevc source")
	assert.NotContains(t, d.TranscodeReasons, "video_codec_unsupported")
}

func TestAudioCopy_EAC3AliasMatchesEC3(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "eac3", Channels: new(2), IsDefault: true},
	}
	client := transcoding.DefaultWebProfile
	client.AudioCodecs = []string{"aac", "ec-3"} // "ec-3" (RFC-6381) alias of eac3, no literal "eac3"
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)

	assert.Equal(t, "copy", d.AudioDecision, "ec-3 alias must match eac3 source")
	assert.NotContains(t, d.TranscodeReasons, "audio_codec_unsupported")
}

// A client advertising AC-3 under the hyphenated RFC/MIME form "ac-3" must match
// an "ac3" source (same codec, different spelling) and copy, not transcode.
func TestAudioCopy_AC3AliasMatches(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "ac3", Channels: new(6), IsDefault: true},
	}
	client := transcoding.DefaultWebProfile
	client.AudioCodecs = []string{"aac", "ac-3"} // "ac-3" RFC form, no literal "ac3"
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)

	assert.Equal(t, "copy", d.AudioDecision, "ac-3 alias must match ac3 source")
	assert.NotContains(t, d.TranscodeReasons, "audio_codec_unsupported")
}

// A client advertising E-AC-3 under the hyphenated form "e-ac-3" must match an
// "eac3" source (same codec, different spelling) and copy, not transcode.
func TestAudioCopy_EAC3AliasMatches(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "eac3", Channels: new(6), IsDefault: true},
	}
	client := transcoding.DefaultWebProfile
	client.AudioCodecs = []string{"aac", "e-ac-3"} // "e-ac-3" RFC form, no literal "eac3"
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)

	assert.Equal(t, "copy", d.AudioDecision, "e-ac-3 alias must match eac3 source")
	assert.NotContains(t, d.TranscodeReasons, "audio_codec_unsupported")
}

func TestAudioTranscode_DTS(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.Equal(t, "aac", d.AudioCodec)
}

// No audio stream is flagged default: ffmpeg serves the FIRST audio track, so the
// decision must judge that track. The first track is DTS (unsupported) → must
// transcode, not copy it raw (previously the loop judged no track and left copy).
func TestAudioTranscode_NoDefaultTrack_JudgesFirst(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: false},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: false},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.AudioDecision, "first (served) audio track is DTS - judge it, don't copy raw")
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.Equal(t, "aac", d.AudioCodec)
}

// A default track exists (AAC, second stream): it is served, not the first (DTS).
// The decision must judge the default, so a supported default copies even though
// an earlier track is unsupported - the fix must not over-transcode.
func TestAudioCopy_DefaultTrackPreferredOverFirst(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: false},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "copy", d.AudioDecision, "default AAC track is served and supported - copy, ignore the earlier DTS")
}

func TestBitrateCap(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), Bitrate: int64Ptr(40000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), Bitrate: int64Ptr(192000), IsDefault: true},
	}
	// user cap 8 Mbps
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 8000)

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
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "resolution_exceeded")
	assert.Equal(t, 1920, d.MaxWidth)
	assert.Equal(t, 1080, d.MaxHeight)
}

func TestDirectStream_MKV_Container(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "matroska", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "direct_stream", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "container_incompatible")
	assert.Equal(t, "mp4", d.Container)
}

// A Matroska whose tracks are all WebM codecs is a WebM to the browser's
// demuxer, whatever the probe called it: a client that plays WebM direct
// plays it. One non-WebM track, or a client without WebM, and the container
// is incompatible as before (the symmetric cases).
func TestDirectPlay_WebMShapedMatroska(t *testing.T) {
	webm := transcoding.ClientProfile{VideoCodecs: []string{"h264", "vp9"}, AudioCodecs: []string{"aac", "opus"}, Containers: []string{"mp4", "webm"}}
	vp9opus := []*library.MediaStream{
		{StreamType: "video", Codec: "vp9", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "opus", Channels: new(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "subrip"},
	}
	// Probed "matroska" but the file is a .webm (both probers say matroska
	// for a WebM); or probed "webm" outright.
	d := apptranscode.MakePlaybackDecision(t.Context(), vp9opus, "matroska", webm, 0, "/media/Spring (2019).webm")
	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Empty(t, d.TranscodeReasons)
	d = apptranscode.MakePlaybackDecision(t.Context(), vp9opus, "webm", webm, 0)
	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Empty(t, d.TranscodeReasons)

	// The same tracks in a .mkv: served as video/x-matroska, which Firefox
	// refuses - a webm client does not take it (the symmetric case).
	d = apptranscode.MakePlaybackDecision(t.Context(), vp9opus, "matroska", webm, 0, "/media/Sprite Fright (2021).mkv")
	assert.Contains(t, d.TranscodeReasons, "container_incompatible", "an .mkv is not a WebM to the browser")
	d = apptranscode.MakePlaybackDecision(t.Context(), vp9opus, "matroska", webm, 0)
	assert.Contains(t, d.TranscodeReasons, "container_incompatible", "unknown extension: not assumed WebM")

	mixed := []*library.MediaStream{
		{StreamType: "video", Codec: "vp9", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d = apptranscode.MakePlaybackDecision(t.Context(), mixed, "matroska", webm, 0, "/media/x.webm")
	assert.Contains(t, d.TranscodeReasons, "container_incompatible", "an AAC track is not WebM")

	mp4Only := webm
	mp4Only.Containers = []string{"mp4"}
	d = apptranscode.MakePlaybackDecision(t.Context(), vp9opus, "matroska", mp4Only, 0, "/media/x.webm")
	assert.Contains(t, d.TranscodeReasons, "container_incompatible", "the client did not declare webm")

	mkv := webm
	mkv.Containers = []string{"mp4", "mkv"}
	d = apptranscode.MakePlaybackDecision(t.Context(), mixed, "matroska", mkv, 0)
	assert.NotContains(t, d.TranscodeReasons, "container_incompatible", "an mkv client takes any Matroska")
}

func TestHDR_NonHDRClient(t *testing.T) {
	hdrType := "HDR10"
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), HDRType: &hdrType, Bitrate: int64Ptr(20000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
}

func hdrClient() transcoding.ClientProfile {
	return transcoding.ClientProfile{
		VideoCodecs:      []string{"h264", "hevc", "vp9"},
		AudioCodecs:      []string{"aac", "mp3", "opus", "flac"},
		Containers:       []string{"mp4", "webm"},
		SupportsHDR:      true,
		MaxAudioChannels: 8,
	}
}

func TestHDR_HDRClient_DirectPlay(t *testing.T) {
	hdrType := "HDR10"
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), HDRType: &hdrType, Bitrate: new(int64(20000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", hdrClient(), 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
}

func TestHDR_DolbyVision_HDRClient_StillTranscode(t *testing.T) {
	hdrType := "dolby_vision"
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), HDRType: &hdrType, Bitrate: new(int64(20000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", hdrClient(), 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed", "DoVi must tonemap even for an HDR client")
}

// The real-world case: no explicit HDRType (the prober never sets it), HDR is
// detected from ColorSpace bt2020 + the Main 10 profile. Without the source-side
// detection the decision would wrongly treat this as SDR.
func TestHDR_FromColorSpace_NonHDRClient_Transcode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), ColorSpace: new("bt2020nc"), Profile: "Main 10", Bitrate: new(int64(20000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
}

func TestHDR_FromColorSpace_HDRClient_DirectPlay(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), ColorSpace: new("bt2020nc"), Profile: "Main 10", Bitrate: new(int64(20000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", hdrClient(), 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
}

// Dolby Vision profile 5 carries a PQ transfer like plain HDR10, but its
// IPTC2 base layer renders green/purple outside a DoVi pipeline - the DoVi
// configuration record (probed side data) must force the transcode even for
// an HDR-capable client.
func TestHDR_DoViProfile5_HDRClient_StillTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: new(3840), Height: new(2160), ColorSpace: new("bt2020nc"), ColorTransfer: new("smpte2084"), DoViProfile: new(5), Profile: "Main 10", Bitrate: new(int64(20000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", hdrClient(), 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "hdr_tonemapping_needed", "DoVi P5 must tonemap even for an HDR client")
}

// Dolby Vision profile 8.1 has an HDR10-compatible base layer: an HDR-capable
// client may be served the raw stream (the base renders correctly, the RPU is
// simply ignored).
func TestHDR_DoViProfile8Compat_HDRClient_DirectPlay(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: new(3840), Height: new(2160), ColorSpace: new("bt2020nc"), ColorTransfer: new("smpte2084"), DoViProfile: new(8), DoViBLCompat: new(1), Profile: "Main 10", Bitrate: new(int64(20000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", hdrClient(), 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed")
}

// burnPrefs selects the Nth subtitle track for burn-in (auto mode), mirroring the
// client's subtitleTrack param. Burn only fires for the SELECTED bitmap track.
func burnPrefs(track int) apptranscode.TranscodingPreferences {
	p := apptranscode.DefaultTranscodingPreferences()
	p.SubtitleBurnInTrack = track
	return p
}

func TestSubtitleBurnIn_PGS(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle", IsDefault: true},
	}
	// User SELECTED the PGS track (subtitle index 0) → burn.
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, burnPrefs(0))

	assert.Equal(t, "burn", d.SubtitleDecision)
	// Burn-in ⟹ never copy: burning overlays onto decoded frames, impossible on a
	// -c:v copy stream. A copyable H.264 source with a burn-in sub MUST transcode
	// video, else the copy path would silently drop the subtitles.
	assert.Equal(t, "transcode", d.VideoDecision, "burn-in must force video transcode")
	assert.Contains(t, d.TranscodeReasons, "subtitle_burn_in")
}

func TestSubtitleBurnIn_DVDSub(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "dvd_subtitle", IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, burnPrefs(0))

	assert.Equal(t, "burn", d.SubtitleDecision)
	assert.Equal(t, "transcode", d.VideoDecision, "burn-in must force video transcode")
	assert.Contains(t, d.TranscodeReasons, "subtitle_burn_in")
}

// The core of the gate-on-selection fix: a title that merely CARRIES a bitmap
// subtitle, played with none selected (the -1 default), must keep its copy rung
// - not be needlessly transcoded. This is what recovers ~20% of a Blu-ray-rip
// library for remux.
func TestSubtitleBitmap_NotSelected_StaysCopy(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "hdmv_pgs_subtitle", IsDefault: true},
	}
	// No subtitle selected (MakePlaybackDecision → default SubtitleBurnInTrack -1).
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.NotEqual(t, "burn", d.SubtitleDecision, "unselected bitmap sub must not burn")
	assert.Equal(t, "copy", d.VideoDecision, "an unselected bitmap sub must not deny the copy rung")
	assert.NotContains(t, d.TranscodeReasons, "subtitle_burn_in")
}

func TestTextSubtitle_External(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
		{StreamType: "subtitle", Codec: "subrip", IsDefault: true, IsExternal: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "external", d.SubtitleDecision)
	// The guard must NOT over-trigger: a text/external subtitle needs no burn, so
	// a copyable source stays copy (the copy rung remains available).
	assert.Equal(t, "copy", d.VideoDecision, "external subtitle must not force transcode")
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
		{StreamType: "video", Codec: "hevc", Width: new(3840), Height: new(2160), Bitrate: int64Ptr(40000000), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: new(6), Bitrate: int64Ptr(1536000), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mkv", client, 8000)

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
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(6), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_channels_exceeded")
	assert.Equal(t, "aac", d.AudioCodec)
	assert.Equal(t, 2, d.MaxAudioChannels)
}

func TestAudioChannelsNotExceeded_NoCap(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264"},
		AudioCodecs:      []string{"aac"},
		Containers:       []string{"mp4"},
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 8,
	}
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	assert.Equal(t, 0, d.MaxAudioChannels, "MaxAudioChannels should stay zero when channels fit")
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
		{StreamType: "video", Codec: "hevc", Width: new(3840), Height: new(2160), HDRType: &hdrType, Bitrate: int64Ptr(20000000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Empty(t, d.TranscodeReasons)
}

// Target video: "Seize ans" (03451ca1) - HEVC + EAC3, MKV container

func hevcEac3Streams() []*library.MediaStream {
	return []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(4000000), IsDefault: true},
		{StreamType: "audio", Codec: "eac3", Channels: new(6), Bitrate: int64Ptr(640000), IsDefault: true},
	}
}

func TestHEVC_EAC3_Firefox_TranscodeBoth(t *testing.T) {
	// Firefox: no HEVC, no EAC3
	firefox := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264", "vp9"},
		AudioCodecs:      []string{"aac", "mp3", "opus", "flac"},
		Containers:       []string{"mp4", "webm"},
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 8,
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), hevcEac3Streams(), "mkv", firefox, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "video_codec_unsupported")
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.Equal(t, "h264", d.VideoCodec)
	assert.Equal(t, "aac", d.AudioCodec)
}

func TestHEVC_EAC3_Safari_CopyBoth(t *testing.T) {
	// Safari/Edge: supports HEVC + EAC3
	safari := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264", "hevc", "vp9"},
		AudioCodecs:      []string{"aac", "mp3", "opus", "flac", "ac3", "eac3"},
		Containers:       []string{"mp4"},
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 8,
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), hevcEac3Streams(), "mkv", safari, 0)

	// Container mismatch (mkv → mp4) but both codecs supported → direct stream (remux)
	assert.Equal(t, "direct_stream", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "container_incompatible")
	assert.Equal(t, "mp4", d.Container)
}

func TestHEVC_EAC3_DefaultWebProfile_TranscodeBoth(t *testing.T) {
	// DefaultWebProfile has no HEVC, no EAC3
	d := apptranscode.MakePlaybackDecision(t.Context(), hevcEac3Streams(), "mkv", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "video_codec_unsupported")
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
}

func TestHEVC_EAC3_PartialSupport_TranscodeAudioOnly(t *testing.T) {
	// Client supports HEVC but not EAC3
	client := transcoding.ClientProfile{
		VideoCodecs:      []string{"h264", "hevc"},
		AudioCodecs:      []string{"aac", "mp3", "opus"},
		Containers:       []string{"mp4"},
		MaxWidth:         3840,
		MaxHeight:        2160,
		MaxAudioChannels: 8,
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), hevcEac3Streams(), "mkv", client, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.NotContains(t, d.TranscodeReasons, "video_codec_unsupported")
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.Equal(t, "aac", d.AudioCodec)
}

// The audio decision must be made on the track the client actually PLAYS
// (prefs.AudioTrack, audio-relative), not blindly on the container default:
// default DTS + selected AAC must yield copy, and the reverse must transcode.
func TestAudioDecision_SelectedTrackWins(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true}, // default
		{StreamType: "audio", Codec: "aac", Channels: new(2)},                  // selected
	}

	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.AudioTrack = 1 // the AAC track
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision, "selected AAC track is client-decodable → copy")
	assert.NotContains(t, d.TranscodeReasons, "audio_codec_unsupported")

	prefs.AudioTrack = 0 // the DTS track
	d = apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision, "selected DTS track is not browser-decodable → transcode")
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")

	// No explicit selection (-1): historical default-flag behavior.
	prefs.AudioTrack = -1
	d = apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision, "default-flag track (DTS) judged when nothing is selected")
}

// A multichannel source that must transcode (DTS) targets its real layout -
// capped at 5.1 and by client/profile prefs - instead of the historical
// silent stereo downmix. Stereo sources keep MaxAudioChannels untouched so
// their cache hash (and bytes) are unchanged.
func TestAudioChannelTarget(t *testing.T) {
	mk := func(codec string, ch int) []*library.MediaStream {
		return []*library.MediaStream{
			{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: new(int64(5000000)), IsDefault: true},
			{StreamType: "audio", Codec: codec, Channels: new(ch), IsDefault: true},
		}
	}
	prefs := apptranscode.DefaultTranscodingPreferences()

	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), mk("dts", 6), "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Equal(t, 6, d.MaxAudioChannels, "DTS 5.1 → AAC 5.1, not stereo")

	d = apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), mk("truehd", 8), "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, 6, d.MaxAudioChannels, "7.1 folds to 5.1 (aacMaxTargetChannels)")

	d = apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), mk("dts", 2), "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, 0, d.MaxAudioChannels, "stereo source untouched (hash + bytes stability)")

	capped := prefs
	capped.MaxAudioChannels = 2
	d = apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), mk("dts", 6), "mp4", transcoding.DefaultWebProfile, 0, capped)
	assert.Equal(t, 2, d.MaxAudioChannels, "profile pref caps the target")

	stereoClient := transcoding.DefaultWebProfile
	stereoClient.MaxAudioChannels = 2
	d = apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), mk("dts", 6), "mp4", stereoClient, 0, prefs)
	assert.Equal(t, 2, d.MaxAudioChannels, "client cap honored")
}

// --- Commentary audio track selection ---

// TestSelectedAudioSpec_SkipsCommentaryDefault verifies that when the
// default-flagged audio track is a commentary track but a non-commentary audio
// track also exists, the non-commentary track is used for the audio decision
// (the commentary default is NOT selected). We observe this indirectly: the
// commentary track uses DTS (requires transcode), the non-commentary track uses
// AAC (copy-eligible). If the non-commentary track is selected, AudioDecision
// is "copy"; if the commentary default is selected, it would be "transcode".
func TestSelectedAudioSpec_SkipsCommentaryDefault(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		// Commentary track marked as default - should NOT be auto-selected when a
		// non-commentary alternative exists.
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true, Commentary: true},
		// Non-commentary track - should be preferred by auto-selection.
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: false, Commentary: false},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mkv", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision, "non-commentary AAC must be selected over commentary DTS default")
}

// TestSelectedAudioSpec_OnlyCommentaryKeptWhenNoAlternative verifies that when
// ALL audio tracks are commentary, the auto-selection still returns one (no
// silent drop).
func TestSelectedAudioSpec_OnlyCommentaryKeptWhenNoAlternative(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true, Commentary: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision, "sole commentary track must still be selected (no audio would be worse)")
}

// TestSelectedAudioSpec_ExplicitAudioTrackHonorsCommentary verifies that an
// explicit AudioTrack selection pointing at a commentary-flagged DTS track is
// honored (the commentary flag must NOT override an explicit user choice).
func TestSelectedAudioSpec_ExplicitAudioTrackHonorsCommentary(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		// Track 0 (audio-relative): commentary DTS - user explicitly selects this.
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true, Commentary: true},
		// Track 1: non-commentary AAC.
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: false},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.AudioTrack = 0 // explicitly select the commentary DTS track
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mkv", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision, "explicit selection of commentary DTS must still transcode")
}

// TestSelectedAudioSpec_NoCommentaryFlagUnchanged verifies that the commentary
// logic does not regress when no track has Commentary=true (the common case):
// the default-flagged track must still be selected.
func TestSelectedAudioSpec_NoCommentaryFlagUnchanged(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: false},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mkv", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision, "default DTS (no commentary flag) must still be selected and transcoded")
}

// The selection has a third and fourth branch that no test reached: what
// happens with NO default-flagged track at all. Four mutants survived in that
// tail, and it is a common shape - plenty of remuxes flag nothing as default.
func TestSelectedAudioSpec_NoDefaultFlagPrefersTheFirstNonCommentary(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		// Nothing is flagged default. The commentary track comes FIRST, so
		// picking "the first audio track" would play the commentary.
		{StreamType: "audio", Codec: "dts", Channels: new(6), Commentary: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2)},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mkv", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision,
		"with no default flag, the first NON-commentary track wins over the first track")
}

// And when nothing is flagged default AND every track is commentary, one must
// still be served - commentary beats silence.
func TestSelectedAudioSpec_NoDefaultAllCommentaryStillSelects(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), Commentary: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), Commentary: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "copy", d.AudioDecision, "an all-commentary set with no default must still yield audio")
}

// A default-flagged NON-commentary track is returned unchanged, even when a
// later non-commentary track exists - the default is the author's choice and
// the commentary rule must not reorder anything else.
func TestSelectedAudioSpec_NonCommentaryDefaultWinsOverLaterTracks(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		// Default, non-commentary, but needs a transcode.
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true},
		// A copy-eligible alternative that must NOT be preferred.
		{StreamType: "audio", Codec: "aac", Channels: new(2)},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mkv", transcoding.DefaultWebProfile, 0, prefs)
	assert.Equal(t, "transcode", d.AudioDecision,
		"a non-commentary default must be honoured even when a cheaper track exists")
}

// A Matroska PNG cover muxed as a plain video track (no attached_pic flag, so
// older scans stored it) must not turn a copyable H.264 film into a transcode:
// prod 2026-09-04, a 1920x804 H.264 film was re-encoded on VAAPI because its
// 200x258 "cover" track was judged an unsupported video codec.
func TestDirectPlay_CoverArtTrackIsNotVideo(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(804), Bitrate: int64Ptr(3300000), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
		{StreamType: "video", Codec: "png", Width: new(200), Height: new(258), Title: "cover"},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Empty(t, d.TranscodeReasons)
}

// Camera footage whose ONLY video stream is MJPEG is still judged as video:
// the still-image codec is only cover art when a real picture sits beside it.
func TestVideoTranscode_SoleMJPEGIsStillVideo(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "mjpeg", Width: new(1280), Height: new(720), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "video_codec_unsupported")
}

// Profile 7 (UHD Blu-ray) hides an HDR10 base layer under its enhancement
// layer: an HDR client gets that base re-encoded with the HDR kept, not SDR.
func TestHDR_DoViProfile7_HDRClient_KeepsHDR(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: new(3840), Height: new(2160), ColorSpace: new("bt2020nc"), ColorTransfer: new("smpte2084"), DoViProfile: new(7), Profile: "Main 10", Bitrate: new(int64(60000000)), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", hdrClient(), 0)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "dovi_base_layer")
	assert.NotContains(t, d.TranscodeReasons, "hdr_tonemapping_needed")

	sdr := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)
	assert.Contains(t, sdr.TranscodeReasons, "hdr_tonemapping_needed", "an SDR client still tone-maps")
}

// Hi10P anime and 4:2:2 captures advertise a codec the browser knows, in a
// pixel format it cannot decode.
func TestPixelFormat_10BitH264_Transcodes(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), BitDepth: new(10), PixelFormat: new("yuv420p10le"), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "pixel_format_unsupported")

	streams[0].BitDepth, streams[0].PixelFormat = new(8), new("yuv422p")
	assert.Contains(t, apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0).TranscodeReasons, "pixel_format_unsupported", "4:2:2")

	streams[0].BitDepth, streams[0].PixelFormat = new(8), new("yuv420p")
	assert.Equal(t, "direct_play", apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0).PlayMethod)

	hevc := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Width: new(1920), Height: new(1080), BitDepth: new(10), PixelFormat: new("yuv420p10le"), IsDefault: true},
	}
	hevcClient := transcoding.DefaultWebProfile
	hevcClient.VideoCodecs = []string{"h264", "hevc"}
	assert.NotContains(t, apptranscode.MakePlaybackDecision(t.Context(), hevc, "mp4", hevcClient, 0).TranscodeReasons, "pixel_format_unsupported", "10-bit HEVC is fine")
}

// A first-generation Chromecast decodes H.264 up to level 4.1: a level 5.1
// 4K H.264 file must transcode for it even though the codec matches.
// A device whose H.264 decoder stops below its general ceiling (a Chromecast
// Ultra: 2160p HEVC, 1080p H.264) declares MaxH264Height: a taller H.264
// source is re-encoded, HEVC and a fitting H.264 are not. The server judges
// this - the cast sender no longer keeps a rule set of its own.
func TestMaxH264Height_ClientCapForcesTranscodeOfH264Only(t *testing.T) {
	h264 := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	ultra := transcoding.DefaultWebProfile
	ultra.VideoCodecs = []string{"h264", "hevc"}
	ultra.MaxHeight, ultra.MaxH264Height = 2160, 1080
	d := apptranscode.MakePlaybackDecision(t.Context(), h264, "mp4", ultra, 0)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "resolution_exceeded")

	h264[0].Height = new(1080)
	assert.Equal(t, "direct_play", apptranscode.MakePlaybackDecision(t.Context(), h264, "mp4", ultra, 0).PlayMethod, "1080p H.264 fits the H.264 decoder")

	hevc := []*library.MediaStream{
		{StreamType: "video", Codec: "hevc", Profile: "Main", Width: new(3840), Height: new(2160), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	assert.Equal(t, "direct_play", apptranscode.MakePlaybackDecision(t.Context(), hevc, "mp4", ultra, 0).PlayMethod, "2160p HEVC reads MaxHeight, not the H.264 ceiling")
}

func TestLevel_ClientCapForcesTranscode(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), Level: new(51), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	capped := transcoding.DefaultWebProfile
	capped.MaxH264Level = 41
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", capped, 0)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "level_exceeded")

	streams[0].Level = new(40)
	assert.Equal(t, "direct_play", apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", capped, 0).PlayMethod, "level 4.0 fits")
	streams[0].Level = new(51)
	assert.Equal(t, "direct_play", apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0).PlayMethod, "no cap, no rule")
}

// A header level is what the encoder wrote, not what the stream needs: a
// 3840x2160 picture at 60 fps is 1 944 000 macroblocks a second, level 5.2,
// whatever the header says. Labelled 5.1, it went to a 5.1 decoder by direct
// play and failed there. The rate counts only when it is known: an unknown
// one must not re-encode every 4K film for a 5.1 device.
func TestLevel_FrameSizeAndRateOutrankALowHeader(t *testing.T) {
	fps := func(v float64) *float64 { return &v }
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(3840), Height: new(2160), Level: new(51), Framerate: fps(60), IsDefault: true},
		{StreamType: "audio", Codec: "aac", Channels: new(2), IsDefault: true},
	}
	capped := transcoding.DefaultWebProfile
	capped.MaxH264Level = 51
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", capped, 0)
	assert.Equal(t, "transcode", d.VideoDecision)
	assert.Contains(t, d.TranscodeReasons, "level_exceeded")

	streams[0].Framerate = fps(30)
	assert.Equal(t, "direct_play", apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", capped, 0).PlayMethod, "2160p30 fits level 5.1")
	streams[0].Framerate = nil
	assert.Equal(t, "direct_play", apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", capped, 0).PlayMethod, "unknown rate: the header decides")
	streams[0].Framerate = fps(60)
	capped.MaxH264Level = 52
	assert.Equal(t, "direct_play", apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", capped, 0).PlayMethod, "a 5.2 decoder plays it")
}
