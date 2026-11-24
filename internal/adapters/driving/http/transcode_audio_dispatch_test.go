package httpapi

// Tests for hlsVODServe.dispatchAudioPlaylist and dispatchAudioSegment.
//
// Both methods are thin routers that:
//   1. Short-circuit to 404 when h.audio is nil ("rendition not wired").
//   2. Parse the audio_T.m3u8 / audio_T_NNN.m4s / audio_T_init.mp4 file name
//      and forward with explicit (trackIdx, segIdx) ints to audioServe.
//
// Construction mirrors buildVODHandler + buildAudioServe exactly (same fakes,
// same package) so no real ffmpeg is spawned.
// The audioServe.segmentWaitTimeout is kept at 50 ms so tests that exercise
// the error path finish quickly.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// buildVODHandlerWithAudio wires an hlsVODServe that has a real *audioServe
// attached (feature flag on). The audioOrch is injectable so callers can
// supply a fakeAudioEnsurer that succeeds or fails on demand.
func buildVODHandlerWithAudio(
	t *testing.T,
	libSvc MediaStreamsGetter,
	libAccess LibraryAccessChecker,
	parental ParentalChecker,
	audioOrch AudioSegmentEnsurer,
) *hlsVODServe {
	t.Helper()
	h, _ := buildVODHandler(t, libSvc, libAccess, parental)
	// audioServe gets a separate cache dir so segments don't collide.
	audio := buildAudioServe(t, true /*flagOn*/, libSvc, libAccess, parental, audioOrch)
	audio.segmentWaitTimeout = 50 * time.Millisecond
	h.audio = audio
	return h
}

// doAudioDispatchRequest fires h.serveHTTP via the synthetic VOD path,
// which goes through commonAuth + dispatchAudio*.
func doAudioDispatchRequest(h *hlsVODServe, mediaID, file string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+mediaID+"/hls/v2/"+file, nil)
	req.SetPathValue("id", mediaID)
	req.SetPathValue("file", file)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	return w
}

// ----------------------------------------------------------------
// dispatchAudioPlaylist
// ----------------------------------------------------------------

// When h.audio is nil the handler returns 404 immediately, before parsing
// the file name.
func TestDispatchAudioPlaylist_AudioNil_404(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	// h.audio is nil by default from buildVODHandler.
	w := doAudioDispatchRequest(h, "m1", "audio_0.m3u8")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Valid audio_0.m3u8 request reaching a wired audioServe (flag on, valid media)
// must produce 200 + HLS content-type.
func TestDispatchAudioPlaylist_ValidRequest_200(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{}, &fakeAudioEnsurer{})

	w := doAudioDispatchRequest(h, "m1", "audio_0.m3u8")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/vnd.apple.mpegurl", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "#EXTM3U")
}

// track index 1 is forwarded correctly - the playlist mentions audio_1_init.mp4
// (not audio_0_init.mp4), proving the parsed index reaches audioServe.
func TestDispatchAudioPlaylist_TrackIdx1_ForwardedCorrectly(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{}, &fakeAudioEnsurer{})

	w := doAudioDispatchRequest(h, "m1", "audio_1.m3u8")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "audio_1_init.mp4",
		"playlist for track 1 must reference the track-1 init segment")
}

// Library-access denied propagates through commonAuth (403 before dispatch).
func TestDispatchAudioPlaylist_LibraryAccessDenied_403(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, denyAll(), testFakeParentalChecker{}, &fakeAudioEnsurer{})

	w := doAudioDispatchRequest(h, "m1", "audio_0.m3u8")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ----------------------------------------------------------------
// dispatchAudioSegment - init variant
// ----------------------------------------------------------------

// audio_T_init.mp4 with h.audio nil → 404.
func TestDispatchAudioSegment_InitAudioNil_404(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	w := doAudioDispatchRequest(h, "m1", "audio_0_init.mp4")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// The init must be asked for as an INIT, never as segment 0. A viewer resuming deep
// into a title (another browser, last position 1h35) has one audio session, started
// at their position, that never writes segment 0: asking for it there made the init
// time out and the title play with a picture and no sound.
func TestDispatchAudioSegment_Init_NeverAsksForSegmentZero(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	ensurer := &fakeAudioEnsurer{}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{}, ensurer)

	doAudioDispatchRequest(h, "m1", "audio_0_init.mp4")

	assert.Equal(t, 1, ensurer.initCalls, "the init must be requested as an init")
	assert.Empty(t, ensurer.calls, "the init must not hinge on any media segment")
}

