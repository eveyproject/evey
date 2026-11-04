package httpapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

// A packager that numbers its renditions with the file's own track number:
// three audio tracks of one title, the middle one left out, and the third
// renamed "Stereo 2" because the second is gone. By name alone "Stereo 2"
// reads as the second source track, which is the one the plan dropped.
const trackNumberedAudioMaster = `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="Stereo",LANGUAGE="eng",X-MKVGO-TRACK="2",URI="od_audio1.m3u8?v=1"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="Stereo 2",LANGUAGE="eng",X-MKVGO-TRACK="4",URI="od_audio2.m3u8?v=1"
#EXT-X-STREAM-INF:BANDWIDTH=1000000,AUDIO="aud"
od_playlist.m3u8?v=1
`

func sameTitleAudioStreams(withTrackIDs bool) []*domainlibrary.MediaStream {
	id := func(n int) *int {
		if !withTrackIDs {
			return nil
		}
		return new(n)
	}
	return []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", SourceTrackID: id(1)},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Language: "eng", Title: "Stereo", SourceTrackID: id(2)},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "opus", Language: "eng", Title: "Stereo", SourceTrackID: id(3)},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Language: "eng", Title: "Stereo", SourceTrackID: id(4)},
	}
}

func TestParsePlanRenditions_ReadTheFileTrackNumber(t *testing.T) {
	rends := parsePlanAudioRenditions(trackNumberedAudioMaster)
	require.Len(t, rends, 2)
	assert.Equal(t, 2, rends[0].track)
	assert.Equal(t, 4, rends[1].track)
	assert.Equal(t, "od_audio2.m3u8?v=1", rends[1].uri)

	subs := parsePlanSubtitleRenditions(`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="French",LANGUAGE="fre",X-MKVGO-TRACK="7",URI="od_sub1.m3u8"`)
	require.Len(t, subs, 1)
	assert.Equal(t, 7, subs[0].track)

	bare := parsePlanAudioRenditions(prodPlanMaster)
	require.NotEmpty(t, bare)
	assert.Zero(t, bare[0].track, "a master without the attribute declares no number")
	odd := parsePlanAudioRenditions(`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="A",X-MKVGO-TRACK="x",URI="a.m3u8"`)
	require.Len(t, odd, 1)
	assert.Zero(t, odd[0].track, "an unreadable number is no number")
}

// The number pairs the rendition with the track that carries it, around the
// track the plan left out and through the colliding names.
func TestPlanRenditionURIFor_PairsByFileTrackNumber(t *testing.T) {
	rends := parsePlanAudioRenditions(trackNumberedAudioMaster)
	streams := sameTitleAudioStreams(true)

	first, ok := planRenditionURIFor(rends, streams, 0)
	require.True(t, ok)
	assert.Equal(t, "od_audio1.m3u8", first)

	_, ok = planRenditionURIFor(rends, streams, 1)
	assert.False(t, ok, "the track the plan left out has no rendition, whatever the names say")

	third, ok := planRenditionURIFor(rends, streams, 2)
	require.True(t, ok)
	assert.Equal(t, "od_audio2.m3u8", third, "track 4 is the plan's second rendition")
}

// Streams stored before their number was probed fall back on the name: the
// numbered master pairs exactly as an unnumbered one would.
func TestPlanRenditionURIFor_StreamsWithoutNumberFallBackOnTheName(t *testing.T) {
	numbered := parsePlanAudioRenditions(`#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="[VFF] AC3 5.1",LANGUAGE="fre",X-MKVGO-TRACK="3",URI="od_audio1.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="[VFQ] AC3 5.1",LANGUAGE="fre",X-MKVGO-TRACK="4",URI="od_audio2.m3u8"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aud",NAME="AC3 5.1",X-MKVGO-TRACK="5",URI="od_audio3.m3u8"
`)
	streams := prodAudioStreams()
	for track, want := range map[int]string{1: "od_audio1.m3u8", 2: "od_audio2.m3u8", 3: "od_audio3.m3u8"} {
		got, ok := planRenditionURIFor(numbered, streams, track)
		require.True(t, ok, track)
		assert.Equal(t, want, got, track)
	}
	_, ok := planRenditionURIFor(numbered, streams, 0)
	assert.False(t, ok)
}

