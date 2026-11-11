package transcoding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// What the phone app declares: its own AC-3/E-AC-3 decoder, no DTS, no TrueHD.
var phoneProfile = transcoding.ClientProfile{
	VideoCodecs:          []string{"h264", "hevc", "vp9"},
	AudioCodecs:          []string{"aac", "mp3", "opus", "flac", "vorbis", "ac3", "eac3"},
	Containers:           []string{"mp4", "mkv", "webm", "hls"},
	MaxWidth:             3840,
	MaxHeight:            3840,
	SupportsHDR:          true,
	MaxAudioChannels:     6,
	MaxH264Level:         52,
	FMP4AudioPassthrough: []string{"mp3", "opus"},
}

// An English DTS track first, the French AC-3 flagged default and forced
// after the subtitles: the layout the phone crashed on.
func dtsBesideAC3() []*library.MediaStream {
	lvl := 41
	return []*library.MediaStream{
		{StreamIndex: 0, StreamType: "video", Codec: "h264", Width: new(1920), Height: new(800), Level: &lvl, IsDefault: true},
		{StreamIndex: 1, StreamType: "audio", Codec: "dts", Language: "eng", Channels: new(6), SampleRate: new(48000)},
		{StreamIndex: 2, StreamType: "subtitle", Codec: "subrip", Language: "eng"},
		{StreamIndex: 3, StreamType: "subtitle", Codec: "subrip", Language: "fre"},
		{StreamIndex: 4, StreamType: "audio", Codec: "ac3", Language: "fre", Channels: new(6), SampleRate: new(48000), IsDefault: true, IsForced: true},
		{StreamIndex: 5, StreamType: "subtitle", Codec: "subrip", Language: "fre", IsDefault: true, IsForced: true},
	}
}

func decideWithAudio(t *testing.T, streams []*library.MediaStream, client transcoding.ClientProfile, audioTrack int) *transcoding.PlaybackDecision {
	t.Helper()
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.AudioTrack = audioTrack
	return apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "matroska", client, 0, prefs)
}

func TestSiblingAudio_UndecodableTrackLeavesDirectPlay(t *testing.T) {
	for _, tc := range []struct {
		name       string
		audioTrack int
	}{
		{"default track", -1},
		{"French AC-3 picked by hand", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := decideWithAudio(t, dtsBesideAC3(), phoneProfile, tc.audioTrack)
			assert.NotEqual(t, "direct_play", d.PlayMethod, "the raw file would hand the player the DTS track")
			assert.NotEqual(t, "direct_stream", d.PlayMethod, "a whole-file remux carries the DTS track too")
			assert.Contains(t, d.TranscodeReasons, "sibling_audio_undecodable")
			assert.Equal(t, "copy", d.VideoDecision, "the picture is not what blocks the raw file")
		})
	}
}

func TestSiblingAudio_PickingTheUndecodableTrackReencodesIt(t *testing.T) {
	d := decideWithAudio(t, dtsBesideAC3(), phoneProfile, 0)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.NotContains(t, d.TranscodeReasons, "sibling_audio_undecodable", "the French track left beside it is decodable")
}

// The symmetric case: every track decodes, the raw file stays the cheapest
// way to play it.
func TestSiblingAudio_AllDecodableKeepsDirectPlay(t *testing.T) {
	streams := dtsBesideAC3()
	streams[1].Codec = "eac3"
	for _, audioTrack := range []int{-1, 0, 1} {
		d := decideWithAudio(t, streams, phoneProfile, audioTrack)
		assert.Equal(t, "direct_play", d.PlayMethod, "audioTrack=%d: %v", audioTrack, d.TranscodeReasons)
		assert.Empty(t, d.TranscodeReasons)
	}
}

// A client that decodes DTS gets the raw file: the reason follows the
// declaration, not the codec.
func TestSiblingAudio_ClientDecodingEveryTrackKeepsDirectPlay(t *testing.T) {
	client := phoneProfile
	client.AudioCodecs = append([]string{"dts"}, phoneProfile.AudioCodecs...)
	d := decideWithAudio(t, dtsBesideAC3(), client, -1)
	assert.Equal(t, "direct_play", d.PlayMethod, "%v", d.TranscodeReasons)
}

