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

func TestMapContainer_FullSDRHEVC(t *testing.T) {
	cicpBT709transfer := uint16(1)
	c := &extmkv.Container{
		DurationMs: 7_200_000,
		Tracks: []extmkv.Track{
			{
				Type: extmkv.VideoTrack, Codec: "hevc", Profile: "Main",
				Width: new(uint32(3840)), Height: new(uint32(1600)),
				VideoBitDepth: new(uint16(8)), ColorSpace: new(cicpBT709),
				ColorTransfer:   &cicpBT709transfer,
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
	res, ok := mapContainer(c, true)
	require.True(t, ok)
	require.Len(t, res.Streams, 3)
	assert.Equal(t, int64(7_200_000), res.Duration.Milliseconds())
	// Container MUST be reported - an empty ContainerFormat reads as
	// "container_incompatible" and forces a needless remux (Android seek/color
	// regression). EBML family -> "matroska" (decision maps it to "mkv"); the
	// same container over the ISO-BMFF route reports "mp4".
	assert.Equal(t, "matroska", res.Container)
	if mp4Res, mp4OK := mapContainer(c, false); assert.True(t, mp4OK) {
		assert.Equal(t, "mp4", mp4Res.Container)
	}

	v := res.Streams[0]
	assert.Equal(t, 0, v.StreamIndex)
	assert.Equal(t, library.StreamTypeVideo, v.StreamType)
	assert.Equal(t, "hevc", v.Codec)
	assert.Equal(t, "Main", v.Profile)
	assert.Equal(t, 3840, *v.Width)
	assert.Equal(t, 1600, *v.Height)
	assert.Equal(t, 8, *v.BitDepth)
	require.NotNil(t, v.ColorSpace)
	assert.Equal(t, "bt709", *v.ColorSpace) // feeds hdr.DetectHDR downstream
	require.NotNil(t, v.ColorTransfer)
	assert.Equal(t, "bt709", *v.ColorTransfer)
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

// HDR candidates (bt2020 colour or PQ/HLG transfer on an HDR-capable codec)
// must bail to ffprobe: mkvgo cannot read the Dolby Vision configuration, and
// a DoVi profile-5 title recorded as plain HDR10 would be raw-served with
// broken colours by the HDR-aware copy/direct-play paths.
func TestMapContainer_HDRDoViGating(t *testing.T) {
	cicpPQ := uint16(16) // smpte2084
	// Base track is HDR10 (bt2020 + PQ); each case mutates the DoVi config.
	track := func(mutate func(*extmkv.Track)) *extmkv.Container {
		tr := extmkv.Track{
			Type: extmkv.VideoTrack, Codec: "hevc", Profile: "Main 10",
			Width: new(uint32(3840)), Height: new(uint32(2160)),
			VideoBitDepth: new(uint16(10)), ColorSpace: new(cicpBT2020nc), ColorTransfer: &cicpPQ,
		}
		mutate(&tr)
		return &extmkv.Container{DurationMs: 1000, Tracks: []extmkv.Track{tr}}
	}

	// Plain HDR10 (no DoVi): mkvgo reads the colour, nothing to misread -> fast-path.
	res, ok := mapContainer(track(func(*extmkv.Track) {}), true)
	require.True(t, ok, "plain HDR10 (no DoVi) must fast-path")
	assert.Nil(t, res.Streams[0].DoViProfile)

	// DoVi P8.1 (HDR10-compatible base, BLCompat==1): safe -> fast-path, profile propagated.
	res, ok = mapContainer(track(func(tr *extmkv.Track) {
		tr.DolbyVision = &extmkv.DolbyVision{Profile: 8, BLSignalCompatID: 1}
	}), true)
	require.True(t, ok, "DoVi P8.1 (HDR10-compatible base) must fast-path")
	require.NotNil(t, res.Streams[0].DoViProfile)
	assert.Equal(t, 8, *res.Streams[0].DoViProfile)
	require.NotNil(t, res.Streams[0].DoViBLCompat)
	assert.Equal(t, 1, *res.Streams[0].DoViBLCompat)

	// DoVi P5 (IPT base): must fall back to ffprobe.
	_, ok = mapContainer(track(func(tr *extmkv.Track) {
		tr.DolbyVision = &extmkv.DolbyVision{Profile: 5, BLSignalCompatID: 0}
	}), true)
	assert.False(t, ok, "DoVi P5 must fall back to ffprobe")

	// DoVi P7/c6 (HDR10 base + RPU, single track): mkvgo reads it byte-identical to
	// ffprobe (validated on the real library) -> fast-path, profile propagated.
	res, ok = mapContainer(track(func(tr *extmkv.Track) {
		tr.DolbyVision = &extmkv.DolbyVision{Profile: 7, BLSignalCompatID: 6}
	}), true)
	require.True(t, ok, "DoVi P7/c6 must fast-path (metadata equals ffprobe)")
	require.NotNil(t, res.Streams[0].DoViProfile)
	assert.Equal(t, 7, *res.Streams[0].DoViProfile)

	// HDR-capable codec with no colour at all: bail (in-band colour mkvgo missed).
	_, ok = mapContainer(track(func(tr *extmkv.Track) { tr.ColorSpace = nil; tr.ColorTransfer = nil }), true)
	assert.False(t, ok, "HDR-capable codec with no colour must fall back")

	// SDR hevc (bt709, no HDR transfer): fast-path.
	_, ok = mapContainer(track(func(tr *extmkv.Track) { tr.ColorSpace = new(cicpBT709); tr.ColorTransfer = nil }), true)
	assert.True(t, ok, "SDR hevc keeps the fast path")
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
	res, ok := mapContainer(c, true)
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
	_, ok := mapContainer(c, true)
	assert.True(t, ok)
}

func TestMapContainer_LanguageAbsent(t *testing.T) {
	mk := func() *extmkv.Container {
		return &extmkv.Container{Tracks: []extmkv.Track{
			{Type: extmkv.VideoTrack, Codec: "h264", Width: new(uint32(1920)), Height: new(uint32(1080)), ColorSpace: new(cicpBT709)},
			// stale Language value with present=false: it must be ignored, and the
			// result must come from the container-family default, not "fra".
			{Type: extmkv.AudioTrack, Codec: "aac", Language: "fra", LanguagePresent: false},
		}}
	}
	// Matroska: no Language element -> spec default "eng" (ffmpeg/ffprobe apply it).
	res, ok := mapContainer(mk(), true)
	require.True(t, ok)
	assert.Equal(t, "eng", res.Streams[1].Language, "Matroska no-language element -> eng default")
	// MP4: no eng default (mdhd carries und when unset), absent stays empty.
	res, ok = mapContainer(mk(), false)
	require.True(t, ok)
	assert.Empty(t, res.Streams[1].Language, "MP4 no-language -> empty")
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
			_, ok := mapContainer(tc.c, true)
			assert.False(t, ok, "must fall back to ffprobe")
		})
	}
}

// TestMapContainer_ExtendedDispositions verifies that the 5 extended
// disposition flags (Commentary / HearingImpaired / VisualImpaired /
// TextDescriptions / Original) are propagated from mkvgo Track to MediaStream
// for both audio and subtitle stream types.
func TestMapContainer_ExtendedDispositions(t *testing.T) {
	c := &extmkv.Container{
		DurationMs: 3_600_000,
		Tracks: []extmkv.Track{
			// Video track - dispositions don't apply but must not block mapping.
			{
				Type: extmkv.VideoTrack, Codec: "h264",
				Width: new(uint32(1920)), Height: new(uint32(1080)),
				ColourDetermined: true,
				IsDefault:        true,
				LanguagePresent:  true, Language: "und",
			},
			// Audio: commentary track (director's commentary).
			{
				Type: extmkv.AudioTrack, Codec: "aac",
				Channels: new(uint8(2)), SampleRate: new(48000.0),
				Language: "eng", LanguagePresent: true,
				Commentary: true,
			},
			// Audio: hearing-impaired descriptive track.
			{
				Type: extmkv.AudioTrack, Codec: "aac",
				Channels: new(uint8(2)), SampleRate: new(48000.0),
				Language: "eng", LanguagePresent: true,
				HearingImpaired: true,
				IsDefault:       true,
			},
			// Subtitle: visually impaired + text descriptions + original.
			{
				Type: extmkv.SubtitleTrack, Codec: "srt",
				Language: "eng", LanguagePresent: true,
				VisualImpaired:   true,
				TextDescriptions: true,
				Original:         true,
			},
		},
	}

	res, ok := mapContainer(c, true)
	require.True(t, ok)
	require.Len(t, res.Streams, 4)

	audioCommentary := res.Streams[1]
	assert.True(t, audioCommentary.Commentary, "commentary flag must be set on audio track")
	assert.False(t, audioCommentary.HearingImpaired)
	assert.False(t, audioCommentary.VisualImpaired)
	assert.False(t, audioCommentary.TextDescriptions)
	assert.False(t, audioCommentary.Original)

	audioHI := res.Streams[2]
	assert.False(t, audioHI.Commentary)
	assert.True(t, audioHI.HearingImpaired, "hearing_impaired flag must be set")

	sub := res.Streams[3]
	assert.False(t, sub.Commentary)
	assert.True(t, sub.VisualImpaired, "visual_impaired flag must be set on subtitle")
	assert.True(t, sub.TextDescriptions, "text_descriptions flag must be set on subtitle")
	assert.True(t, sub.Original, "original flag must be set on subtitle")
}

// An HDR-capable codec with no colour tags bails ONLY when the colour was never
// read. When mkvgo parsed a colour source (container Colour / MP4 colr / SPS VUI)
// and it resolved to unspecified, ColourDetermined is true and the track is
// confirmed SDR -> it must fast-path, not fall back (validated 230/230 on real
// HEVC/AV1 SDR titles). When the colour was never read (ColourDetermined=false),
// real HDR could be hiding in the bitstream, so it still bails to ffprobe.
func TestMapContainer_ColourDetermined_SDRFastPath(t *testing.T) {
	mk := func(codec string, determined bool) *extmkv.Container {
		return &extmkv.Container{Tracks: []extmkv.Track{
			{Type: extmkv.VideoTrack, Codec: codec,
				Width: new(uint32(1920)), Height: new(uint32(1080)),
				VideoBitDepth: new(uint16(10)), ColourDetermined: determined},
		}}
	}
	for _, codec := range []string{"hevc", "av1"} {
		_, ok := mapContainer(mk(codec, true), true)
		assert.True(t, ok, codec+" no-colour but ColourDetermined must fast-path as SDR")
		_, ok = mapContainer(mk(codec, false), true)
		assert.False(t, ok, codec+" no-colour and not determined must bail to ffprobe")
	}
}

// The persisted field_order is the finer word mkvgo knows: the order when the
// container states it, the scan-type verdict otherwise (an interlaced H.264
// SPS yields "interlaced" and no order), nothing when neither is known - and
// whatever is persisted, the domain reads the same verdict mkvgo does.
func TestMapContainer_FieldOrderPersistsTheFinerScanWord(t *testing.T) {
	video := func(scan, order string) *extmkv.Container {
		return &extmkv.Container{Tracks: []extmkv.Track{{
			Type: extmkv.VideoTrack, Codec: "h264", Width: new(uint32(1920)), Height: new(uint32(1080)),
			ScanType: scan, FieldOrder: order,
		}}}
	}
	cases := []struct {
		scan, order string
		want        *string
	}{
		{"interlaced", "tt", new("tt")},
		{"interlaced", "", new("interlaced")},
		{"progressive", "progressive", new("progressive")},
		{"", "", nil},
	}
	for _, tc := range cases {
		res, ok := mapContainer(video(tc.scan, tc.order), true)
		require.True(t, ok)
		got := res.Streams[0].FieldOrder
		if tc.want == nil {
			assert.Nil(t, got, "%s/%s", tc.scan, tc.order)
			continue
		}
		if assert.NotNil(t, got, "%s/%s", tc.scan, tc.order) {
			assert.Equal(t, *tc.want, *got)
			assert.Equal(t, extmkv.ScanTypeOf(*got), string(library.ScanTypeOf(*got)), "the domain agrees with mkvgo on %q", *got)
		}
	}
}

// The domain's vocabulary table is pinned to mkvgo's: a word the two read
// differently would let a persisted value mean one thing to the prober and
// another to the resolver.
func TestScanTypeOf_AgreesWithMkvgo(t *testing.T) {
	for _, w := range []string{"progressive", "interlaced", "tt", "bb", "tb", "bt", "", "unknown"} {
		assert.Equal(t, extmkv.ScanTypeOf(w), string(library.ScanTypeOf(w)), "%q", w)
	}
}
