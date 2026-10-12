package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// supersede4KItem is a 4K HEVC source: the client caps (h264 only) force the
// transcode ladder, which is the only path that takes a video-encode slot.
func supersede4KItem(t *testing.T) (*domainlibrary.MediaItem, []*domainlibrary.MediaStream) {
	t.Helper()
	w, hgt := 3840, 2160
	dur, err := shared.NewDurationMs(60_000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &hgt},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "eac3", Language: "fr", IsDefault: true},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "eac3", Language: "en"},
	}
	return item, streams
}

func doVODRequestAs(h *hlsVODServe, streamToken, mediaID, file, rawQuery string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+mediaID+"/hls/v2/"+file+"?"+rawQuery, nil)
	req.SetPathValue("id", mediaID)
	req.SetPathValue("file", file)
	req = withHTTPClaims(req, testClaims(false))
	req = req.WithContext(context.WithValue(req.Context(), rawTokenCtxKey, streamToken))
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	return w
}

// The bug: ONE viewer switching audio track re-fingerprints the same viewing
// session. The hash they left keeps its video-encode slot for minutes (the
// paused/idle sweeps), so on a box calibrated at one concurrent encode the
// switch is refused with "too many streams" - by the viewer's own abandoned
// encoder. The handler must hand that hash back the moment the same stream
// token lands on a new one.
func TestHLSVOD_AudioTrackSwitch_ReclaimsTheAbandonedHash(t *testing.T) {
	item, streams := supersede4KItem(t)
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})

	var reclaimed []string
	h.reclaimHash = func(hash, _ string) int { reclaimed = append(reclaimed, hash); return 1 }

	first := doVODRequestAs(h, "tok-viewer", "m1", "master.m3u8", "audioTrack=0")
	require.Equal(t, http.StatusOK, first.Code)
	require.Empty(t, reclaimed, "a first master takes a hash, it supersedes nothing")

	// Same master again: the steady-state re-fetch must never reclaim anything.
	require.Equal(t, http.StatusOK, doVODRequestAs(h, "tok-viewer", "m1", "master.m3u8", "audioTrack=0").Code)
	require.Empty(t, reclaimed, "re-fetching the same master must not reclaim its own encoder")

	// The switch.
	second := doVODRequestAs(h, "tok-viewer", "m1", "master.m3u8", "audioTrack=1")
	require.Equal(t, http.StatusOK, second.Code)
	require.Len(t, reclaimed, 1, "the abandoned hash must be reclaimed on the switch")
	assert.NotEmpty(t, reclaimed[0])
}

// A second viewer on the track the first one just left must keep their
// encoder: reclaiming there would cut a live playback.
func TestHLSVOD_AudioTrackSwitch_SecondViewerOnTheOldTrackIsProtected(t *testing.T) {
	item, streams := supersede4KItem(t)
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})

	var reclaimed []string
	h.reclaimHash = func(hash, _ string) int { reclaimed = append(reclaimed, hash); return 1 }

	require.Equal(t, http.StatusOK, doVODRequestAs(h, "tok-a", "m1", "master.m3u8", "audioTrack=0").Code)
	require.Equal(t, http.StatusOK, doVODRequestAs(h, "tok-b", "m1", "master.m3u8", "audioTrack=0").Code)

	require.Equal(t, http.StatusOK, doVODRequestAs(h, "tok-a", "m1", "master.m3u8", "audioTrack=1").Code)
	assert.Empty(t, reclaimed, "tok-b is still watching that track: its encoder must survive")

	require.Equal(t, http.StatusOK, doVODRequestAs(h, "tok-b", "m1", "master.m3u8", "audioTrack=1").Code)
	assert.Len(t, reclaimed, 1, "once the LAST viewer leaves the track, its encoder is dead weight")
}

// prewarm-next and cast-next build the NEXT episode's fingerprint while the
// CURRENT one is still playing, under that playback's own stream token. Such a
// resolution must leave the registry alone: binding it moved the token onto the
// next episode's hash and handed the hash of the episode being watched to
// reclaimHash, killing the encoder feeding the viewer - in the last minute of
// the episode, which is exactly when the prewarm fires.
func TestHLSVOD_NextEpisodePrewarm_DoesNotReclaimTheEpisodeBeingWatched(t *testing.T) {
	item, streams := supersede4KItem(t)
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})

	var reclaimed []string
	h.reclaimHash = func(hash, _ string) int { reclaimed = append(reclaimed, hash); return 1 }

	require.Equal(t, http.StatusOK, doVODRequestAs(h, "tok-viewer", "m1", "master.m3u8", "audioTrack=0").Code)
	require.Empty(t, reclaimed)

	next, nextStreams := supersede4KItem(t)
	next.ID = "m2"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/m1/prewarm-next", nil)
	req = withHTTPClaims(req, testClaims(false))
	req = req.WithContext(context.WithValue(req.Context(), rawTokenCtxKey, "tok-viewer"))
	_, err := h.buildFingerprint(req, "m2", next, nextStreams)
	require.NoError(t, err)

	assert.Empty(t, reclaimed,
		"warming the next episode must not report the one playing as superseded")
}

// A request with no stream token (share links, native players before the
// cookie lands) must stay inert rather than collapsing every anonymous viewer
// onto one registry entry and reclaiming each other's encoders.
func TestHLSVOD_NoStreamToken_NeverReclaims(t *testing.T) {
	item, streams := supersede4KItem(t)
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})

	var reclaimed []string
	h.reclaimHash = func(hash, _ string) int { reclaimed = append(reclaimed, hash); return 1 }

	require.Equal(t, http.StatusOK, doVODRequestAs(h, "", "m1", "master.m3u8", "audioTrack=0").Code)
	require.Equal(t, http.StatusOK, doVODRequestAs(h, "", "m1", "master.m3u8", "audioTrack=1").Code)
	assert.Empty(t, reclaimed)
}