// A master without the attribute pairs as before, numbered streams or not.
func TestPlanRenditionURIFor_UnnumberedMasterIgnoresStreamNumbers(t *testing.T) {
	rends := parsePlanAudioRenditions(prodPlanMaster)
	streams := prodAudioStreams()
	for i, s := range streams {
		s.SourceTrackID = new(i + 1)
	}
	_, ok := planRenditionURIFor(rends, streams, 0)
	assert.False(t, ok)
	for track, want := range map[int]string{1: "od_audio1.m3u8", 2: "od_audio2.m3u8", 3: "od_audio3.m3u8"} {
		got, ok := planRenditionURIFor(rends, streams, track)
		require.True(t, ok, track)
		assert.Equal(t, want, got, track)
	}
}

// When both sides carry a number it is the only evidence: a rendition with
// another number is another track even under the very same name, and the
// same number pairs even when the names disagree.
func TestRenditionIsTrack_NumberDecidesOverTheName(t *testing.T) {
	s := &domainlibrary.MediaStream{StreamType: domainlibrary.StreamTypeAudio, Title: "Stereo", Language: "eng", SourceTrackID: new(3)}
	assert.False(t, renditionIsTrack(planAudioRendition{name: "Stereo", lang: "eng", track: 4}, s))
	assert.True(t, renditionIsTrack(planAudioRendition{name: "Stereo 2", lang: "fre", track: 3}, s))
	assert.True(t, renditionIsTrack(planAudioRendition{name: "Stereo", lang: "eng"}, s), "no number on the rendition: the name decides")
	s.SourceTrackID = nil
	assert.True(t, renditionIsTrack(planAudioRendition{name: "Stereo", lang: "eng", track: 4}, s), "no number on the track: the name decides")
}

// Subtitles pair the same way: the second French track is left out, the
// third is published as "French 2".
func TestMapPlanSubtitleRenditions_PairsByFileTrackNumber(t *testing.T) {
	master := `#EXTM3U
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="French",LANGUAGE="fre",X-MKVGO-TRACK="4",URI="od_sub1.m3u8?v=1"
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="French 2",LANGUAGE="fre",X-MKVGO-TRACK="6",URI="od_sub2.m3u8?v=1"
`
	streams := func(withTrackIDs bool) []*domainlibrary.MediaStream {
		id := func(n int) *int {
			if !withTrackIDs {
				return nil
			}
			return new(n)
		}
		return []*domainlibrary.MediaStream{
			{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", SourceTrackID: id(1)},
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "fre", Title: "French", SourceTrackID: id(4)},
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "fre", Title: "French", SourceTrackID: id(5)},
			{StreamType: domainlibrary.StreamTypeSubtitle, Codec: "subrip", Language: "fre", Title: "French", SourceTrackID: id(6)},
		}
	}
	rends := parsePlanSubtitleRenditions(master)

	assert.Equal(t, []apptranscoding.SubtitleRendition{
		{TrackIdx: 0, Name: "French", LanguageCode: "fre", URI: "od_sub1.m3u8"},
		{TrackIdx: 2, Name: "French", LanguageCode: "fre", URI: "od_sub2.m3u8"},
	}, mapPlanSubtitleRenditions(rends, streams(true)))

	// Without the numbers the name is all there is, and it reads "French 2"
	// as the second track: the fallback is the previous pairing, unchanged.
	assert.Equal(t, []apptranscoding.SubtitleRendition{
		{TrackIdx: 0, Name: "French", LanguageCode: "fre", URI: "od_sub1.m3u8"},
		{TrackIdx: 1, Name: "French", LanguageCode: "fre", URI: "od_sub2.m3u8"},
	}, mapPlanSubtitleRenditions(rends, streams(false)))
}
