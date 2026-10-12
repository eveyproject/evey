package transcoding_test

import (
	"testing"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
)

// The reported case: E-AC-3 5.1 in Matroska on a browser that decodes AC-3 in
// wasm. The server serves that track untouched, so calling it a transcode made
// the panel announce one over two copied tracks.
func wasmStreams(codec string, channels int) []*library.MediaStream {
	return []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), IsDefault: true},
		{StreamType: "audio", Codec: codec, Channels: new(channels), IsDefault: true},
	}
}

func wasmPrefs(on bool) apptranscode.TranscodingPreferences {
	p := apptranscode.DefaultTranscodingPreferences()
	p.WasmAudio = on
	return p
}

func TestWasmAudio_TrackIsNotTranscoded(t *testing.T) {
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), wasmStreams("eac3", 6), "matroska",
		transcoding.DefaultWebProfile, 0, wasmPrefs(true))

	assert.Equal(t, "wasm", d.AudioDecision)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.NotContains(t, d.TranscodeReasons, "audio_codec_unsupported")
	assert.NotContains(t, d.TranscodeReasons, "audio_channels_exceeded")
	// The container is still unreadable as-is, and that reason is real.
	assert.Contains(t, d.TranscodeReasons, "container_incompatible")
}

// PlayMethod routes: direct_stream hands web the /transcode?remux=1 pipe, while
// the wasm path needs the HLS master carrying the out-of-band rendition.
func TestWasmAudio_PlayMethodStaysTranscode(t *testing.T) {
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), wasmStreams("eac3", 6), "matroska",
		transcoding.DefaultWebProfile, 0, wasmPrefs(true))

	assert.Equal(t, "transcode", d.PlayMethod)
}

func TestWasmAudio_WithoutTheFlagStillTranscodes(t *testing.T) {
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), wasmStreams("eac3", 6), "matroska",
		transcoding.DefaultWebProfile, 0, wasmPrefs(false))

	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_codec_unsupported")
}

// 7.1 is above the ceiling the master handler enforces: it declines and serves
// the AAC rendition, so the decision must say transcode.
func TestWasmAudio_AboveChannelCeilingTranscodes(t *testing.T) {
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), wasmStreams("eac3", 8), "matroska",
		transcoding.DefaultWebProfile, 0, wasmPrefs(true))

	assert.Equal(t, "transcode", d.AudioDecision)
}

// An unprobed track (0 channels) is not treated as small.
func TestWasmAudio_UnknownChannelsTranscodes(t *testing.T) {
	streams := []*library.MediaStream{
		{StreamType: "video", Codec: "h264", IsDefault: true},
		{StreamType: "audio", Codec: "eac3", IsDefault: true},
	}
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "matroska",
		transcoding.DefaultWebProfile, 0, wasmPrefs(true))

	assert.Equal(t, "transcode", d.AudioDecision)
}

// The flag rides every request from a capable browser, including titles whose
// audio the wasm decoder has nothing to do with.
func TestWasmAudio_NonDolbyTrackUnaffected(t *testing.T) {
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), wasmStreams("aac", 2), "mp4",
		transcoding.DefaultWebProfile, 0, wasmPrefs(true))

	assert.Equal(t, "copy", d.AudioDecision)
	assert.Equal(t, "direct_play", d.PlayMethod)
}

// The decision, the master handler and the segment fingerprint must name the
// same sessions.
func TestWasmAudioApplies_MatchesTheMasterHandlerConditions(t *testing.T) {
	assert.True(t, apptranscode.WasmAudioApplies(wasmStreams("eac3", 6), 0, 0))
	assert.True(t, apptranscode.WasmAudioApplies(wasmStreams("ac3", 2), 0, 0))
	assert.False(t, apptranscode.WasmAudioApplies(wasmStreams("eac3", 8), 0, 0))
	assert.False(t, apptranscode.WasmAudioApplies(wasmStreams("aac", 6), 0, 0))
	assert.False(t, apptranscode.WasmAudioApplies(wasmStreams("eac3", 0), 0, 0))
}

// A viewer who capped audio at stereo must get a downmix, and this path cannot
// make one - it hands over the source track. It has to decline so the AAC
// rendition takes over.
func TestWasmAudio_ProfileChannelCapDeclinesTheWasmPath(t *testing.T) {
	assert.False(t, apptranscode.WasmAudioApplies(wasmStreams("eac3", 6), 0, 2))
	assert.True(t, apptranscode.WasmAudioApplies(wasmStreams("eac3", 2), 0, 2))
	// A cap at or above the source changes nothing.
	assert.True(t, apptranscode.WasmAudioApplies(wasmStreams("eac3", 6), 0, 6))

	prefs := wasmPrefs(true)
	prefs.MaxAudioChannels = 2
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), wasmStreams("eac3", 6), "matroska",
		transcoding.DefaultWebProfile, 0, prefs)

	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_channels_exceeded")
	assert.Equal(t, 2, d.MaxAudioChannels)
}
