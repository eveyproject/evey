package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// ?maxHeight= is what the mobile/TV quality menu and the cast receiver's
// device profile send: the master must stop its ladder there and bake the
// value into every child URL. It was parsed by GetStreamInfo only, so the
// quality menu changed the stream-info verdict and nothing in the stream.
func TestHLSVOD_Master_HonoursMaxHeight(t *testing.T) {
	w, hgt := 3840, 2160
	v := &domainlibrary.MediaStream{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &hgt, Profile: "Main"}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute(), streams: []*domainlibrary.MediaStream{v}},
		allowAll(), testFakeParentalChecker{block: false})
	h.capability = func() domaintranscoding.HostCapabilitySnapshot {
		return domaintranscoding.EncodeCapability{SoftwareMaxHeight: 2160}.Snapshot()
	}

	rec := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264&maxHeight=480")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	heights := regexp.MustCompile(`RESOLUTION=\d+x(\d+)`).FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, heights)
	for _, m := range heights {
		n, _ := strconv.Atoi(m[1])
		assert.LessOrEqual(t, n, 480, "rung above the declared ceiling: %s", m[0])
	}
	assert.Contains(t, body, "maxHeight=480", "the ceiling must ride the child URLs")

	unbounded := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264").Body.String()
	assert.Contains(t, unbounded, "RESOLUTION=1920x1080", "without the ceiling the ladder keeps its taller rungs")
}

// The ffmpeg audio rendition copies a track the client decodes and the fMP4
// packaging carries; it re-encodes to AAC only what it must. The master's
// CODECS names every codec the rendition group really carries.
func TestHLSVOD_AudioRendition_CopiesWhatTheClientDecodes(t *testing.T) {
	w, hgt := 1920, 1080
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &hgt, Profile: "Main"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: new(6), IsDefault: true, Language: "fre"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "dts", Channels: new(6), Language: "eng"},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: testMediaItemWith1Minute(), streams: streams}, allowAll(), testFakeParentalChecker{block: false})
	h.audioRenditionEnabled = true
	h.frameRateProbe = func(context.Context, string) float64 { return 24 }
	h.encoderReorderProbe = func(context.Context, string, string, string, []string) (int, bool) { return 2, true }
	item := testMediaItemWith1Minute()

	// A TV that decodes AC-3: the AC-3 track is copied, the DTS one re-encoded.
	tv := "videoCodecs=h264&audioCodecs=aac,ac3,eac3&maxAudioChannels=6"
	rec := doVODRequestQ(h, "m1", "master.m3u8", tv)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `CODECS="avc1.64001f,ac-3,mp4a.40.2"`, "the group carries a copied AC-3 and a re-encoded DTS")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/audio_0.m3u8?"+tv, nil)
	req = withHTTPClaims(req, testClaims(false))
	assert.Equal(t, "ac3", h.audioRenditionCopyCodec(req, item, streams, 0))
	assert.Empty(t, h.audioRenditionCopyCodec(req, item, streams, 1), "DTS is never carried")

	// A browser without AC-3: both re-encoded, one codec advertised.
	web := "videoCodecs=h264&audioCodecs=aac,mp3&maxAudioChannels=6"
	body = doVODRequestQ(h, "m1", "master.m3u8", web).Body.String()
	assert.Contains(t, body, `CODECS="avc1.64001f,mp4a.40.2"`)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/audio_0.m3u8?"+web, nil)
	req = withHTTPClaims(req, testClaims(false))
	assert.Empty(t, h.audioRenditionCopyCodec(req, item, streams, 0))

	// A resync offset needs the samples re-encoded: no copy.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/audio_0.m3u8?"+tv+"&audioOffsetMs=200", nil)
	req = withHTTPClaims(req, testClaims(false))
	assert.Empty(t, h.audioRenditionCopyCodec(req, item, streams, 0))
}
