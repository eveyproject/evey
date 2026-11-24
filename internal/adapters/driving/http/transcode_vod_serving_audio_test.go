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

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
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
	h := buildVODHandlerWithAudio(t, &testFakeStreamsGetter{item: testMediaItemWith1Minute(), streams: streams}, allowAll(), testFakeParentalChecker{block: false}, &fakeAudioEnsurer{})
	h.audioRenditionEnabled = true
	h.frameRateProbe = func(context.Context, string) float64 { return 24 }
	h.encoderReorderProbe = func(context.Context, string, string, string, []string) (int, bool) { return 2, true }
	return h, streams
}

// renditionStamp is what the served witness is told an audio rendition
// response carries, for a client declaring caps: the same fingerprint the
// rendition's ffmpeg runs under.
func renditionStamp(t *testing.T, h *hlsVODServe, streams []*domainlibrary.MediaStream, caps string) *deliveryStamp {
	t.Helper()
	item := testMediaItemWith1Minute()
	st := &deliveryStamp{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/audio_0_init.mp4?"+caps, nil)
	req = withHTTPClaims(req, testClaims(false))
	req = req.WithContext(context.WithValue(req.Context(), deliveryStampKey{}, st))
	fp := h.audio.buildAudioFingerprint(req, "m1", item, 0, 0, h.audioRenditionCopyCodec(req, item, streams, 0))
	h.audio.stampRendition(req, "m1", 0, fp, "/nonexistent/init.mp4")
	require.True(t, st.audio.set)
	return st
}

// A rendition of a multi-rung ladder carries the AC-3 verbatim for a TV that
// decodes it: it said "transcode" when the report asked the muxed gate, which
// a multi-rung ladder never satisfies. The served leg now comes from the
// fingerprint ffmpeg runs under.
func TestAudioRendition_StampsACopyAsCopy(t *testing.T) {
	h, streams := ladderWithAC3(t)
	st := renditionStamp(t, h, streams, "videoCodecs=h264&audioCodecs=aac,ac3,eac3&maxAudioChannels=6")
	assert.True(t, st.audio.copy, "the rendition carries the AC-3 verbatim")
	assert.Equal(t, "ffmpeg", st.audio.engine, "ffmpeg still segments it, it just does not re-encode it")
	assert.Equal(t, 0, st.audio.track)
}

// The other direction, so it cannot degrade into "always copy": a client that
// cannot take AC-3 gets a real AAC re-encode.
func TestAudioRendition_StampsAReencodeAsEncoded(t *testing.T) {
	h, streams := ladderWithAC3(t)
	st := renditionStamp(t, h, streams, "videoCodecs=h264&audioCodecs=aac,mp3&maxAudioChannels=6")
	assert.False(t, st.audio.copy)
}

// A resync offset filters the samples, so the rendition re-encodes even for a
// client that decodes the source codec.
func TestAudioRendition_AResyncOffsetIsNotACopy(t *testing.T) {
	h, streams := ladderWithAC3(t)
	st := renditionStamp(t, h, streams, "videoCodecs=h264&audioCodecs=aac,ac3,eac3&maxAudioChannels=6&audioOffsetMs=200")
	assert.False(t, st.audio.copy)
}

// The ad= pin is minted from the SELECTED track and rides every child URL,
// including the playlists of renditions it says nothing about. Applied there
// it did real damage: pick the TrueHD track nobody can carry in fMP4, and the
// AC-3 tracks beside it were re-encoded to AAC on the strength of a verdict
// about a track the viewer is not listening to.
func TestHLSVOD_AudioPin_DoesNotJudgeAnotherTrack(t *testing.T) {
	w, hgt := 1920, 1080
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &hgt, Profile: "Main"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "truehd", Channels: new(8), IsDefault: true, Language: "eng"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: new(6), Language: "fre"},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: testMediaItemWith1Minute(), streams: streams}, allowAll(), testFakeParentalChecker{block: false})
	h.audioRenditionEnabled = true
	h.frameRateProbe = func(context.Context, string) float64 { return 24 }
	h.encoderReorderProbe = func(context.Context, string, string, string, []string) (int, bool) { return 2, true }
	item := testMediaItemWith1Minute()

	// The selected track is the TrueHD one: fMP4 cannot carry it, so the
	// session's own verdict is transcode and the master stamps ad=transcode.
	caps := "videoCodecs=h264&audioCodecs=aac,ac3,eac3,truehd&maxAudioChannels=8&audioTrack=0"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/audio_1.m3u8?"+caps+"&ad=transcode", nil)
	req = withHTTPClaims(req, testClaims(false))

	assert.Empty(t, h.audioRenditionCopyCodec(req, item, streams, 0), "TrueHD is never carried in fMP4")
	assert.Equal(t, "ac3", h.audioRenditionCopyCodec(req, item, streams, 1),
		"the AC-3 rendition is carriable on its own merits - the other track's pin must not re-encode it")
}
