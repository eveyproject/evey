package transcoding

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

func i64(n int64) string { return strconv.FormatInt(n, 10) }

func dashInput(fps float64, durationSec float64) ManifestInput {
	return ManifestInput{
		MediaID:             "m1",
		DurationSec:         durationSec,
		Grid:                transcoding.GridForFrameRate(fps),
		StreamToken:         "tok",
		UseFMP4:             true,
		AudioTrack:          0,
		SubtitleBurnInTrack: -1,
		ChosenVideoCodec:    "h264",
		ChosenAudioCodec:    "aac",
		Profiles: []transcoding.HLSProfile{
			{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 128},
			{Name: "720p", Width: 1280, Height: 720, VideoBitrate: 2800, AudioBitrate: 192},
			{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 192},
		},
	}
}

var timelineRE = regexp.MustCompile(`(?s)<SegmentTimeline>(.*?)</SegmentTimeline>`)

// Every rung of the switch set declares the same timeline, exact in the
// grid's ticks: 399 full segments of 72 frames and a last one of 44 at
// 23.976 fps for a 20 min title - what mkvgo's switch-set validation
// demands of the fragments ffmpeg then cuts on that grid.
func TestMPD_ABRLadderSharesOneExactTimeline(t *testing.T) {
	t.Parallel()
	in := dashInput(23.976, 1200.042)
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)

	assert.Contains(t, mpd, `type="static"`)
	assert.Contains(t, mpd, `mediaPresentationDuration="PT1200.042S"`)
	assert.Contains(t, mpd, `frameRate="24000/1001"`)
	timelines := timelineRE.FindAllStringSubmatch(mpd, -1)
	require.Len(t, timelines, 3, "one timeline per rung")
	want := "\n            <S t=\"0\" d=\"72072\" r=\"398\"/>\n            <S d=\"44044\"/>\n          "
	for i, tl := range timelines {
		assert.Equal(t, want, tl[1], "rung %d", i)
	}
	assert.Equal(t, 3, strings.Count(mpd, `timescale="24000"`))
	for i, p := range in.Profiles {
		assert.Contains(t, mpd, `<Representation id="v`+itoa(i)+`" bandwidth="`+i64((p.VideoBitrate+p.AudioBitrate)*1000)+`" width="`+itoa(p.Width)+`" height="`+itoa(p.Height)+`" codecs="avc1.`)
		assert.Contains(t, mpd, `initialization="stream_`+itoa(i)+`_init.mp4?audioTrack=0&amp;token=tok"`)
		assert.Contains(t, mpd, `media="stream_`+itoa(i)+`_$Number%03d$.m4s?audioTrack=0&amp;token=tok"`)
	}
	assert.Contains(t, mpd, ",mp4a.40.2\"", "muxed audio rides the rung's codecs")
	assert.NotContains(t, mpd, `contentType="audio"`)
	assert.NotContains(t, mpd, "&token", "the query suffix must be XML-escaped")
}

// Exact spans at every canonical rate: 75 frames of 512 ticks at 25 fps, 90
// of 1001 at 29.97, and the tail holds the leftover frames.
func TestMPD_TimelinePerFrameRate(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		fps      float64
		timeline string
	}{
		{25, "\n            <S t=\"0\" d=\"38400\" r=\"398\"/>\n            <S d=\"38400\"/>\n          "},
		{29.97, "\n            <S t=\"0\" d=\"90090\" r=\"398\"/>\n            <S d=\"54054\"/>\n          "},
		{24, "\n            <S t=\"0\" d=\"36864\" r=\"398\"/>\n            <S d=\"36864\"/>\n          "},
	} {
		in := dashInput(c.fps, 1200)
		in.Profiles = in.Profiles[:1]
		mpd, err := NewManifestGenerator().MPD(in)
		require.NoError(t, err, "%v", c.fps)
		tl := timelineRE.FindStringSubmatch(mpd)
		require.NotNil(t, tl, "%v", c.fps)
		assert.Equal(t, c.timeline, tl[1], "%v fps", c.fps)
	}
	// 29.97 fps, 1200.5 s: 35979 frames, 399 full segments (35910) + 69 frames.
	in := dashInput(29.97, 1200.5)
	in.Profiles = in.Profiles[:1]
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Equal(t, "\n            <S t=\"0\" d=\"90090\" r=\"398\"/>\n            <S d=\"69069\"/>\n          ", timelineRE.FindStringSubmatch(mpd)[1])
}

