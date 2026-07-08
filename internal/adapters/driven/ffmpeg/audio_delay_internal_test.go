package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAudioStartDelay(t *testing.T) {
	// Shaped like `ffprobe -show_entries stream=codec_type,start_time:
	// stream_disposition=attached_pic -of json` on a delayed-audio repack with
	// a cover-art stream first and two audio tracks.
	out := []byte(`{"streams":[
		{"codec_type":"video","start_time":"0.000000","disposition":{"attached_pic":1}},
		{"codec_type":"video","start_time":"0.005000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"0.900000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"N/A","disposition":{"attached_pic":0}},
		{"codec_type":"subtitle","disposition":{"attached_pic":0}}
	]}`)

	d, err := parseAudioStartDelay(out, 0)
	require.NoError(t, err)
	assert.InDelta(t, 0.895, d, 1e-9, "delay = audio start - REAL video start (cover art skipped)")

	// Track 1 has no usable start_time: no evidence of a delay -> 0, nil.
	d, err = parseAudioStartDelay(out, 1)
	require.NoError(t, err)
	assert.Zero(t, d)

	// Track index beyond the stream list: same "nothing to cancel" outcome.
	d, err = parseAudioStartDelay(out, 5)
	require.NoError(t, err)
	assert.Zero(t, d)

	// Malformed JSON is a real error (the caller logs and fails soft).
	_, err = parseAudioStartDelay([]byte("not json"), 0)
	assert.Error(t, err)
}

func TestParseMaxAudioStartDelay(t *testing.T) {
	out := []byte(`{"streams":[
		{"codec_type":"video","start_time":"0.000000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"0.300000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"0.900000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"N/A","disposition":{"attached_pic":0}}
	]}`)
	d, err := parseMaxAudioStartDelay(out)
	require.NoError(t, err)
	assert.InDelta(t, 0.9, d, 1e-9, "max across audio tracks")

	// Audio EARLIER than video must not go negative (0 = nothing to flag).
	early := []byte(`{"streams":[
		{"codec_type":"video","start_time":"1.000000","disposition":{"attached_pic":0}},
		{"codec_type":"audio","start_time":"0.000000","disposition":{"attached_pic":0}}
	]}`)
	d, err = parseMaxAudioStartDelay(early)
	require.NoError(t, err)
	assert.Zero(t, d)

	// No video (music file): nothing to measure against.
	audioOnly := []byte(`{"streams":[
		{"codec_type":"audio","start_time":"0.500000","disposition":{"attached_pic":0}}
	]}`)
	d, err = parseMaxAudioStartDelay(audioOnly)
	require.NoError(t, err)
	assert.Zero(t, d)
}
