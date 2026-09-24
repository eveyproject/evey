package transcoding

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// single360p is a minimal single-profile used throughout the Master/Variant tests.
var single360p = []transcoding.HLSProfile{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: 800, AudioBitrate: 96},
}

// baseInput returns a valid ManifestInput for the given profiles and duration.
func baseInput(profiles []transcoding.HLSProfile, dur float64) ManifestInput {
	return ManifestInput{
		MediaID:             "media-abc",
		DurationSec:         dur,
		Profiles:            profiles,
		SubtitleBurnInTrack: -1, // disabled by default
	}
}

// --- Validate ---

func TestManifestGenerator_Validate_MediaIDEmpty(t *testing.T) {
	in := baseInput(single360p, 10.0)
	in.MediaID = ""
	err := in.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MediaID")
}

func TestManifestGenerator_Validate_DurationZero(t *testing.T) {
	in := baseInput(single360p, 0)
	err := in.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DurationSec must be > 0")
}

func TestManifestGenerator_Validate_DurationNegative(t *testing.T) {
	in := baseInput(single360p, -5.0)
	err := in.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DurationSec must be > 0")
}

func TestManifestGenerator_Validate_ProfilesEmpty(t *testing.T) {
	in := baseInput(nil, 10.0)
	err := in.Validate()
	require.Error(t, err)
}

func TestManifestGenerator_Validate_OK(t *testing.T) {
	in := baseInput(single360p, 10.0)
	require.NoError(t, in.Validate())
}

// --- SegmentCount ---

func TestManifestGenerator_SegmentCount(t *testing.T) {
	tests := []struct {
		name     string
		dur      float64
		expected int
	}{
		{"exact multiple", 9.0, 3},
		{"ceil", 10.0, 4},
		{"zero", 0, 0},
		{"negative", -1.0, 0},
		{"less than one segment", 2.5, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := ManifestInput{DurationSec: tc.dur}
			assert.Equal(t, tc.expected, in.SegmentCount())
		})
	}
}

// --- Master ---

func TestManifestGenerator_Master_SingleProfile(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	out, err := g.Master(in)
	require.NoError(t, err)

	lines := nonEmptyLines(out)
	// #EXTM3U, #EXT-X-VERSION:6, #EXT-X-INDEPENDENT-SEGMENTS, then 1×STREAM-INF + 1×URL = 5 lines total
	require.Len(t, lines, 5)
	assert.Equal(t, "#EXTM3U", lines[0])
	assert.Equal(t, "#EXT-X-VERSION:6", lines[1])
	assert.Equal(t, "#EXT-X-INDEPENDENT-SEGMENTS", lines[2])
	assert.Contains(t, lines[3], "#EXT-X-STREAM-INF:")
	assert.Contains(t, lines[4], "stream_0.m3u8")
}

func TestManifestGenerator_Master_FourProfiles(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(transcoding.DefaultHLSProfiles, 10.0)
	out, err := g.Master(in)
	require.NoError(t, err)

	lines := nonEmptyLines(out)
	// 3 header lines + 4×(STREAM-INF + URL) = 11 lines
	assert.Len(t, lines, 11)

	// Each profile must have its own stream_N.m3u8 URL
	for i := range 4 {
		url := fmt.Sprintf("stream_%d.m3u8", i)
		assert.True(t, containsLine(lines, url), "expected URL line for profile %d", i)
	}
}

func TestManifestGenerator_Master_Bandwidth(t *testing.T) {
	// 360p: VideoBitrate=800, AudioBitrate=96 → (800+96)*1000 = 896000
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "BANDWIDTH=896000")
}

// Every rung of the real ladder must be advertised at video+audio. Derived from
// the table rather than pinned to figures: those are a product decision that has
// already moved (audio floors raised on 2026-09-04), and a frozen copy here
// tests nothing but its own staleness. What is asserted is the RULE.
func TestManifestGenerator_Master_AllProfileBandwidths(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(transcoding.DefaultHLSProfiles, 10.0)
	out, err := g.Master(in)
	require.NoError(t, err)

	require.NotEmpty(t, transcoding.DefaultHLSProfiles)
	for _, p := range transcoding.DefaultHLSProfiles {
		t.Run(p.Name, func(t *testing.T) {
			assert.Contains(t, out, fmt.Sprintf("BANDWIDTH=%d", (p.VideoBitrate+p.AudioBitrate)*1000))
		})
	}
}

