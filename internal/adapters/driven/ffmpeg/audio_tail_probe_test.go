package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Matroska muxers (mkvmerge, ffmpeg) stamp each track with a DURATION tag in
// HH:MM:SS.nnnnnnnnn form. Parsing it is the head-only fast path of the tail
// probe.
func TestParseTrackDurationTag(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"00:00:18.000000000", 18.0, true},
		{"01:35:42.123000000", 5742.123, true},
		{"00:30:00.000000000", 1800.0, true},
		{"1:2:3.5", 3723.5, true},
		{"", 0, false},
		{"N/A", 0, false},
		{"12.5", 0, false}, // bare seconds is not the tag form
		{"aa:bb:cc", 0, false},
	}
	for _, c := range cases {
		got, ok := parseTrackDurationTag(c.in)
		assert.Equal(t, c.ok, ok, "input %q", c.in)
		if c.ok {
			assert.InDelta(t, c.want, got, 1e-6, "input %q", c.in)
		}
	}
}

// The head probe alone settles the gap when every audio track carries a
// DURATION tag: gap = declared container duration - the EARLIEST audio end.
func TestAudioTailGapFromHead_AllTagged(t *testing.T) {
	head := []byte(`{
	  "streams": [
	    {"index": 0, "codec_type": "video", "tags": {"DURATION": "00:01:39.851000000"}},
	    {"index": 1, "codec_type": "audio", "tags": {"DURATION": "00:01:35.700000000"}},
	    {"index": 2, "codec_type": "audio", "tags": {"DURATION": "00:01:39.851000000"}}
	  ],
	  "format": {"duration": "99.851000"}
	}`)
	dur, audio, err := parseAudioTailHead(head)
	require.NoError(t, err)
	assert.InDelta(t, 99.851, dur, 1e-6)
	require.Len(t, audio, 2)

	gap, allTagged := audioTailGapFromTags(dur, audio)
	assert.True(t, allTagged)
	assert.InDelta(t, 99.851-95.700, gap, 1e-6, "the worst (shortest) track defines the gap")
}

// A lowercase tag key (some muxers) must parse the same.
func TestAudioTailGapFromHead_LowercaseTag(t *testing.T) {
	head := []byte(`{
	  "streams": [
	    {"index": 1, "codec_type": "audio", "tags": {"duration": "00:00:18.000000000"}}
	  ],
	  "format": {"duration": "30.000000"}
	}`)
	dur, audio, err := parseAudioTailHead(head)
	require.NoError(t, err)
	gap, allTagged := audioTailGapFromTags(dur, audio)
	assert.True(t, allTagged)
	assert.InDelta(t, 12.0, gap, 1e-6)
}

// An untagged audio stream means the head cannot settle the question - the
// caller must fall back to reading the tail packets.
func TestAudioTailGapFromHead_UntaggedStream(t *testing.T) {
	head := []byte(`{
	  "streams": [
	    {"index": 1, "codec_type": "audio", "tags": {"DURATION": "00:00:18.000000000"}},
	    {"index": 2, "codec_type": "audio", "tags": {"language": "eng"}}
	  ],
	  "format": {"duration": "30.000000"}
	}`)
	dur, audio, err := parseAudioTailHead(head)
	require.NoError(t, err)
	_, allTagged := audioTailGapFromTags(dur, audio)
	assert.False(t, allTagged, "an untagged track cannot be assumed to reach the end")
}

// No audio streams at all → nothing to measure (a video-only file is not the
// shortfall defect).
func TestAudioTailGapFromHead_NoAudio(t *testing.T) {
	head := []byte(`{
	  "streams": [{"index": 0, "codec_type": "video"}],
	  "format": {"duration": "30.000000"}
	}`)
	dur, audio, err := parseAudioTailHead(head)
	require.NoError(t, err)
	assert.Empty(t, audio)
	gap, allTagged := audioTailGapFromTags(dur, audio)
	assert.True(t, allTagged)
	assert.Zero(t, gap)
}

// The tail fallback reads packets from near the end and keeps the LAST
// timestamp per stream; pts wins, dts fills in when pts is absent.
func TestParseAudioTailPackets(t *testing.T) {
	out := []byte(`{
	  "packets": [
	    {"stream_index": 1, "pts_time": "17.947000"},
	    {"stream_index": 2, "pts_time": "29.100000"},
	    {"stream_index": 1, "pts_time": "17.979000"},
	    {"stream_index": 2, "dts_time": "29.958000"},
	    {"stream_index": 3, "codec_type": "audio"}
	  ]
	}`)
	last, err := parseAudioTailPackets(out)
	require.NoError(t, err)
	assert.InDelta(t, 17.979, last[1], 1e-6)
	assert.InDelta(t, 29.958, last[2], 1e-6, "dts fills in when pts is absent")
	_, ok := last[3]
	assert.False(t, ok, "a packet with no timestamp settles nothing")
}