// audio_0_init.mp4 with wired audioServe but ensurer failing → 503.
func TestDispatchAudioSegment_Init_OrchestratorError_503(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{},
		&fakeAudioEnsurer{err: errTest})

	w := doAudioDispatchRequest(h, "m1", "audio_0_init.mp4")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// ----------------------------------------------------------------
// dispatchAudioSegment - numbered segment variant
// ----------------------------------------------------------------

// audio_T_NNN.m4s with h.audio nil → 404.
func TestDispatchAudioSegment_SegAudioNil_404(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	w := doAudioDispatchRequest(h, "m1", "audio_0_005.m4s")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// A segment past the recorded end of the audio stream is GONE, not a transient
// 503: hls.js retries 503s and every retry used to respawn a doomed ffmpeg
// against the same wall (prod 2026-07-26: 92 spawns for one segment of a track
// ending 4 min before the container's declared duration). 410 is the stable
// "this will never exist" answer.
func TestDispatchAudioSegment_PastEndOfStream_410(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{},
		&fakeAudioEnsurer{err: apptranscoding.ErrAudioPastEndOfStream})

	w := doAudioDispatchRequest(h, "m1", "audio_0_1914.m4s")
	assert.Equal(t, http.StatusGone, w.Code)
}

// audio_0_005.m4s with wired audioServe but ensurer failing → 503.
func TestDispatchAudioSegment_Seg_OrchestratorError_503(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{},
		&fakeAudioEnsurer{err: errTest})

	w := doAudioDispatchRequest(h, "m1", "audio_0_005.m4s")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// Parental check fires inside commonAuth before dispatch - dispatch path
// never runs when the item is restricted.
func TestDispatchAudioSegment_ParentalBlocked_NotFound(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(),
		testFakeParentalChecker{block: true}, &fakeAudioEnsurer{})

	w := doAudioDispatchRequest(h, "m1", "audio_0_000.m4s")
	assert.Equal(t, http.StatusNotFound, w.Code) // parental block hides existence (404, not 403)
}

// Deep segment index (> 999) is parsed correctly - audioSegmentNameRE uses
// \d+ (not \d{3}) so segment 1000 must not 404 on routing.
func TestDispatchAudioSegment_DeepIndex_NotRoutedAs404(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{},
		&fakeAudioEnsurer{err: errTest})

	// audio_0_1000.m4s - index 1000 exceeds the 3-digit minimum.
	// If the RE or parse was broken it would fall through to the default 404.
	// With the ensurer returning an error we expect 503, proving dispatch worked.
	w := doAudioDispatchRequest(h, "m1", "audio_0_1000.m4s")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code,
		"deep audio segment index must route to dispatchAudioSegment, not fall through to 404")
}

// Verify that the fakeAudioEnsurer records the correct segment index when
// dispatchAudioSegment forwards to serveSegmentFile.
func TestDispatchAudioSegment_SegIdx_ForwardedCorrectly(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	ensurer := &fakeAudioEnsurer{err: errTest} // fail fast so we can inspect calls
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{}, ensurer)

	doAudioDispatchRequest(h, "m1", "audio_0_007.m4s")

	require.Len(t, ensurer.calls, 1, "EnsureAudioSegment must have been called once")
	assert.Equal(t, 7, ensurer.calls[0], "segment index 7 must reach the ensurer")
}

// The rendition's init and segments must be asked under the same plan as the
// playlist: they re-decide the video to put the audio on its timeline, and a
// transcoded rung's rendition without vd= was decided copy there - served at 0
// while its video presents a few frames late.
func TestDispatchAudioPlaylist_CarriesThePlanPins(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	h := buildVODHandlerWithAudio(t, libSvc, allowAll(), testFakeParentalChecker{}, &fakeAudioEnsurer{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/audio_0.m3u8?vd=transcode&vc=h264&ad=transcode&fmp4AudioPassthrough=mp3%2Copus", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "audio_0.m3u8")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "audio_0_init.mp4")
	for line := range strings.SplitSeq(w.Body.String(), "\n") {
		if !strings.Contains(line, "audio_0_") {
			continue
		}
		for _, pin := range []string{"vd=transcode", "vc=h264", "ad=transcode"} {
			assert.Contains(t, line, pin, "child URI must carry the plan's pin")
		}
		// The passthrough opt-in is a decision input too: without it the init
		// and segments re-decide to AAC under a master that promised the copy.
		assert.Contains(t, line, "fmp4AudioPassthrough=mp3%2Copus", "child URI must carry the passthrough opt-in")
	}
}