func TestManifestGenerator_Master_Resolution(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "RESOLUTION=640x360")
}

func TestManifestGenerator_Master_Codecs(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	out, err := g.Master(in)
	require.NoError(t, err)

	// CODECS value must be quoted in the attribute line
	assert.Contains(t, out, `CODECS="avc1.640028,mp4a.40.2"`)
}

func TestManifestGenerator_Master_AudioRendition_CodecAndSanitize(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.ChosenAudioCodec = "ac3" // the muxed decision says AC-3...
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, LanguageCode: "eng", Name: `Director's "cut"`},
	}
	out, err := g.Master(in)
	require.NoError(t, err)

	// The variant CODECS must advertise the rendition's real codec (the rendition
	// is always transcoded to AAC → mp4a.40.2), NOT the decision's ac3 - else
	// Safari refuses the master on an ec-3/ac-3 claim over an AAC stream.
	assert.Contains(t, out, "mp4a.40.2")
	assert.NotContains(t, out, "ac-3", "rendition CODECS must not claim ac-3 over an AAC rendition")

	// A NAME with a literal " must be STRIPPED (RFC 8216 forbids it in a
	// quoted-string), not backslash-escaped - an escaped quote breaks hls.js's
	// attribute parser and drops the rendition.
	assert.Contains(t, out, "#EXT-X-MEDIA:")
	assert.NotContains(t, out, `\"`, "NAME must not contain a backslash-escaped quote")
	assert.Contains(t, out, `NAME="Director's cut"`)
}

func TestManifestGenerator_Master_ExternalAudio_VideoOnly(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.ChosenVideoCodec = "h264"
	in.ChosenAudioCodec = "ac3" // source is AC-3, decoded client-side by wasm
	// ExternalAudio wins even if AudioRenditions were populated: no in-master
	// audio group must be emitted (MSE can't decode the ec-3 the wasm path serves
	// out of band).
	in.AudioRenditions = []AudioRendition{{SourceTrackIdx: 0, LanguageCode: "eng", Name: "English"}}
	in.ExternalAudio = true
	out, err := g.Master(in)
	require.NoError(t, err)

	// No audio group, no per-variant AUDIO= tie: the master is pure video.
	assert.NotContains(t, out, "#EXT-X-MEDIA:", "video-only master must emit no audio rendition group")
	assert.NotContains(t, out, "AUDIO=", "video-only variant must not reference an audio group")
	// CODECS lists the video codec ALONE - no mp4a/ac-3/ec-3 audio codec that the
	// video-only segments don't carry (Safari would refuse the master otherwise).
	assert.Contains(t, out, `CODECS="avc1.640028"`)
	assert.NotContains(t, out, "mp4a", "video-only CODECS must not advertise an audio codec")
	assert.NotContains(t, out, "ac-3")
	assert.NotContains(t, out, "ec-3")
	// The video variant is still there.
	assert.Contains(t, out, "stream_0.m3u8")
}

func TestManifestGenerator_Master_ExternalAudio_AudioURITag(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.ChosenVideoCodec = "h264"
	in.ExternalAudio = true
	in.ExternalAudioURI = "od_audio1.m3u8?token=abc"
	out, err := g.Master(in)
	require.NoError(t, err)

	// The custom tag names the out-of-band AC-3 rendition; hls.js ignores it.
	assert.Contains(t, out, `#EVEY-AUDIO-URI:"od_audio1.m3u8?token=abc"`)
	// No tag when ExternalAudio is off even if a URI was set by mistake.
	in2 := baseInput(single360p, 10.0)
	in2.ExternalAudioURI = "od_audio1.m3u8"
	out2, err := g.Master(in2)
	require.NoError(t, err)
	assert.NotContains(t, out2, "#EVEY-AUDIO-URI")
}

