package transcoding

import (
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
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
		// DASH carries audio as its own AdaptationSet only.
		AudioRenditions: []AudioRendition{{SourceTrackIdx: 0, Codec: "aac", SampleRate: 48000, Channels: 2, BitrateKbps: 192}},
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
		assert.Contains(t, mpd, `<Representation id="v`+itoa(i)+`" bandwidth="`+i64(p.VideoBitrate*1000)+`" width="`+itoa(p.Width)+`" height="`+itoa(p.Height)+`" codecs="avc1.`)
		assert.Contains(t, mpd, `initialization="stream_`+itoa(i)+`_init.mp4?audioTrack=0&amp;token=tok"`)
		assert.Contains(t, mpd, `media="stream_`+itoa(i)+`_$Number%03d$.m4s?audioTrack=0&amp;token=tok"`)
	}
	assert.NotContains(t, mpd, ",mp4a.40.2\"", "the rungs carry video alone")
	assert.Equal(t, 1, strings.Count(mpd, `contentType="audio"`), "the audio is its own AdaptationSet")
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
// one audio AdaptationSet per rendition carrying the rendition's own rate,
// layout and bits, addressed by the grid's nominal duration in its rate.
func TestMPD_SeparateAudioRenditions(t *testing.T) {
	t.Parallel()
	in := dashInput(23.976, 600)
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "English", LanguageCode: "eng", Codec: "aac", SampleRate: 48000, Channels: 6, BitrateKbps: 384},
		{SourceTrackIdx: 2, Name: "Français", LanguageCode: "fra", Codec: "ac3", SampleRate: 48000, Channels: 6, BitrateKbps: 640},
	}
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(mpd, `contentType="audio"`))
	assert.Contains(t, mpd, `<AdaptationSet mimeType="audio/mp4" contentType="audio" lang="eng">`)
	assert.Contains(t, mpd, `<Representation id="a0" bandwidth="384000" codecs="mp4a.40.2" audioSamplingRate="48000">`)
	assert.Contains(t, mpd, `<Representation id="a1" bandwidth="640000" codecs="ac-3" audioSamplingRate="48000">`, "a copied track keeps its codec and its bits")
	assert.Equal(t, 2, strings.Count(mpd, `<AudioChannelConfiguration schemeIdUri="urn:mpeg:dash:23003:3:audio_channel_configuration:2011" value="6"/>`))
	assert.Contains(t, mpd, `initialization="audio_0_init.mp4?audioTrack=0&amp;token=tok" media="audio_0_$Number%03d$.m4s?audioTrack=0&amp;token=tok" startNumber="0" timescale="48000" duration="144144"/>`)
	assert.Contains(t, mpd, `media="audio_2_$Number%03d$.m4s`)
	assert.Contains(t, mpd, `<Representation id="v0" bandwidth="800000" width="640" height="360" codecs="avc1.`, "video alone: no audio bits, no audio codec")
	assert.NotContains(t, mpd, `,mp4a.40.2"`)
}

// The requested track (?audioTrack=) is the main one, as DEFAULT=YES in the
// HLS master: without a Role the DASH player picked by its own locale and a
// language switch changed nothing but the URLs.
func TestMPD_RequestedAudioTrackIsMain(t *testing.T) {
	t.Parallel()
	roles := func(track int) []string {
		in := dashInput(25, 60)
		in.AudioTrack = track
		in.AudioRenditions = []AudioRendition{
			{SourceTrackIdx: 0, Name: "English", LanguageCode: "eng", Codec: "aac", SampleRate: 48000, Channels: 2},
			{SourceTrackIdx: 1, Name: "Fran\u00e7ais & co", LanguageCode: "fra", Codec: "aac", SampleRate: 48000, Channels: 2},
		}
		mpd, err := NewManifestGenerator().MPD(in)
		require.NoError(t, err)
		assert.Contains(t, mpd, "<Label>Fran\u00e7ais &amp; co</Label>")
		return regexp.MustCompile(`role:2011" value="(\w+)"`).FindAllString(mpd, -1)
	}
	assert.Equal(t, []string{`role:2011" value="main"`, `role:2011" value="alternate"`}, roles(0))
	assert.Equal(t, []string{`role:2011" value="alternate"`, `role:2011" value="main"`}, roles(1))
	assert.Equal(t, []string{`role:2011" value="main"`, `role:2011" value="alternate"`}, roles(7), "an unknown track falls back to the first, as in HLS")
}

