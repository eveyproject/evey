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