func TestManifestGenerator_Master_ExternalAudio_AacURITag(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.ChosenVideoCodec = "h264"
	in.ExternalAudio = true
	in.ExternalAudioURI = "od_audio1.m3u8?token=abc"
	in.ExternalAudioAACURI = "audio_0.m3u8?token=abc"
	out, err := g.Master(in)
	require.NoError(t, err)

	// The relay tag names the AAC rendition the client decodes NATIVELY while
	// the wasm decoder boots; hls.js ignores it like the AC-3 tag.
	assert.Contains(t, out, `#EVEY-AUDIO-AAC-URI:"audio_0.m3u8?token=abc"`)

	// No tag when ExternalAudio is off, or when no AAC URI was provided.
	in2 := baseInput(single360p, 10.0)
	in2.ExternalAudioAACURI = "audio_0.m3u8"
	out2, err := g.Master(in2)
	require.NoError(t, err)
	assert.NotContains(t, out2, "#EVEY-AUDIO-AAC-URI")
	in3 := baseInput(single360p, 10.0)
	in3.ExternalAudio = true
	in3.ExternalAudioURI = "od_audio1.m3u8"
	out3, err := g.Master(in3)
	require.NoError(t, err)
	assert.NotContains(t, out3, "#EVEY-AUDIO-AAC-URI")
}

func TestManifestGenerator_Master_WasmAudio_QueryParam(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.WasmAudio = true
	out, err := g.Master(in)
	require.NoError(t, err)
	// wasmAudio=1 rides every emitted URI so the segment fingerprint matches.
	assert.Contains(t, out, "wasmAudio=1")
}

func TestManifestGenerator_Master_ExternalAudio_CopyRungVideoOnlyCodecs(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput([]transcoding.HLSProfile{
		{Name: "src", Width: 1920, Height: 1080, VideoBitrate: 8000, AudioBitrate: 0, Copy: true},
	}, 10.0)
	in.ChosenVideoCodec = "h264"
	in.CopyVideoCodec = "avc1.640028"
	in.ExternalAudio = true
	out, err := g.Master(in)
	require.NoError(t, err)

	// A video-COPY rung under ExternalAudio advertises its exact source codec with
	// NO trailing audio codec appended.
	assert.Contains(t, out, `CODECS="avc1.640028"`)
	assert.NotContains(t, out, "mp4a")
}

func TestManifestGenerator_Master_TokenInQuery(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.StreamToken = "tok123"
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "token=tok123")
}

func TestManifestGenerator_Master_NoTokenWhenEmpty(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.StreamToken = ""
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.NotContains(t, out, "token=")
}

func TestManifestGenerator_Master_AudioTrackAlwaysPresent(t *testing.T) {
	// audioTrack=0 must appear even when StreamToken is empty and all optional fields unset.
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioTrack = 0
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "audioTrack=0")
}

func TestManifestGenerator_Master_SubtitleTrackPresent(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.SubtitleBurnInTrack = 2
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "subtitleTrack=2")
}

func TestManifestGenerator_Master_SubtitleTrackAbsentWhenMinus1(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.SubtitleBurnInTrack = -1
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.NotContains(t, out, "subtitleTrack")
}

func TestManifestGenerator_Master_VideoCodecsPresentWhenSet(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.VideoCodecs = []string{"h264", "hevc"}
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "videoCodecs=h264%2Chevc")
}

func TestManifestGenerator_Master_VideoCodecsAbsentWhenEmpty(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.VideoCodecs = nil
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.NotContains(t, out, "videoCodecs")
}

func TestManifestGenerator_Master_AudioCodecsPresentWhenSet(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioCodecs = []string{"aac"}
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "audioCodecs=aac")
}

func TestManifestGenerator_Master_AudioCodecsAbsentWhenEmpty(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioCodecs = nil
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.NotContains(t, out, "audioCodecs")
}

func TestManifestGenerator_Master_ContainersPresentWhenSet(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.Containers = []string{"mp4", "mkv"}
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "containers=mp4%2Cmkv")
}

func TestManifestGenerator_Master_ContainersAbsentWhenEmpty(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.Containers = nil
	out, err := g.Master(in)
	require.NoError(t, err)

	assert.NotContains(t, out, "containers")
}