// Separate audio renditions: video-only rungs (video codec and bits alone),
// one audio AdaptationSet per rendition addressed by the grid's nominal
// duration in the rendition's sample rate.
func TestMPD_SeparateAudioRenditions(t *testing.T) {
	t.Parallel()
	in := dashInput(23.976, 600)
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "English", LanguageCode: "eng", Codec: "aac"},
		{SourceTrackIdx: 2, Name: "Français", LanguageCode: "fra", Codec: "ac3"},
	}
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(mpd, `contentType="audio"`))
	assert.Contains(t, mpd, `<AdaptationSet mimeType="audio/mp4" contentType="audio" lang="eng">`)
	assert.Contains(t, mpd, `<Representation id="a0" bandwidth="128000" codecs="mp4a.40.2" audioSamplingRate="48000">`)
	assert.Contains(t, mpd, `<Representation id="a1" bandwidth="128000" codecs="ac-3">`, "a copied track keeps its codec, its rate is not declared")
	assert.Contains(t, mpd, `initialization="audio_0_init.mp4?audioTrack=0&amp;token=tok" media="audio_0_$Number%03d$.m4s?audioTrack=0&amp;token=tok" startNumber="0" timescale="48000" duration="144144"/>`)
	assert.Contains(t, mpd, `media="audio_2_$Number%03d$.m4s`)
	assert.Contains(t, mpd, `<Representation id="v0" bandwidth="800000" width="640" height="360" codecs="avc1.`, "video alone: no audio bits, no audio codec")
	assert.NotContains(t, mpd, `,mp4a.40.2"`)
}

// The ffmpeg copy fallback declares its keyframe plan: variable spans in
// milliseconds, runs of equal spans folded, the source's own codec string.
func TestMPD_CopyRungDeclaresTheKeyframePlan(t *testing.T) {
	t.Parallel()
	in := dashInput(0, 5.5) // no exact grid: the plan is the timeline
	in.Profiles = []transcoding.HLSProfile{{Name: "copy", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 192, Copy: true}}
	in.CopyVideoCodec = "avc1.640028"
	in.CopySegments = []CopySegment{{StartMs: 0, EndMs: 2000}, {StartMs: 2000, EndMs: 4000}, {StartMs: 4000, EndMs: 5500}}
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Contains(t, mpd, `codecs="avc1.640028,mp4a.40.2"`)
	assert.Contains(t, mpd, `timescale="1000"`)
	assert.Equal(t, "\n            <S t=\"0\" d=\"2000\" r=\"1\"/>\n            <S d=\"1500\"/>\n          ", timelineRE.FindStringSubmatch(mpd)[1])
	assert.NotContains(t, mpd, "frameRate=")

	in.CopySegments = nil
	_, err = NewManifestGenerator().MPD(in)
	require.ErrorIs(t, err, ErrDASHUnavailable, "a copy rung without its plan has no timeline to declare")
}

// The legacy clock grid (unknown or variable frame rate) declares nothing:
// its segments are not exact, so DASH is not offered - HLS still is.
func TestMPD_LegacyGridIsNotOffered(t *testing.T) {
	t.Parallel()
	in := dashInput(0, 600)
	_, err := NewManifestGenerator().MPD(in)
	require.ErrorIs(t, err, ErrDASHUnavailable)
	in.Grid = transcoding.GridForFrameRate(23.976)
	_, err = NewManifestGenerator().MPD(in)
	require.NoError(t, err)
}

// The HLS playlists declare the same grid the MPD does: 3.003 per segment at
// 23.976 fps, the leftover on the last, target duration 4.
func TestVariant_DeclaresTheGridSegments(t *testing.T) {
	t.Parallel()
	in := dashInput(23.976, 1200.042)
	pl, err := NewManifestGenerator().Variant(in, 0)
	require.NoError(t, err)
	assert.Contains(t, pl, "#EXT-X-TARGETDURATION:4\n")
	assert.Equal(t, 399, strings.Count(pl, "#EXTINF:3.003,\n"))
	assert.Contains(t, pl, "#EXTINF:1.835,\nstream_0_399.m4s")
	assert.Equal(t, 400, in.SegmentCount())
	// Legacy: byte-for-byte the former arithmetic.
	legacy := dashInput(0, 1200.042)
	pl, err = NewManifestGenerator().Variant(legacy, 0)
	require.NoError(t, err)
	assert.Contains(t, pl, "#EXT-X-TARGETDURATION:3\n")
	assert.Equal(t, 400, strings.Count(pl, "#EXTINF:3.000,\n"))
	assert.Contains(t, pl, "#EXTINF:0.042,\nstream_0_400.m4s")
	assert.Equal(t, 401, legacy.SegmentCount())
}