// A copied AAC keeps its source rate: a 44.1 kHz track is declared 44.1 kHz,
// in its own timescale, never the 48 kHz of the re-encode.
func TestMPD_CopiedAACKeepsItsSampleRate(t *testing.T) {
	t.Parallel()
	in := dashInput(25, 60)
	in.AudioRenditions = []AudioRendition{{SourceTrackIdx: 0, Codec: "aac", SampleRate: 44100, Channels: 2, BitrateKbps: 160}}
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Contains(t, mpd, `<Representation id="a0" bandwidth="160000" codecs="mp4a.40.2" audioSamplingRate="44100">`)
	assert.Contains(t, mpd, `value="2"/>`)
	assert.Contains(t, mpd, `timescale="44100" duration="132300"/>`, "3 s in the track's own ticks")
	assert.NotContains(t, mpd, `48000`)
}

// A rendition whose shape is unknown declares no rate and no layout rather
// than a guessed one; its bits fall back to the rung's audio budget.
func TestMPD_UnknownAudioShapeIsNotGuessed(t *testing.T) {
	t.Parallel()
	in := dashInput(25, 60)
	in.AudioRenditions = []AudioRendition{{SourceTrackIdx: 0, Codec: "ac3"}}
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Contains(t, mpd, `<Representation id="a0" bandwidth="128000" codecs="ac-3">`)
	assert.NotContains(t, mpd, `audioSamplingRate`)
	assert.NotContains(t, mpd, `AudioChannelConfiguration`)
}