// --- Variant ---

func TestManifestGenerator_Variant_ProfileIdxNegative(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	_, err := g.Variant(in, -1)
	require.Error(t, err)
}

func TestManifestGenerator_Variant_ProfileIdxOutOfRange(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	_, err := g.Variant(in, 1) // len == 1, so idx 1 is out of range
	require.Error(t, err)
}

func TestManifestGenerator_Variant_Header(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 9.0)
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	assert.Contains(t, out, "#EXTM3U\n")
	assert.Contains(t, out, "#EXT-X-VERSION:6\n")
	assert.Contains(t, out, "#EXT-X-PLAYLIST-TYPE:VOD\n")
	assert.Contains(t, out, "#EXT-X-TARGETDURATION:3\n")
	assert.Contains(t, out, "#EXT-X-MEDIA-SEQUENCE:0\n")
}

func TestManifestGenerator_Variant_Footer(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 9.0)
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(strings.TrimRight(out, "\n"), "#EXT-X-ENDLIST"),
		"playlist must end with #EXT-X-ENDLIST")
}

func TestManifestGenerator_Variant_DurationExact3Segments(t *testing.T) {
	// 9.0s / 3.0s = exactly 3 segments, all full
	g := NewManifestGenerator()
	in := baseInput(single360p, 9.0)
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	extinfLines := extractEXTINF(out)
	require.Len(t, extinfLines, 3, "expected 3 segments")
	for _, line := range extinfLines {
		assert.Equal(t, "#EXTINF:3.000,", line)
	}
}

func TestManifestGenerator_Variant_DurationCeil10(t *testing.T) {
	// 10.0s → ceil(10/3) = 4 segments; last = 10 - 3*3 = 1.0s
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	extinfLines := extractEXTINF(out)
	require.Len(t, extinfLines, 4, "expected 4 segments")
	assert.Equal(t, "#EXTINF:3.000,", extinfLines[0])
	assert.Equal(t, "#EXTINF:3.000,", extinfLines[1])
	assert.Equal(t, "#EXTINF:3.000,", extinfLines[2])
	assert.Equal(t, "#EXTINF:1.000,", extinfLines[3])
}

func TestManifestGenerator_Variant_DurationLessThanSegment(t *testing.T) {
	// 2.5s < 3.0s → 1 segment at 2.500
	g := NewManifestGenerator()
	in := baseInput(single360p, 2.5)
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	extinfLines := extractEXTINF(out)
	require.Len(t, extinfLines, 1, "expected 1 segment")
	assert.Equal(t, "#EXTINF:2.500,", extinfLines[0])
}

func TestManifestGenerator_Variant_ModeTS_Extension(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 9.0)
	in.UseFMP4 = false
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	assert.Contains(t, out, ".ts")
	assert.NotContains(t, out, ".m4s")
	assert.NotContains(t, out, "#EXT-X-MAP")
}

func TestManifestGenerator_Variant_ModeFMP4_Extension(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 9.0)
	in.UseFMP4 = true
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	assert.Contains(t, out, ".m4s")
	assert.NotContains(t, out, ".ts")
}

func TestManifestGenerator_Variant_ModeFMP4_InitMap(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(transcoding.DefaultHLSProfiles, 9.0)
	in.UseFMP4 = true
	out, err := g.Variant(in, 2) // profileIdx=2 → stream_2_init.mp4
	require.NoError(t, err)

	assert.Contains(t, out, `#EXT-X-MAP:URI="stream_2_init.mp4`)
}

func TestManifestGenerator_Variant_SegmentURLFormat_TS(t *testing.T) {
	// 9.0s → segments 000, 001, 002
	g := NewManifestGenerator()
	in := baseInput(single360p, 9.0)
	in.UseFMP4 = false
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	assert.Contains(t, out, "stream_0_000.ts")
	assert.Contains(t, out, "stream_0_001.ts")
	assert.Contains(t, out, "stream_0_002.ts")
}

