package httpapi

// What the session is TOLD about the audio it is being served. The decision
// engine was right all along here; the reporting was not, and a wrong report
// is what sends someone to "fix" a path that works.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainplayback "github.com/evey-project/evey/internal/domain/playback"
)

// ladderWithAC3 has the SHAPE of the living-room case of 2026-09-19: a 4K HEVC
// film whose default track is AC-3 5.1, played by a television that decodes
// AC-3 but whose video must be re-encoded (there it was the tone-map, here the
// codec - the shape is what matters). The ladder is multi-rung, so the MUXED
// copy gate (AudioCopyEligible, single-rung by contract) refuses, and the audio
// rides its own #EXT-X-MEDIA rendition, copied verbatim.
func ladderWithAC3(t *testing.T) (*hlsVODServe, []*domainlibrary.MediaStream) {
	t.Helper()
	w, hgt := 3840, 2160
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &hgt, Profile: "Main 10"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: new(6), IsDefault: true, Language: "fre"},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: testMediaItemWith1Minute(), streams: streams}, allowAll(), testFakeParentalChecker{block: false})
	h.audioRenditionEnabled = true
	h.frameRateProbe = func(context.Context, string) float64 { return 24 }
	h.encoderReorderProbe = func(context.Context, string, string, string, []string) (int, bool) { return 2, true }
	return h, streams
}

// The report the admin panel and the stream sheet read. It said "transcode"
// over an ffmpeg running `-c:a copy`, because it only ever asked the muxed
// gate - which a multi-rung ladder can never satisfy.
func TestHLSVOD_ServingPath_ReportsARenditionCopyAsCopy(t *testing.T) {
	h, _ := ladderWithAC3(t)
	var got domainplayback.ServingPath
	h.servingRecord = func(_ context.Context, _, _ string, p domainplayback.ServingPath) { got = p }

	tv := "videoCodecs=h264&audioCodecs=aac,ac3,eac3&maxAudioChannels=6"
	rec := doVODRequestQ(h, "m1", "master.m3u8", tv)
	require.Equal(t, http.StatusOK, rec.Code)

	// The premise of the case: the video really is re-encoded on a ladder of
	// more than one rung, so this is NOT the single-rung muxed copy.
	assert.Equal(t, "transcode", got.VideoDecision)
	assert.Contains(t, rec.Body.String(), "#EXT-X-MEDIA:TYPE=AUDIO", "the audio must ride its own rendition here")
	assert.Equal(t, "copy", got.AudioDecision, "the rendition carries the AC-3 verbatim - saying transcode is a lie about the bytes")
	assert.Equal(t, "ffmpeg", got.AudioEngine, "ffmpeg still segments it, it just does not re-encode it")
}

// The other direction, so the fix cannot degrade into "always copy": a client
// that cannot take AC-3 gets a real AAC re-encode, and the report says so.
func TestHLSVOD_ServingPath_ReportsARealReencodeAsTranscode(t *testing.T) {
	h, _ := ladderWithAC3(t)
	var got domainplayback.ServingPath
	h.servingRecord = func(_ context.Context, _, _ string, p domainplayback.ServingPath) { got = p }

	web := "videoCodecs=h264&audioCodecs=aac,mp3&maxAudioChannels=6"
	require.Equal(t, http.StatusOK, doVODRequestQ(h, "m1", "master.m3u8", web).Code)
	assert.Equal(t, "transcode", got.AudioDecision)
}

// A resync offset filters the samples, so the rendition must re-encode even
// for a client that decodes the source codec - and the report follows.
func TestHLSVOD_ServingPath_AResyncOffsetIsNotACopy(t *testing.T) {
	h, _ := ladderWithAC3(t)
	var got domainplayback.ServingPath
	h.servingRecord = func(_ context.Context, _, _ string, p domainplayback.ServingPath) { got = p }

	tv := "videoCodecs=h264&audioCodecs=aac,ac3,eac3&maxAudioChannels=6&audioOffsetMs=200"
	require.Equal(t, http.StatusOK, doVODRequestQ(h, "m1", "master.m3u8", tv).Code)
	assert.Equal(t, "transcode", got.AudioDecision)
}

// Unit view of the same rule, including the muxed short-circuit and the
// no-rendition case - the two branches the master requests above cannot reach
// on their own.
func TestAudioServedAsCopy_EachBranch(t *testing.T) {
	h, streams := ladderWithAC3(t)
	item := testMediaItemWith1Minute()
	req := func(q string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?"+q, nil)
		return withHTTPClaims(r, testClaims(false))
	}
	tv := req("videoCodecs=h264&audioCodecs=aac,ac3,eac3&maxAudioChannels=6")

	assert.True(t, h.audioServedAsCopy(tv, item, streams, true, false, 0),
		"the muxed single-rung copy still reports copy, untouched")
	assert.False(t, h.audioServedAsCopy(tv, item, streams, false, false, 0),
		"no rendition and no muxed copy: the audio really is re-encoded")
	assert.True(t, h.audioServedAsCopy(tv, item, streams, false, true, 0),
		"rendition path, carriable codec: copy")
	assert.False(t, h.audioServedAsCopy(req("videoCodecs=h264&audioCodecs=aac"), item, streams, false, true, 0),
		"rendition path, codec the client cannot take: re-encode")
}