// A plan fitted to one rung of its ladder names that rung's fragments, as the
// HLS master's stream_<rung>.m3u8 does: rung 0's bytes under rung 1's shape
// would be a stranger's picture.
func TestMPD_FittedRungKeepsItsLadderName(t *testing.T) {
	t.Parallel()
	in := dashInput(25, 60)
	in.Profiles = in.Profiles[1:2]
	in.FirstRungIndex = 1
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Contains(t, mpd, `<Representation id="v1" bandwidth="2800000" width="1280" height="720"`)
	assert.Contains(t, mpd, `initialization="stream_1_init.mp4?`)
	assert.Contains(t, mpd, `media="stream_1_$Number%03d$.m4s?`)
	assert.NotContains(t, mpd, `stream_0_`)
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
	assert.Contains(t, mpd, `codecs="avc1.640028"`)
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

// Audio muxed into the video fragments, or decoded by the client (wasm), is
// not offered as DASH: dash.js drops a two-codec Representation ("No streams
// to play", measured on dash.js 5.2) and an MPD has no place for the
// client's own decoder. HLS serves both.
func TestMPD_MuxedOrClientAudioIsNotOffered(t *testing.T) {
	t.Parallel()
	muxed := dashInput(25, 60)
	muxed.AudioRenditions = nil
	_, err := NewManifestGenerator().MPD(muxed)
	require.ErrorIs(t, err, ErrDASHUnavailable)

	wasm := dashInput(25, 60)
	wasm.ExternalAudio = true
	_, err = NewManifestGenerator().MPD(wasm)
	require.ErrorIs(t, err, ErrDASHUnavailable)
}

// A copied AAC is announced by its own object type - HE-AAC mp4a.40.5,
// HE-AACv2 mp4a.40.29 - in the MPD and in the HLS master alike; an unknown
// or absent profile keeps mp4a.40.2, nothing guessed.
func TestAudioRendition_CodecFollowsTheCopiedProfile(t *testing.T) {
	t.Parallel()
	for profile, want := range map[string]string{
		"LC": "mp4a.40.2", "HE-AAC": "mp4a.40.5", "HE-AACv2": "mp4a.40.29", "": "mp4a.40.2", "Weird": "mp4a.40.2",
	} {
		in := dashInput(25, 60)
		in.AudioRenditions = []AudioRendition{{SourceTrackIdx: 0, Codec: "aac", SampleRate: 48000, Channels: 2, Profile: profile}}
		mpd, err := NewManifestGenerator().MPD(in)
		require.NoError(t, err)
		assert.Contains(t, mpd, `<Representation id="a0" bandwidth="128000" codecs="`+want+`"`, "profile %q", profile)
		master, err := NewManifestGenerator().Master(in)
		require.NoError(t, err)
		assert.Contains(t, master, `,`+want+`"`, "profile %q: the HLS CODECS", profile)
	}
	in := dashInput(25, 60)
	in.AudioRenditions = []AudioRendition{{SourceTrackIdx: 0, Codec: "ac3", Profile: "HE-AAC"}}
	mpd, err := NewManifestGenerator().MPD(in)
	require.NoError(t, err)
	assert.Contains(t, mpd, `codecs="ac-3"`, "an AAC profile never rewrites another codec")
}

// The hybrid manifest keeps the packager's video and subtitle sets byte for
// byte (their timeline is the packager's) and replaces its audio - the
// source's tracks - with Evey's renditions, as the hybrid HLS master does.
func TestHybridMPD_PackagerVideoBesideEveyAudio(t *testing.T) {
	t.Parallel()
	video := `    <AdaptationSet mimeType="video/mp4" contentType="video">
      <Representation id="v" bandwidth="1954245" width="1280" height="720" codecs="avc1.4D401F">
        <SegmentTemplate initialization="od_init.mp4?v=1" media="od_seg$Number%05d$.m4s?v=1" startNumber="1" timescale="1000">
          <SegmentTimeline><S t="0" d="6000" r="119"/></SegmentTimeline>
        </SegmentTemplate>
      </Representation>
    </AdaptationSet>
`
	text := `    <AdaptationSet mimeType="text/vtt" contentType="text" lang="fre">
      <Representation id="s1" bandwidth="256"><BaseURL>od_sub1.vtt?v=1</BaseURL></Representation>
    </AdaptationSet>
`
	packager := `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT720.000S">
  <Period>
` + video + `    <AdaptationSet mimeType="audio/mp4" contentType="audio" lang="eng">
      <Label>English</Label>
      <Representation id="a1" bandwidth="0" codecs="ac-3"><SegmentTemplate media="od_seg_a1_$Number%05d$.m4s?v=1"/></Representation>
    </AdaptationSet>
` + text + `  </Period>
</MPD>
`
	in := dashInput(25, 720)
	in.AudioTrack = 1
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "English", LanguageCode: "eng", Codec: "aac", SampleRate: 48000, Channels: 2},
		{SourceTrackIdx: 1, Name: "Francais", LanguageCode: "fra", Codec: "aac", SampleRate: 48000, Channels: 2},
	}
	mpd, err := NewManifestGenerator().HybridMPD(in, packager)
	require.NoError(t, err)
	var doc struct {
		Periods []struct {
			Sets []struct {
				ContentType string `xml:"contentType,attr"`
			} `xml:"AdaptationSet"`
		} `xml:"Period"`
	}
	require.NoError(t, xml.Unmarshal([]byte(mpd), &doc), mpd)
	require.Len(t, doc.Periods, 1)
	var kinds []string
	for _, s := range doc.Periods[0].Sets {
		kinds = append(kinds, s.ContentType)
	}
	assert.Equal(t, []string{"video", "text", "audio", "audio"}, kinds)
	assert.Contains(t, mpd, video, "the packager's video set, verbatim")
	assert.Contains(t, mpd, text, "the packager's subtitles, verbatim")
	assert.NotContains(t, mpd, "od_seg_a1_", "the packager's audio (the source's AC-3) is gone")
	assert.Contains(t, mpd, `media="audio_1_$Number%03d$.m4s?audioTrack=1&amp;token=tok"`)
	assert.Equal(t, []string{`role:2011" value="alternate"`, `role:2011" value="main"`},
		regexp.MustCompile(`role:2011" value="(\w+)"`).FindAllString(mpd, -1), "the requested track is the main one")
	assert.Contains(t, mpd, "    </AdaptationSet>\n  </Period>\n</MPD>\n")

	_, err = NewManifestGenerator().HybridMPD(in, `<MPD><Period></Period></MPD>`)
	require.Error(t, err, "no video set: nothing to stand beside")
	in.AudioRenditions = nil
	_, err = NewManifestGenerator().HybridMPD(in, packager)
	assert.ErrorIs(t, err, ErrDASHUnavailable, "no separate audio: nothing to add")
}