func TestManifestGenerator_Variant_SegmentURLFormat_FMP4(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(transcoding.DefaultHLSProfiles, 9.0)
	in.UseFMP4 = true
	out, err := g.Variant(in, 1) // profileIdx=1
	require.NoError(t, err)

	assert.Contains(t, out, "stream_1_000.m4s")
	assert.Contains(t, out, "stream_1_001.m4s")
	assert.Contains(t, out, "stream_1_002.m4s")
}

func TestManifestGenerator_Variant_QuerySuffixOnAllSegments(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 9.0)
	in.StreamToken = "mytoken"
	in.UseFMP4 = false
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	// Every segment URL line must carry the token query param.
	for _, line := range nonEmptyLines(out) {
		if strings.HasPrefix(line, "stream_") {
			assert.Contains(t, line, "token=mytoken", "segment URL must carry token: %q", line)
		}
	}
}

func TestManifestGenerator_Variant_QuerySuffixOnInitMap(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 9.0)
	in.StreamToken = "tok"
	in.UseFMP4 = true
	out, err := g.Variant(in, 0)
	require.NoError(t, err)

	// The EXT-X-MAP URI must also carry the query suffix
	mapLine := ""
	for _, line := range nonEmptyLines(out) {
		if strings.HasPrefix(line, "#EXT-X-MAP:") {
			mapLine = line
			break
		}
	}
	require.NotEmpty(t, mapLine, "EXT-X-MAP line not found")
	assert.Contains(t, mapLine, "token=tok")
}

// --- helpers ---

// nonEmptyLines splits s into non-empty lines.
func nonEmptyLines(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// containsLine reports whether any element of lines equals target.
func containsLine(lines []string, target string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, target) {
			return true
		}
	}
	return false
}

// extractEXTINF returns all #EXTINF lines from a playlist.
func extractEXTINF(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(line, "#EXTINF:") {
			out = append(out, line)
		}
	}
	return out
}

// --- Audio rendition path ---

func TestManifestGenerator_Master_NoAudioRenditions_BackwardCompatible(t *testing.T) {
	// Bit-for-bit verification that the pre-feature behaviour stays
	// when AudioRenditions is empty: no EXT-X-MEDIA, no AUDIO= attr
	// on STREAM-INF. Any regression here breaks every existing client.
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	out, err := g.Master(in)
	require.NoError(t, err)
	assert.NotContains(t, out, "#EXT-X-MEDIA")
	assert.NotContains(t, out, "AUDIO=")
}

func TestManifestGenerator_Master_WithAudioRenditions_EmitsEXTXMEDIA(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "English", LanguageCode: "en"},
		{SourceTrackIdx: 1, Name: "Français", LanguageCode: "fr"},
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	lines := nonEmptyLines(out)

	// AudioTrack defaults to 0: track 0 = DEFAULT; every rendition is
	// AUTOSELECT=YES so a client can switch to any track itself.
	assert.True(t, containsLine(lines, `#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="English",DEFAULT=YES,AUTOSELECT=YES,LANGUAGE="en",URI="audio_0.m3u8`))
	assert.True(t, containsLine(lines, `#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="Français",DEFAULT=NO,AUTOSELECT=YES,LANGUAGE="fr",URI="audio_1.m3u8`))

	// Every STREAM-INF must now carry AUDIO="aud".
	for _, l := range lines {
		if strings.HasPrefix(l, "#EXT-X-STREAM-INF:") {
			assert.Contains(t, l, `AUDIO="aud"`, "STREAM-INF must link to the audio group: %s", l)
		}
	}
}

// The language-switch bug: the master always marked the first rendition
// DEFAULT, so ?audioTrack=1 changed nothing the player auto-selected and the
// audio stayed on track 0 (French) on both web and mobile. The requested
// track must be the DEFAULT one. Confirmed by diffing the live master.
func TestManifestGenerator_Master_AudioTrack_MarksRequestedRenditionDefault(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioTrack = 1
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "English", LanguageCode: "en"},
		{SourceTrackIdx: 1, Name: "Français", LanguageCode: "fr"},
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	lines := nonEmptyLines(out)

	// The requested track (1, Français) is DEFAULT=YES; track 0 drops to NO.
	assert.True(t, containsLine(lines, `#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="English",DEFAULT=NO,AUTOSELECT=YES,LANGUAGE="en",URI="audio_0.m3u8`))
	assert.True(t, containsLine(lines, `#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="Français",DEFAULT=YES,AUTOSELECT=YES,LANGUAGE="fr",URI="audio_1.m3u8`))
}