// Codec aliases fold before the comparison: a sibling probed "dca" is the
// same DTS a client declaring "dts" decodes.
func TestSiblingAudio_AliasesFold(t *testing.T) {
	streams := dtsBesideAC3()
	streams[1].Codec = "dca"
	client := phoneProfile
	client.AudioCodecs = append([]string{"dts"}, phoneProfile.AudioCodecs...)
	d := decideWithAudio(t, streams, client, -1)
	assert.Equal(t, "direct_play", d.PlayMethod, "%v", d.TranscodeReasons)

	d = decideWithAudio(t, streams, phoneProfile, -1)
	assert.Contains(t, d.TranscodeReasons, "sibling_audio_undecodable")
}

// Each undecodable codec a file can carry beside a decodable one.
func TestSiblingAudio_EveryUndecodableCodec(t *testing.T) {
	for _, codec := range []string{"dts", "truehd", "mlp", "pcm_s24le", "pcm_bluray", "mp2", "wmav2"} {
		streams := dtsBesideAC3()
		streams[1].Codec = codec
		d := decideWithAudio(t, streams, phoneProfile, -1)
		assert.Contains(t, d.TranscodeReasons, "sibling_audio_undecodable", codec)
		assert.NotEqual(t, "direct_play", d.PlayMethod, codec)
	}
}

// A single-track file has no sibling to judge.
func TestSiblingAudio_SingleTrackUnaffected(t *testing.T) {
	streams := dtsBesideAC3()
	streams = append(streams[:1], streams[2:]...)
	d := decideWithAudio(t, streams, phoneProfile, -1)
	assert.Equal(t, "direct_play", d.PlayMethod, "%v", d.TranscodeReasons)
}

// A client that declares PCM decodes every sample layout of it, and the
// DTS-HD spellings name the DTS a client declares.
func TestAudioCodecAliases_PCMAndDTSHD(t *testing.T) {
	for _, tc := range []struct{ source, declared string }{
		{"pcm_s16le", "pcm"},
		{"pcm_s24le", "pcm"},
		{"pcm_bluray", "pcm"},
		{"dca", "dts-hd"},
		{"dts", "dtshd"},
	} {
		streams := dtsBesideAC3()
		streams[4].Codec = tc.source
		client := phoneProfile
		client.AudioCodecs = append([]string{tc.declared}, phoneProfile.AudioCodecs...)
		d := decideWithAudio(t, streams, client, 1)
		assert.NotContains(t, d.TranscodeReasons, "audio_codec_unsupported", "%s declared as %s", tc.source, tc.declared)
	}
}

// A selection naming no track judges the default one, as the request entry
// points serve: a DTS default is not waved through as a copy.
func TestSelectedAudio_OutOfRangeJudgesTheDefault(t *testing.T) {
	streams := dtsBesideAC3()
	streams[1].IsDefault, streams[4].IsDefault = true, false
	d := decideWithAudio(t, streams, phoneProfile, 5)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
}

// A track whose channel count was never measured cannot be copied to a
// client with a ceiling; without a ceiling there is nothing to exceed.
func TestAudio_UnknownChannelsAgainstACeiling(t *testing.T) {
	streams := dtsBesideAC3()
	streams[4].Channels = nil
	d := decideWithAudio(t, streams[:5:5], phoneProfile, 1)
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_layout_unknown")
	assert.Equal(t, 6, d.MaxAudioChannels)

	unbounded := phoneProfile
	unbounded.MaxAudioChannels = 0
	d = decideWithAudio(t, streams, unbounded, 1)
	assert.NotContains(t, d.TranscodeReasons, "audio_layout_unknown")
}

// A file stored in one-track blocks is served segmented: an MP3 track the
// client takes raw from a file but not inside fMP4 segments is re-encoded,
// as for any other segmented session.
func TestNeedsRemux_AppliesTheSegmentedAudioGate(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		{StreamType: "audio", Codec: "mp3", Channels: new(2), SampleRate: new(48000), IsDefault: true},
	}
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SourceNeedsRemux = true
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "mp4", transcoding.DefaultWebProfile, 0, prefs)
	assert.Contains(t, d.TranscodeReasons, "file_needs_remux")
	assert.Equal(t, "transcode", d.AudioDecision, "%v", d.TranscodeReasons)
	assert.Contains(t, d.TranscodeReasons, "audio_passthrough_unsupported")
}
