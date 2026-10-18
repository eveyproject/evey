package transcoding_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// A copied track keeps its sample rate, and players stop at 48 kHz: a 96 kHz
// AAC copied as is came out at 96000 Hz. The track is re-encoded instead, which
// the HLS encoder already does at 48 kHz.
func TestAudioSampleRateAbove48kHz_Transcodes(t *testing.T) {
	client := transcoding.ClientProfile{
		VideoCodecs: []string{"h264"},
		AudioCodecs: []string{"aac"},
		Containers:  []string{"mp4"},
		MaxWidth:    3840,
		MaxHeight:   2160,
	}
	decide := func(rate *int) *transcoding.PlaybackDecision {
		streams := []*library.MediaStream{
			{StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080), Bitrate: int64Ptr(5000000), IsDefault: true},
			{StreamType: "audio", Codec: "aac", Channels: new(2), SampleRate: rate, IsDefault: true},
		}
		return apptranscode.MakePlaybackDecision(t.Context(), streams, "mp4", client, 0)
	}

	d := decide(new(96000))
	assert.Equal(t, "transcode", d.AudioDecision)
	assert.Contains(t, d.TranscodeReasons, "audio_samplerate_exceeded")
	assert.Equal(t, transcoding.DefaultAudioCodec, d.AudioCodec)
	assert.Equal(t, "copy", d.VideoDecision)

	for _, rate := range []*int{new(48000), new(44100), nil} {
		d := decide(rate)
		assert.Equal(t, "copy", d.AudioDecision, "rate %v", rate)
		assert.NotContains(t, d.TranscodeReasons, "audio_samplerate_exceeded")
	}
}
