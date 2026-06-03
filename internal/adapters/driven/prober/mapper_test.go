package prober

import (
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
)

const (
	cicpBT2020nc = uint16(9)
	cicpBT709    = uint16(1)
)

func TestMapContainer_FullHDR(t *testing.T) {
	c := &extmkv.Container{
		DurationMs: 7_200_000,
		Tracks: []extmkv.Track{
			{
				Type: extmkv.VideoTrack, Codec: "hevc", Profile: "Main 10",
				Width: new(uint32(3840)), Height: new(uint32(1600)),
				VideoBitDepth: new(uint16(10)), ColorSpace: new(cicpBT2020nc),
				FrameRate:       new(23.976),
				IsDefault:       true,
				LanguagePresent: true, Language: "und",
			},
			{
				Type: extmkv.AudioTrack, Codec: "eac3",
				Channels: new(uint8(6)), SampleRate: new(48000.0),
				Language: "fre", LanguagePresent: true, IsDefault: true,
			},
			{
				Type: extmkv.SubtitleTrack, Codec: "srt",
				Language: "fre", LanguagePresent: true, Name: "Forced", IsForced: true,
			},
		},
	}
	res, ok := mapContainer(c)
	require.True(t, ok)
	require.Len(t, res.Streams, 3)
	assert.Equal(t, int64(7_200_000), res.Duration.Milliseconds())

	v := res.Streams[0]
	assert.Equal(t, 0, v.StreamIndex)
	assert.Equal(t, library.StreamTypeVideo, v.StreamType)
	assert.Equal(t, "hevc", v.Codec)
	assert.Equal(t, "Main 10", v.Profile)
	assert.Equal(t, 3840, *v.Width)
	assert.Equal(t, 1600, *v.Height)
	assert.Equal(t, 10, *v.BitDepth)
	require.NotNil(t, v.ColorSpace)
	assert.Equal(t, "bt2020nc", *v.ColorSpace) // feeds hdr.DetectHDR downstream
	require.NotNil(t, v.Framerate)
	assert.InDelta(t, 23.976, *v.Framerate, 0.001)
	assert.True(t, v.IsDefault)
	assert.Equal(t, "und", v.Language)

	a := res.Streams[1]
	assert.Equal(t, library.StreamTypeAudio, a.StreamType)
	assert.Equal(t, "eac3", a.Codec)
	assert.Equal(t, 6, *a.Channels)
	assert.Equal(t, 48000, *a.SampleRate)
	assert.Equal(t, "fre", a.Language)

	s := res.Streams[2]
	assert.Equal(t, library.StreamTypeSubtitle, s.StreamType)
	assert.Equal(t, "subrip", s.Codec) // srt -> subrip via FFprobeCodecName
	assert.Equal(t, "Forced", s.Title)
	assert.True(t, s.IsForced)
	assert.Equal(t, "fre", s.Language)
}

func TestMapContainer_H264_SDR_NoColourOK(t *testing.T) {
	// h264 with bt709 colour -> mapped, ok (h264 SDR is fine even without HDR).
	c := &extmkv.Container{
		DurationMs: 5_000_000,
		Tracks: []extmkv.Track{
			{Type: extmkv.VideoTrack, Codec: "h264", Profile: "High",
				Width: new(uint32(1920)), Height: new(uint32(1080)), ColorSpace: new(cicpBT709)},
			{Type: extmkv.AudioTrack, Codec: "aac", Channels: new(uint8(2))},
		},
	}
	res, ok := mapContainer(c)
	require.True(t, ok)
	assert.Equal(t, "bt709", *res.Streams[0].ColorSpace)
	assert.Equal(t, "High", res.Streams[0].Profile)
}

func TestMapContainer_H264_NoColour_StillOK(t *testing.T) {
	// h264 with NO colour read at all -> still OK (h264 HDR is nonexistent; SDR
	// legitimately has no colour, matching ffprobe leaving it empty).
	c := &extmkv.Container{Tracks: []extmkv.Track{
		{Type: extmkv.VideoTrack, Codec: "h264", Width: new(uint32(1920)), Height: new(uint32(1080))},
	}}
	_, ok := mapContainer(c)
	assert.True(t, ok)
}

func TestMapContainer_LanguageAbsent(t *testing.T) {
	c := &extmkv.Container{Tracks: []extmkv.Track{
		{Type: extmkv.VideoTrack, Codec: "h264", Width: new(uint32(1920)), Height: new(uint32(1080)), ColorSpace: new(cicpBT709)},
		{Type: extmkv.AudioTrack, Codec: "aac", Language: "eng", LanguagePresent: false}, // present=false -> ""
	}}
	res, ok := mapContainer(c)
	require.True(t, ok)
	assert.Empty(t, res.Streams[1].Language, "no language element -> empty (mirror ffprobe)")
}

func TestMapContainer_FallbackGates(t *testing.T) {
	cases := []struct {
		name string
		c    *extmkv.Container
	}{
		{"nil", nil},
		{"zero tracks", &extmkv.Container{}},
		{"unknown track type", &extmkv.Container{Tracks: []extmkv.Track{
			{Type: extmkv.TrackType("button"), Codec: "x"}}}},
		{"unmapped/exotic codec", &extmkv.Container{Tracks: []extmkv.Track{
			{Type: extmkv.VideoTrack, Codec: "V_REAL/RV40", Width: new(uint32(640)), Height: new(uint32(480))}}}},
		{"video missing height", &extmkv.Container{Tracks: []extmkv.Track{
			{Type: extmkv.VideoTrack, Codec: "h264", Width: new(uint32(1920))}}}},
		{"hevc no colour (possible-HDR -> ffprobe)", &extmkv.Container{Tracks: []extmkv.Track{
			{Type: extmkv.VideoTrack, Codec: "hevc", Width: new(uint32(3840)), Height: new(uint32(2160))}}}},
		{"av1 no colour", &extmkv.Container{Tracks: []extmkv.Track{
			{Type: extmkv.VideoTrack, Codec: "av1", Width: new(uint32(3840)), Height: new(uint32(2160))}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := mapContainer(tc.c)
			assert.False(t, ok, "must fall back to ffprobe")
		})
	}
}
