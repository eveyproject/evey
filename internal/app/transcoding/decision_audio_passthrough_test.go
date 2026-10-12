package transcoding_test

// Regression lock for the 2026-09-06 prod report: "the background is not on the
// same plane as the voices", on an old DVD rip (XVID + MP3 128 kbps in AVI)
// played from the Android app.
//
// Two distinct defects met on that file:
//
//  1. The audio was re-encoded to AAC although nothing needed it. Re-encoding an
//     already-lossy 128k MP3 cascades: measured coding noise under the signal at
//     the 192k rendition was 30.6 dB on dialogue and 20.3 dB on dense music,
//     BELOW the 26.9 dB the 128k rendition scored on a clean source in
//     September. Jellyfin copies that track (`-codec:a:0 copy`) and adds none.
//  2. Nothing said so. The DECISION answered "copy", the packager refused the
//     passthrough afterwards, and the two never reconciled: the mobile stream
//     panel showed "Audio [copy] - MP3 - 128 kbps" over segments carrying
//     AAC-LC at 195 kb/s, and `hls: ffmpeg start` logged an empty audio_codec.
//
// The rules below are what keeps both closed.

import (
	"testing"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

// mp3Client is one of our ExoPlayer apps: it decodes mp3 AND accepts it muxed
// in fMP4. optOutClient is every web/iOS client, which declares the first and
// not the second.
func mp3PassthroughClient() transcoding.ClientProfile {
	c := transcoding.DefaultWebProfile
	c.FMP4AudioPassthrough = []string{"mp3"}
	return c
}

// The exact prod source: MPEG-4 ASP video (never direct-playable on the web
// profile, so the session is segmented) with an MP3 stereo track.
func xvidMP3Streams() []*library.MediaStream {
	return []*library.MediaStream{
		{StreamType: "video", Codec: "mpeg4", Width: new(640), Height: new(336), Bitrate: int64Ptr(786118), IsDefault: true},
		{StreamType: "audio", Codec: "mp3", Channels: new(2), Bitrate: int64Ptr(128880), IsDefault: true},
	}
}

// Defect 2. A segmented session whose client did NOT opt in must ANNOUNCE the
// re-encode, not claim a copy the packager will not honour.
func TestPackagedAudio_MP3WithoutOptInIsAnnouncedAsTranscode(t *testing.T) {
	d := apptranscode.MakePlaybackDecision(t.Context(), xvidMP3Streams(), "avi", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.PlayMethod)
	assert.Equal(t, "transcode", d.AudioDecision, "the packager re-encodes mp3; the decision must say so")
	assert.Equal(t, "aac", d.AudioCodec, "an empty AudioCodec is what made the panel show the SOURCE codec")
	assert.Contains(t, d.TranscodeReasons, "audio_passthrough_unsupported")
}

// Defect 1. The same source, for a client that accepts MP3 in fMP4: the track
// rides through untouched and no cascade happens.
func TestPackagedAudio_MP3PassesThroughForOptedInClient(t *testing.T) {
	d := apptranscode.MakePlaybackDecision(t.Context(), xvidMP3Streams(), "avi", mp3PassthroughClient(), 0)

	assert.Equal(t, "copy", d.AudioDecision, "an opted-in client must keep the source track")
	assert.NotContains(t, d.TranscodeReasons, "audio_passthrough_unsupported")
	// The video still transcodes (MPEG-4 ASP is not in the client's list): the
	// audio verdict is independent of it.
	assert.Equal(t, "transcode", d.VideoDecision)
}

// The guard that matters most: DIRECT PLAY hands over the raw file, no
// packaging, so no passthrough set applies. Demoting audio here would have
// killed direct play for every mp3-in-mp4 file in the library.
func TestPackagedAudio_DirectPlayIsNeverDemoted(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1280), Height: new(720), Bitrate: int64Ptr(3000000), IsDefault: true},
		{StreamType: "audio", Codec: "mp3", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "direct_play", d.PlayMethod)
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Empty(t, d.TranscodeReasons)
}

// A container-only mismatch is a "pure remux", but that remux still goes out
// through the fMP4 copy rung - the same packager. So an un-opted client loses
// direct_stream here, and gains an honest transcode verdict.
func TestPackagedAudio_DirectStreamDemotesWithoutOptIn(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1280), Height: new(720), Bitrate: int64Ptr(3000000), IsDefault: true},
		{StreamType: "audio", Codec: "mp3", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "matroska", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "copy", d.VideoDecision, "only the container was wrong")
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Equal(t, "transcode", d.PlayMethod, "audio no longer copies, so this is not a pure remux")
}

// Same remux for an opted-in client: both tracks copy, so it stays the cheap
// direct_stream it always was.
func TestPackagedAudio_DirectStreamSurvivesWithOptIn(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1280), Height: new(720), Bitrate: int64Ptr(3000000), IsDefault: true},
		{StreamType: "audio", Codec: "mp3", Channels: new(2), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "matroska", mp3PassthroughClient(), 0)

	assert.Equal(t, "copy", d.AudioDecision)
	assert.Equal(t, "direct_stream", d.PlayMethod)
}

// A codec the client cannot even decode was already forced to transcode by
// evaluateAudio; the new pass must not double-report the reason or overwrite
// the codec choice.
func TestPackagedAudio_UndecodableCodecKeepsItsOwnReason(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1280), Height: new(720), Bitrate: int64Ptr(3000000), IsDefault: true},
		{StreamType: "audio", Codec: "dts", Channels: new(6), IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0)

	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.NotContains(t, d.TranscodeReasons, "audio_passthrough_unsupported",
		"the codec check already refused it; the passthrough pass must stay quiet")
}

// The demo's Blender films: VP9/Opus in a .mkv, which the web plays as an
// HLS copy. An opted-in browser gets the Opus track verbatim (direct_stream
// survives); one without the opt-in is told the track is re-encoded.
func TestPackagedAudio_OpusFollowsTheOptIn(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "vp9", Width: new(1920), Height: new(858), Bitrate: int64Ptr(6000000), IsDefault: true},
		{StreamType: "audio", Codec: "opus", Channels: new(2), IsDefault: true},
	}
	web := transcoding.ClientProfile{VideoCodecs: []string{"h264", "vp9"}, AudioCodecs: []string{"aac", "opus"}, Containers: []string{"mp4", "webm"}}

	d := apptranscode.MakePlaybackDecision(t.Context(), streams, "matroska", web, 0, "/media/Sprite Fright (2021).mkv")
	assert.Equal(t, "transcode", d.AudioDecision, "no opt-in: the packager re-encodes and the decision says so")
	assert.Contains(t, d.TranscodeReasons, "audio_passthrough_unsupported")

	web.FMP4AudioPassthrough = []string{"opus"}
	d = apptranscode.MakePlaybackDecision(t.Context(), streams, "matroska", web, 0, "/media/Sprite Fright (2021).mkv")
	assert.Equal(t, "copy", d.AudioDecision)
	assert.Equal(t, "direct_stream", d.PlayMethod, "video and audio both copy: a pure remux")
	assert.NotContains(t, d.TranscodeReasons, "audio_passthrough_unsupported")
}
