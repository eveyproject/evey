package ffmpeg

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
)

func TestParseFrameRate_Valid(t *testing.T) {
	cases := []struct {
		input    string
		expected float64
	}{
		{"24000/1001", 24000.0 / 1001.0},
		{"30/1", 30.0},
		{"25/1", 25.0},
		{"60/1", 60.0},
		{"50/1", 50.0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseFrameRate(tc.input)
			assert.InDelta(t, tc.expected, got, 0.001)
		})
	}
}

func TestParseFrameRate_Invalid(t *testing.T) {
	cases := []struct {
		input string
	}{
		{"0/0"},
		{""},
		{"abc"},
		{"24/0"},
		{"abc/1"},
		{"24/abc"},
		{"nodivider"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseFrameRate(tc.input)
			assert.InDelta(t, 0.0, got, 0.001)
		})
	}
}

func TestCodecTypeToStreamType(t *testing.T) {
	cases := []struct {
		input    string
		expected library.StreamType
	}{
		{"video", library.StreamTypeVideo},
		{"audio", library.StreamTypeAudio},
		{"subtitle", library.StreamTypeSubtitle},
		{"attachment", library.StreamTypeAttachment},
		{"unknown", library.StreamTypeAttachment},
		{"data", library.StreamTypeAttachment},
		{"", library.StreamTypeAttachment},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := codecTypeToStreamType(tc.input)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestProbeStreams_InvalidBinary(t *testing.T) {
	p := NewProber("nonexistent_binary_xyz")
	_, err := p.ProbeStreams(context.Background(), "/tmp/fakefile.mkv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffprobe")
}

func TestProbeStreams_WithFakeScript(t *testing.T) {
	fakeJSON := `{"streams":[{"index":0,"codec_name":"h264","codec_type":"video","width":1920,"height":1080,"r_frame_rate":"24/1","disposition":{"default":1,"forced":0},"tags":{"language":"eng"}},{"index":1,"codec_name":"aac","codec_type":"audio","channels":6,"channel_layout":"5.1","sample_rate":"48000","disposition":{"default":1,"forced":0},"tags":{"language":"eng","title":"Surround"}}],"format":{"duration":"7200.5"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()

	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(7200500), result.Duration.Milliseconds())
	require.Len(t, result.Streams, 2)

	video := result.Streams[0]
	assert.Equal(t, 0, video.StreamIndex)
	assert.Equal(t, library.StreamTypeVideo, video.StreamType)
	assert.Equal(t, "h264", video.Codec)
	assert.Equal(t, "eng", video.Language)
	assert.True(t, video.IsDefault)
	assert.False(t, video.IsForced)
	require.NotNil(t, video.Width)
	assert.Equal(t, 1920, *video.Width)
	require.NotNil(t, video.Height)
	assert.Equal(t, 1080, *video.Height)
	require.NotNil(t, video.Framerate)
	assert.InDelta(t, 24.0, *video.Framerate, 0.001)

	audio := result.Streams[1]
	assert.Equal(t, 1, audio.StreamIndex)
	assert.Equal(t, library.StreamTypeAudio, audio.StreamType)
	assert.Equal(t, "aac", audio.Codec)
	assert.Equal(t, "eng", audio.Language)
	assert.Equal(t, "Surround", audio.Title)
	assert.True(t, audio.IsDefault)
	require.NotNil(t, audio.Channels)
	assert.Equal(t, 6, *audio.Channels)
	require.NotNil(t, audio.ChannelLayout)
	assert.Equal(t, "5.1", *audio.ChannelLayout)
	require.NotNil(t, audio.SampleRate)
	assert.Equal(t, 48000, *audio.SampleRate)
}

func TestProbeStreams_WithColorSpace(t *testing.T) {
	fakeJSON := `{"streams":[{"index":0,"codec_name":"hevc","codec_type":"video","width":3840,"height":2160,"r_frame_rate":"60/1","color_space":"bt2020nc","bits_per_raw_sample":"10","bit_rate":"20000000","disposition":{"default":1,"forced":0},"tags":{}}],"format":{"duration":"5400.0"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_hdr_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(5400000), result.Duration.Milliseconds())
	require.Len(t, result.Streams, 1)

	s := result.Streams[0]
	assert.Equal(t, "hevc", s.Codec)
	require.NotNil(t, s.ColorSpace)
	assert.Equal(t, "bt2020nc", *s.ColorSpace)
	require.NotNil(t, s.BitDepth)
	assert.Equal(t, 10, *s.BitDepth)
	require.NotNil(t, s.Bitrate)
	assert.Equal(t, int64(20000000), *s.Bitrate)
}

func TestProbeStreams_EmptyStreams(t *testing.T) {
	fakeJSON := `{"streams":[],"format":{"duration":"0"}}`

	script, err := os.CreateTemp("", "fake_ffprobe_empty_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho '" + fakeJSON + "'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	result, err := p.ProbeStreams(context.Background(), "/fake/empty.mkv")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int64(0), result.Duration.Milliseconds())
	assert.Empty(t, result.Streams)
}

func TestProbeStreams_InvalidJSON(t *testing.T) {
	script, err := os.CreateTemp("", "fake_ffprobe_bad_*.sh")
	require.NoError(t, err)
	defer os.Remove(script.Name())

	_, err = script.WriteString("#!/bin/sh\necho 'not valid json'\n")
	require.NoError(t, err)
	script.Close()
	require.NoError(t, os.Chmod(script.Name(), 0755))

	p := NewProber(script.Name())
	_, err = p.ProbeStreams(context.Background(), "/fake/movie.mkv")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffprobe parse")
}