func TestManifestGenerator_Master_AudioRendition_LanguageOptional(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "Commentary"}, // no LanguageCode
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	// When no language, the LANGUAGE attr is omitted entirely (not
	// LANGUAGE="" which some players reject).
	assert.NotContains(t, out, `LANGUAGE=""`)
	assert.Contains(t, out, `NAME="Commentary",DEFAULT=YES,AUTOSELECT=YES,URI="audio_0.m3u8`)
}

func TestManifestGenerator_Master_AudioRendition_NameFallback(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 2, Name: ""}, // missing Name - fall back to "Track 3"
	}
	out, err := g.Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `NAME="Track 3"`)
}

func TestManifestGenerator_AudioPlaylist_OK(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "English", LanguageCode: "en"},
	}
	out, err := g.AudioPlaylist(in, 0)
	require.NoError(t, err)
	lines := nonEmptyLines(out)
	assert.Equal(t, "#EXTM3U", lines[0])
	assert.Contains(t, out, "#EXT-X-VERSION:6")
	assert.Contains(t, out, "#EXT-X-PLAYLIST-TYPE:VOD")
	assert.Contains(t, out, `#EXT-X-MAP:URI="audio_0_init.mp4`)
	assert.Contains(t, out, "audio_0_000.m4s")
	assert.Contains(t, out, "audio_0_003.m4s") // 10s / 3s = ceil(3.33) = 4 segments → 000..003
	assert.Contains(t, out, "#EXT-X-ENDLIST")
}

func TestManifestGenerator_AudioPlaylist_UnknownTrack(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "English"},
	}
	_, err := g.AudioPlaylist(in, 5) // not in renditions
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trackIdx 5 not in AudioRenditions")
}

func TestManifestGenerator_AudioPlaylist_NoRenditions(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 10.0)
	_, err := g.AudioPlaylist(in, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no audio renditions")
}

func TestManifestGenerator_AudioPlaylist_HonoursResumeOffset(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 100.0)
	in.AudioRenditions = []AudioRendition{
		{SourceTrackIdx: 0, Name: "English"},
	}
	in.ResumeOffsetSec = 42.5
	out, err := g.AudioPlaylist(in, 0)
	require.NoError(t, err)
	assert.Contains(t, out, "#EXT-X-START:TIME-OFFSET=42.500,PRECISE=YES")
}

// The pinned video decision must ride EVERY baked URL (variant + segment):
// it is what keeps a live session's plan stable across playlist polls when
// background enrichment flips copy-eligibility (the mid-play green-frames
// incident). Empty = legacy request, suffix unchanged.
func TestQuerySuffix_PinnedVideoDecision(t *testing.T) {
	in := ManifestInput{StreamToken: "tok", PinnedVideoDecision: "transcode"}
	assert.Contains(t, QuerySuffixFor(in), "vd=transcode")

	in.PinnedVideoDecision = ""
	assert.NotContains(t, QuerySuffixFor(in), "vd=")
}

// Same contract for the audio-side pin (ad=) and the user resync offset:
// both are decision inputs, so both must ride every baked URL.
func TestQuerySuffix_PinnedAudioDecisionAndOffset(t *testing.T) {
	in := ManifestInput{StreamToken: "tok", PinnedAudioDecision: "copy", AudioOffsetMs: 300}
	suffix := QuerySuffixFor(in)
	assert.Contains(t, suffix, "ad=copy")
	assert.Contains(t, suffix, "audioOffsetMs=300")

	in.PinnedAudioDecision = ""
	in.AudioOffsetMs = 0
	suffix = QuerySuffixFor(in)
	assert.NotContains(t, suffix, "ad=")
	assert.NotContains(t, suffix, "audioOffsetMs=")
}

