package httpapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// The packager's master for an episode with a forced and a full French SRT
// (prod, "Le sang par le sang").
const prodPlanMasterWithSubs = `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="European",AUTOSELECT=YES,LANGUAGE="fre",DEFAULT=YES,URI="od_audio1.m3u8?v=v0.30.0"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="European (Forced)",AUTOSELECT=YES,LANGUAGE="fre",DEFAULT=YES,FORCED=YES,URI="od_sub1.m3u8?v=v0.30.0"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="European",AUTOSELECT=YES,LANGUAGE="fre",URI="od_sub2.m3u8?v=v0.30.0"
#EXT-X-STREAM-INF:BANDWIDTH=12986906,RESOLUTION=1920x960,CODECS="avc1.640028,ec-3",AUDIO="aud",SUBTITLES="subs"
od_playlist.m3u8?v=v0.30.0
`

func TestParsePlanSubtitleRenditions_ReadsNameLanguageURIAndForced(t *testing.T) {
	rends := parsePlanSubtitleRenditions(prodPlanMasterWithSubs)
	require.Len(t, rends, 2)
	assert.Equal(t, planSubtitleRendition{name: "European (Forced)", lang: "fre", uri: "od_sub1.m3u8?v=v0.30.0", forced: true}, rends[0])
	assert.Equal(t, planSubtitleRendition{name: "European", lang: "fre", uri: "od_sub2.m3u8?v=v0.30.0", forced: false}, rends[1])
}

// The pickers count every subtitle stream (image and external ones too); the
// packager only publishes the container's text tracks. A PGS track between
// two SRTs shifts the ordinal, not the rendition.
func TestMapPlanSubtitleRenditions_KeepsThePickerOrdinalAroundImageAndExternalTracks(t *testing.T) {
	rends := parsePlanSubtitleRenditions(prodPlanMasterWithSubs)
	ext := "/media/film.fr.srt"
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "eac3", Language: "fre", Title: "European"},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "fre", Title: "European (Forced)", IsForced: true},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "hdmv_pgs_subtitle", Language: "fre", Title: "PGS"},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "fre", Title: "European"},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "fre", IsExternal: true, ExternalPath: &ext},
	}
	got := mapPlanSubtitleRenditions(rends, streams)
	assert.Equal(t, []apptranscoding.SubtitleRendition{
		{TrackIdx: 0, Name: "European (Forced)", LanguageCode: "fre", URI: "od_sub1.m3u8", Forced: true},
		{TrackIdx: 2, Name: "European", LanguageCode: "fre", URI: "od_sub2.m3u8"},
	}, got)
	assert.Nil(t, mapPlanSubtitleRenditions(nil, streams))
}

// The packager's own master (pure copy path) gets the ordinal tag per line,
// matched by URI so a dropped rendition cannot shift the others.
func TestTagPlanMasterSubtitles_TagsEachRenditionByURI(t *testing.T) {
	rends := []apptranscoding.SubtitleRendition{
		{TrackIdx: 0, URI: "od_sub1.m3u8"},
		{TrackIdx: 2, URI: "od_sub2.m3u8"},
	}
	got := tagPlanMasterSubtitles(prodPlanMasterWithSubs, rends)
	assert.Contains(t, got, `NAME="European (Forced)",AUTOSELECT=NO,LANGUAGE="fre",DEFAULT=NO,FORCED=YES,X-EVEY-TRACK="0",URI="od_sub1.m3u8?v=v0.30.0"`)
	assert.Contains(t, got, `AUTOSELECT=NO,LANGUAGE="fre",X-EVEY-TRACK="2",URI="od_sub2.m3u8?v=v0.30.0"`)
	assert.Contains(t, got, `TYPE=AUDIO,GROUP-ID="aud",NAME="European",AUTOSELECT=YES,LANGUAGE="fre",DEFAULT=YES`, "audio flags untouched")
	assert.NotContains(t, got, `od_audio1.m3u8?v=v0.30.0"`+`,X-EVEY`, "audio lines untouched")
	assert.Equal(t, prodPlanMasterWithSubs, tagPlanMasterSubtitles(prodPlanMasterWithSubs, nil))
}

// Sprite Fright's packager master: the Portuguese title ends in a newline the
// packager escapes as "\n", and the Matroska implicit-English track of test5
// carries no LANGUAGE at all.
const spritePlanMaster = `#EXTM3U
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="Italiano",AUTOSELECT=YES,LANGUAGE="it",URI="sub4.m3u8"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="Português\n",AUTOSELECT=YES,LANGUAGE="pt",URI="sub5.m3u8"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="Русский",AUTOSELECT=YES,LANGUAGE="ru",URI="sub6.m3u8"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="Subtitles 1",AUTOSELECT=YES,URI="sub7.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=1,CODECS="vp09.00.40.08,opus",SUBTITLES="subs"
playlist.m3u8
`

// One title the packager spells differently ("Português\n" against a stored
// title ending in a newline) must not drop the tracks that follow it: every
// track stays, and the served lines carry the stored title and language.
func TestPlanSubtitleRenditions_AnEscapedTitleDropsNothingAfterIt(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "ita", Title: "Italiano"},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "por", Title: "Português\n"}, // a real newline, as probed
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "rus", Title: "Русский"},
		{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "eng"},
	}
	got := mapPlanSubtitleRenditions(parsePlanSubtitleRenditions(spritePlanMaster), streams)
	require.Len(t, got, 4, "the tracks after the escaped title were dropped")
	assert.Equal(t, 2, got[2].TrackIdx)

	served := tagPlanMasterSubtitles(spritePlanMaster, got)
	assert.Contains(t, served, `NAME="Português",AUTOSELECT=NO,LANGUAGE="pt"`)
	assert.Contains(t, served, `NAME="Subtitles 1",AUTOSELECT=NO,LANGUAGE="eng",X-EVEY-TRACK="3"`, "the implicit English gets its language")
	assert.NotContains(t, served, `\n`)
}

// The prewarm's full read is skipped for a browser on a packager-subtitled
// file; the apps and the receiver keep it, they still fetch /subtitle.
func TestBrowserUserAgent(t *testing.T) {
	assert.True(t, browserUserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:155.0) Gecko/20100101 Firefox/155.0"))
	assert.False(t, browserUserAgent("EveyMobile/0.0.1-beta.61 (Android 16; BVL-N49)"))
	assert.False(t, browserUserAgent("EveyTV (Android TV 14; SHIELD)"))
	assert.False(t, browserUserAgent("Mozilla/5.0 (X11; Linux armv7l) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.225 Safari/537.36 CrKey/1.56.500000 DeviceType/Chromecast"))
	assert.False(t, browserUserAgent("okhttp/4.12"))
}
