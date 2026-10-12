package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// The exact master the production plan published on 2026-09-09 for the 4-track
// release. Three renditions for four source tracks: the TrueHD is not in this
// plan, so every rendition after it sits one position earlier than the
// arithmetic assumed.
const prodPlanMaster = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="[VFF] AC3 5.1",AUTOSELECT=YES,LANGUAGE="fre",DEFAULT=YES,URI="od_audio1.m3u8?v=v0.30.0"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="[VFQ] AC3 5.1",AUTOSELECT=YES,LANGUAGE="fre",URI="od_audio2.m3u8?v=v0.30.0"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="AC3 5.1",AUTOSELECT=YES,URI="od_audio3.m3u8?v=v0.30.0"
#EXT-X-STREAM-INF:BANDWIDTH=1000000,AUDIO="aud"
od_playlist.m3u8?v=v0.30.0
`

// The source, in file order. TrueHD first - which is what shifts everything.
func prodAudioStreams() []*domainlibrary.MediaStream {
	return []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "truehd", Language: "eng", Title: "TrueHD Atmos 7.1"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Language: "fre", Title: "[VFF] AC3 5.1"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Language: "fre", Title: "[VFQ] AC3 5.1"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Language: "eng", Title: "AC3 5.1"},
	}
}

func TestParsePlanAudioRenditions_ReadsNameLanguageAndURI(t *testing.T) {
	rends := parsePlanAudioRenditions(prodPlanMaster)
	require.Len(t, rends, 3, "only the audio renditions, and only the ones the plan kept")
	assert.Equal(t, "[VFF] AC3 5.1", rends[0].name)
	assert.Equal(t, "fre", rends[0].lang)
	assert.Equal(t, "od_audio1.m3u8?v=v0.30.0", rends[0].uri)
	assert.Equal(t, "AC3 5.1", rends[2].name)
	assert.Empty(t, rends[2].lang, "a track with no language declares none")
}

// The regression itself, in the shape that shipped: audioTrack=2 is the VFQ, and
// the derived name (audio3) is the ENGLISH track.
func TestPlanRenditionURIFor_MapsAroundATrackThePlanDropped(t *testing.T) {
	rends := parsePlanAudioRenditions(prodPlanMaster)
	streams := prodAudioStreams()

	// audioTrack 0 is the TrueHD: the plan does not carry it at all.
	_, ok := planRenditionURIFor(rends, streams, 0)
	assert.False(t, ok, "a track the plan dropped has no rendition to name")

	vff, ok := planRenditionURIFor(rends, streams, 1)
	require.True(t, ok)
	assert.Equal(t, "od_audio1.m3u8", vff, "the VFF is the plan's FIRST rendition, not its second")

	vfq, ok := planRenditionURIFor(rends, streams, 2)
	require.True(t, ok)
	assert.Equal(t, "od_audio2.m3u8", vfq, "the VFQ is audio2; audioTrack+1 named audio3, the English track")

	eng, ok := planRenditionURIFor(rends, streams, 3)
	require.True(t, ok)
	assert.Equal(t, "od_audio3.m3u8", eng)
}

// The ordinary file, where every track survives: the published mapping must
// agree with the arithmetic exactly, or this changes behaviour for everyone.
func TestPlanRenditionURIFor_AgreesWithTheArithmeticWhenNothingIsDropped(t *testing.T) {
	master := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="VF",LANGUAGE="fre",URI="od_audio1.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="VO",LANGUAGE="eng",URI="od_audio2.m3u8"
`
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Language: "fre", Title: "VF"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Language: "eng", Title: "VO"},
	}
	rends := parsePlanAudioRenditions(master)
	for track, want := range map[int]string{0: "od_audio1.m3u8", 1: "od_audio2.m3u8"} {
		got, ok := planRenditionURIFor(rends, streams, track)
		require.True(t, ok, "track %d", track)
		assert.Equal(t, want, got, "track %d: audioTrack+1 is right here and must stay right", track)
	}
}

// Untitled, language-less tracks carry no evidence at all. Positional matching
// is then all there is, and it must still resolve rather than strand the file.
func TestPlanRenditionURIFor_BareRenditionsMatchOnPosition(t *testing.T) {
	master := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",URI="od_audio1.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",URI="od_audio2.m3u8"
`
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3"},
	}
	got, ok := planRenditionURIFor(parsePlanAudioRenditions(master), streams, 1)
	require.True(t, ok)
	assert.Equal(t, "od_audio2.m3u8", got)
}

// No renditions at all (a plan that could not render its master): the caller
// keeps its arithmetic, so this must decline rather than invent a name.
func TestPlanRenditionURIFor_NoRenditionsDeclines(t *testing.T) {
	_, ok := planRenditionURIFor(nil, prodAudioStreams(), 1)
	assert.False(t, ok)
}

// End to end through the master builder: with the production plan's published
// renditions, picking the VFQ must name od_audio2. Before this, it named
// od_audio3 and the viewer heard English.
func TestBuildWasmAudioMaster_NamesThePublishedRenditionNotTheDerivedOne(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.onDemandTrackTrim = false
	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000}}

	ch := func(n int) *int { return &n }
	vw, vh, lvl := 3840, 2160, 153
	streams := prodAudioStreams()
	streams[0] = &domainlibrary.MediaStream{
		StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Profile: "Main 10",
		Width: &vw, Height: &vh, Level: &lvl, Keyframes: []int64{0, 6000, 12000},
	}
	for _, s := range streams[1:] {
		s.Channels = ch(6)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?wasmAudio=1&audioTrack=2&videoCodecs=h264&audioCodecs=aac&containers=mp4", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))

	master, err := h.buildWasmAudioMaster(req, "m1", item, streams, planFor(t, h, req, item, streams), false, true, 2, parsePlanAudioRenditions(prodPlanMaster))
	require.NoError(t, err)
	assert.Contains(t, master, `#EVEY-AUDIO-URI:"od_audio2.m3u8?audioTrack=2`, "the VFQ is the plan's second rendition")
	assert.NotContains(t, master, "od_audio3.m3u8", "od_audio3 is the ENGLISH track - naming it is the bug")
}