// A fallback variant is another plan's rung riding this master: it must come
// AFTER the main variants (players that start on the first listed variant keep
// starting on the main one), carry its own muxed CODECS with NO rendition
// group, and its URI must be emitted verbatim - the master's own suffix baked
// onto it would point the rung at the WRONG plan (wrong fp, wrong vd pin).
func TestMaster_FallbackVariant_AppendedVerbatim(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput([]transcoding.HLSProfile{
		{Name: "od-copy", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 128, Copy: true},
	}, 60.0)
	in.StreamToken = "tok"
	in.VideoPlaylistURI = "od_playlist.m3u8"
	in.AudioRenditions = []AudioRendition{{SourceTrackIdx: 0, Name: "VF"}}
	in.FallbackVariants = []FallbackVariant{{
		Profile:    transcoding.HLSProfile{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1500, AudioBitrate: 128},
		URI:        "stream_0.m3u8?fp=other&vd=transcode",
		VideoCodec: "h264",
		AudioCodec: "aac",
	}}

	out, err := g.Master(in)
	require.NoError(t, err)

	assert.Contains(t, out, "stream_0.m3u8?fp=other&vd=transcode\n", "URI verbatim, no master suffix appended")
	assert.Less(t, strings.Index(out, "od_playlist.m3u8"), strings.Index(out, "stream_0.m3u8"),
		"the copy variant stays first: first-variant players must keep starting on it")

	var fbLine string
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "stream_0.m3u8") {
			fbLine = l
		}
	}
	require.NotEmpty(t, fbLine, "the fallback URI must be preceded by its own EXT-X-STREAM-INF")
	assert.Contains(t, fbLine, "BANDWIDTH=1628000")
	assert.Contains(t, fbLine, "RESOLUTION=854x480")
	assert.Contains(t, fbLine, `CODECS="avc1.640028,mp4a.40.2"`)
	assert.NotContains(t, fbLine, "AUDIO=", "a muxed fallback rung references no rendition group")
}

// A fallback rung whose plan serves audio as renditions carries video-only
// segments and plays its OWN plan's renditions, shifted onto its own video:
// the copy's group sits on the copy's timeline, 76 ms off a transcoded rung's
// (measured 2026-09-24). Same names, so the player keeps the viewer's track.
func TestMaster_FallbackVariant_VideoOnlyPlaysItsOwnAudioGroup(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput([]transcoding.HLSProfile{
		{Name: "od-copy", Width: 1920, Height: 1080, VideoBitrate: 5000, AudioBitrate: 128, Copy: true},
	}, 60.0)
	in.StreamToken = "tok"
	in.VideoPlaylistURI = "od_playlist.m3u8"
	in.AudioRenditions = []AudioRendition{{SourceTrackIdx: 0, Name: "VF", Codec: "aac"}}
	in.FallbackVariants = []FallbackVariant{{
		Profile:         transcoding.HLSProfile{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1500, AudioBitrate: 128},
		URI:             "stream_0.m3u8?fp=other&vd=transcode",
		VideoCodec:      "h264",
		AudioCodec:      "aac",
		AudioRenditions: []AudioRendition{{SourceTrackIdx: 0, Name: "VF", Codec: "aac"}},
		AudioSuffix:     "?fp=other&vd=transcode",
	}}

	out, err := g.Master(in)
	require.NoError(t, err)
	lines := strings.Split(out, "\n")
	var fbLine string
	for i, l := range lines {
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "stream_0.m3u8") {
			fbLine = l
		}
	}
	require.NotEmpty(t, fbLine)
	assert.Contains(t, fbLine, `AUDIO="aud-fb0"`)
	assert.Contains(t, fbLine, `CODECS="avc1.640028,mp4a.40.2"`)
	assert.Contains(t, out, `GROUP-ID="aud-fb0",NAME="VF",DEFAULT=YES,AUTOSELECT=YES,URI="audio_0.m3u8?fp=other&vd=transcode"`,
		"the rung's group points at its own plan's rendition")
	assert.Contains(t, out, `GROUP-ID="aud",NAME="VF"`, "the copy keeps its own group")
}

