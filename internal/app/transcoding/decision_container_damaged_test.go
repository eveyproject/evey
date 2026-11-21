package transcoding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// The television's own profile: it demuxes Matroska and decodes the codecs,
// so a sound file of this shape is handed over whole.
var nativeMatroskaClient = transcoding.ClientProfile{
	VideoCodecs: []string{"h264", "hevc", "vp9"},
	AudioCodecs: []string{"aac", "ac3", "eac3"},
	Containers:  []string{"mp4", "mkv", "webm", "hls"},
	MaxWidth:    1920, MaxHeight: 1080, MaxAudioChannels: 6,
}

func damagedSourceStreams() []*library.MediaStream {
	return []*library.MediaStream{
		{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(824), Bitrate: int64Ptr(3000000), IsDefault: true},
		{StreamType: "audio", Codec: "ac3", Channels: new(2), IsDefault: true},
	}
}

// A container a device's demuxer stops on (a few undecodable bytes ffmpeg and
// mkvgo step over) is never handed over whole: the tracks still copy, the
// server remuxes them - the same verdict as an incompatible container, never
// a re-encode.
func TestContainerDamaged_RemuxesInsteadOfHandingTheFileOver(t *testing.T) {
	streams := damagedSourceStreams()
	sound := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "matroska", nativeMatroskaClient, 0, apptranscode.DefaultTranscodingPreferences())
	assert.Equal(t, "direct_play", sound.PlayMethod, "a sound file of this shape is handed over whole")

	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SourceRefusesDirectPlay = true
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), streams, "matroska", nativeMatroskaClient, 0, prefs)

	assert.Equal(t, "direct_stream", d.PlayMethod, "remuxed, never the raw file")
	assert.Equal(t, []string{"container_damaged"}, d.TranscodeReasons)
	assert.Equal(t, "copy", d.VideoDecision)
	assert.Equal(t, "copy", d.AudioDecision)
}

// Next to an incompatible container (a browser), the reason changes nothing
// the verdict did not already say.
func TestContainerDamaged_WeighsLikeAnIncompatibleContainer(t *testing.T) {
	prefs := apptranscode.DefaultTranscodingPreferences()
	prefs.SourceRefusesDirectPlay = true
	client := nativeMatroskaClient
	client.Containers = []string{"mp4", "hls"}
	d := apptranscode.MakePlaybackDecisionWithPrefs(t.Context(), damagedSourceStreams(), "matroska", client, 0, prefs)

	assert.Equal(t, "direct_stream", d.PlayMethod)
	assert.ElementsMatch(t, []string{"container_incompatible", "container_damaged"}, d.TranscodeReasons)
	assert.Equal(t, "video_transcode", apptranscode.TranscodeCause(transcoding.PlaybackDecision{VideoDecision: "transcode", TranscodeReasons: d.TranscodeReasons}, nil),
		"neither container reason is named as the cause of a picture re-encode")
}
