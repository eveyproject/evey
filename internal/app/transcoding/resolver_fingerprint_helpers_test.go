package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func videoWithKeyframes(kf ...int64) []*library.MediaStream {
	return []*library.MediaStream{
		{StreamType: library.StreamTypeAudio},
		nil,
		{StreamType: library.StreamTypeVideo, Keyframes: kf},
	}
}

// The disk a windowed copy session needs is sized on the longest run of
// windowSegs keyframe spans, at twice the mean rate, never more than the
// source; an unknown window or index falls back to the whole source, an
// unknown size to nothing.
func TestCopyWorkingSetBytes(t *testing.T) {
	const source = 1_000_000
	uniform := videoWithKeyframes(0, 1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000)
	cases := []struct {
		name       string
		source     int64
		streams    []*library.MediaStream
		windowSegs int
		want       int64
	}{
		{"unknown source size", 0, uniform, 2, 0},
		{"negative source size", -5, uniform, 2, 0},
		{"no video stream", source, []*library.MediaStream{{StreamType: library.StreamTypeAudio}}, 2, source},
		{"no keyframe index", source, videoWithKeyframes(), 2, source},
		{"no window", source, uniform, 0, source},
		{"fewer keyframes than the window", source, videoWithKeyframes(0, 1000), 2, source},
		{"as many keyframes as the window", source, videoWithKeyframes(0, 1000), 2, source},
		// 10 s of film in 1 s segments, a 2-segment window: 2/10 of the bytes, doubled.
		{"even segments", source, uniform, 2, 400_000},
		// A 4 s segment among 1 s ones: the window that holds it spans 5 s of 13.333 s.
		{"one long segment sizes the window", source, videoWithKeyframes(0, 1000, 2000, 6000, 7000, 8000, 9000, 10000, 11000, 12000), 2, 750_018},
		// The need is capped at the source: 3 of 4 seconds, doubled, is more than the film.
		{"never more than the source", source, videoWithKeyframes(0, 1000, 2000, 3000), 3, source},
		{"keyframes that span no time", source, videoWithKeyframes(5, 5, 5), 1, source},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.InDelta(t, c.want, copyWorkingSetBytes(c.source, c.streams, c.windowSegs), 1)
		})
	}
}

// The AAC target the wasm master's key was built on: the lower of the
// client's and the profile's ceilings when the source is above it, the
// source layout up to 5.1 otherwise, and nothing to say about stereo.
func TestWasmMasterAudioTarget(t *testing.T) {
	cases := []struct {
		name                    string
		source, client, profile int
		wantChannels            int
	}{
		{"stereo source", 2, 0, 0, 0},
		{"mono source", 1, 2, 2, 0},
		{"5.1 source, no ceiling", 6, 0, 0, 6},
		{"7.1 source, no ceiling", 8, 0, 0, aacMaxTargetChannels},
		{"client ceiling below the source", 6, 2, 0, 2},
		{"profile ceiling below the source", 6, 0, 2, 2},
		{"client ceiling at the source", 6, 6, 0, 6},
		{"the lower ceiling is the profile's", 6, 6, 2, 2},
		{"the lower ceiling is the client's", 8, 2, 6, 2},
		{"7.1 under a 5.1 client", 8, 6, 0, 6},
		{"stereo under stereo ceilings", 2, 2, 2, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			codec, channels := wasmMasterAudioTarget(c.source,
				transcoding.ClientProfile{MaxAudioChannels: c.client},
				TranscodingPreferences{MaxAudioChannels: c.profile})
			assert.Equal(t, transcoding.DefaultAudioCodec, codec)
			assert.Equal(t, c.wantChannels, channels)
		})
	}
}