// No fallback variants (the default) must keep the master bit-for-bit what it
// was - the field is purely additive.
func TestMaster_NoFallbackVariants_Unchanged(t *testing.T) {
	g := NewManifestGenerator()
	in := baseInput(single360p, 60.0)
	withoutField, err := g.Master(in)
	require.NoError(t, err)
	in.FallbackVariants = nil
	withNilField, err := g.Master(in)
	require.NoError(t, err)
	assert.Equal(t, withoutField, withNilField)
	assert.NotContains(t, withoutField, "stream_1.m3u8")
}

// The channel ceiling is a decision input on the audio rendition exactly like
// hdr=1 is on the video rung: the EXT-X-MEDIA / init / segment URIs must carry
// it. Without it the rendition re-derives the SOURCE layout and hands a
// stereo-only sink the 5.1 the master just negotiated away.
func TestQuerySuffix_MaxAudioChannels(t *testing.T) {
	in := ManifestInput{StreamToken: "tok", MaxAudioChannels: 2}
	assert.Contains(t, QuerySuffixFor(in), "maxAudioChannels=2")

	in.MaxAudioChannels = 0
	assert.NotContains(t, QuerySuffixFor(in), "maxAudioChannels")
}

// The passthrough opt-in is a decision input like the codec lists: the segment
// handler re-parses the query to rebuild the plan, so a URI that drops it makes
// the segment re-derive an AAC re-encode under a master that already advertised
// mp4a.6B over the source MP3 (2026-09-06).
func TestQuerySuffix_FMP4AudioPassthrough(t *testing.T) {
	in := ManifestInput{StreamToken: "tok", FMP4AudioPassthrough: []string{"mp3"}}
	assert.Contains(t, QuerySuffixFor(in), "fmp4AudioPassthrough=mp3")

	in.FMP4AudioPassthrough = nil
	assert.NotContains(t, QuerySuffixFor(in), "fmp4AudioPassthrough",
		"a client that declared nothing must keep byte-identical URLs")
}

// maxH264Level shapes the ladder, so a segment request must carry it like
// screenHeight or the segment-time plan re-derives a taller ladder.
func TestQuerySuffix_BakesMaxH264Level(t *testing.T) {
	assert.Contains(t, QuerySuffixFor(ManifestInput{MaxH264Level: 41}), "maxH264Level=41")
	assert.NotContains(t, QuerySuffixFor(ManifestInput{}), "maxH264Level")
}

// Text subtitles ride the master as a "subs" rendition group pointing at the
// packager's windowed WebVTT, with the picker's ordinal on each line; every
// variant (copy and fallback) is tied to the group. Off = byte-identical.
func TestManifestGenerator_Master_SubtitleRenditions(t *testing.T) {
	in := baseInput(single360p, 10.0)
	in.StreamToken = "tok"
	in.SubtitleRenditions = []SubtitleRendition{
		{TrackIdx: 0, Name: "European (Forced)", LanguageCode: "fre", URI: "od_sub1.m3u8", Forced: true},
		{TrackIdx: 2, Name: "", LanguageCode: "", URI: "od_sub2.m3u8"},
	}
	in.FallbackVariants = []FallbackVariant{{Profile: transcoding.HLSProfile{Name: "480p", Width: 854, Height: 480, VideoBitrate: 1000, AudioBitrate: 128}, VideoCodec: "h264", AudioCodec: "aac", URI: "stream_0.m3u8?fp=x"}}
	out, err := NewManifestGenerator().Master(in)
	require.NoError(t, err)
	assert.Contains(t, out, `#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="European (Forced)",DEFAULT=NO,AUTOSELECT=NO,LANGUAGE="fre",FORCED=YES,X-EVEY-TRACK="0",URI="od_sub1.m3u8?audioTrack=0&token=tok"`)
	assert.Contains(t, out, `#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="Subtitle 3",DEFAULT=NO,AUTOSELECT=NO,X-EVEY-TRACK="2",URI="od_sub2.m3u8?audioTrack=0&token=tok"`)
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			assert.Contains(t, line, `,SUBTITLES="subs"`, line)
		}
	}

	in.SubtitleRenditions = nil
	plain, err := NewManifestGenerator().Master(in)
	require.NoError(t, err)
	assert.NotContains(t, plain, "SUBTITLES")
}
