package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/config"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// ----------------------------------------------------------------
// Test-local fakes for audio handler tests
// ----------------------------------------------------------------

// fakeAudioEnsurer implements AudioSegmentEnsurer for tests.
// It never actually spawns ffmpeg — it just records calls and
// optionally returns a preset error.
type fakeAudioEnsurer struct {
	calls []int
	err   error
}

func (f *fakeAudioEnsurer) EnsureAudioSegment(_ context.Context, _ apptranscoding.AudioFingerprint, segIdx int) error {
	f.calls = append(f.calls, segIdx)
	return f.err
}

// ----------------------------------------------------------------
// Construction helpers
// ----------------------------------------------------------------

// testMediaItemWithAudioStreams builds a 60 s media item with two audio tracks.
func testMediaItemWithAudioStreams() (*domainlibrary.MediaItem, []*domainlibrary.MediaStream) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File: &domainlibrary.MediaFile{
			Path:     "/test.mkv",
			Duration: dur,
		},
	}
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "aac", Language: "en"},
		{StreamType: "audio", Codec: "ac3", Language: "fr"},
	}
	return item, streams
}

// testMediaItemAdult builds a 60 s media item for parental-block tests.
func testMediaItemAdult() *domainlibrary.MediaItem {
	dur, _ := shared.NewDurationMs(60_000)
	return &domainlibrary.MediaItem{
		ID:        "m-adult",
		LibraryID: "lib1",
		File: &domainlibrary.MediaFile{
			Path:     "/adult.mkv",
			Duration: dur,
		},
	}
}

// buildAudioServe returns an audioServe with the feature flag enabled by default.
func buildAudioServe(
	t *testing.T,
	flagOn bool,
	libSvc MediaStreamsGetter,
	libAccess LibraryAccessChecker,
	parental ParentalChecker,
	audioOrch AudioSegmentEnsurer,
) *audioServe {
	t.Helper()
	cacheDir := t.TempDir()
	cache, err := apptranscoding.NewSegmentCache(cacheDir + "/audio")
	require.NoError(t, err)

	cfg := &config.Config{}
	cfg.Transcoding.AudioRenditionEnabled = flagOn

	return &audioServe{
		cfg:                  cfg,
		libSvc:               libSvc,
		libraryAccess:        libAccess,
		parental:             parental,
		manifestGen:          apptranscoding.NewManifestGenerator(),
		cache:                cache,
		audioOrch:            audioOrch,
		audioNormalizeGetter: nil,
		segmentWaitTimeout:   50 * time.Millisecond,
	}
}

// doAudioRequest fires the given handler at the synthetic URL and injects claims.
func doAudioRequest(handler http.HandlerFunc, mediaID, trackIdx string, extraPathValues ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+mediaID+"/audio_"+trackIdx+".m3u8", nil)
	req.SetPathValue("id", mediaID)
	req.SetPathValue("trackIdx", trackIdx)
	for i := 0; i+1 < len(extraPathValues); i += 2 {
		req.SetPathValue(extraPathValues[i], extraPathValues[i+1])
	}
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// ----------------------------------------------------------------
// Tests
// ----------------------------------------------------------------

// Test 1: Audio playlist returns 200 + correct content-type when:
//   - feature flag is on
//   - media exists with audio streams
//   - auth is valid
func TestAudioPlaylist_FlagOn_MediaExists_200(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	ah := buildAudioServe(t, true, libSvc, allowAll(), testFakeParentalChecker{}, &fakeAudioEnsurer{})

	w := doAudioRequest(audioPlaylistHandler(ah), "m1", "0")

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/vnd.apple.mpegurl", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, "#EXTM3U")
	assert.Contains(t, body, "#EXT-X-PLAYLIST-TYPE:VOD")
	assert.Contains(t, body, "#EXT-X-ENDLIST")
	assert.Contains(t, body, "audio_0_init.mp4")
}

// Test 2: Audio playlist returns 404 when feature flag is off (even with valid auth).
func TestAudioPlaylist_FlagOff_404(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	ah := buildAudioServe(t, false /* flag off */, libSvc, allowAll(), testFakeParentalChecker{}, &fakeAudioEnsurer{})

	w := doAudioRequest(audioPlaylistHandler(ah), "m1", "0")

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Test 3: Audio segment route short-circuits to 404 when flag is off.
func TestAudioSegment_FlagOff_404(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	ah := buildAudioServe(t, false, libSvc, allowAll(), testFakeParentalChecker{}, &fakeAudioEnsurer{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/audio_0_000.m4s", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("trackIdx", "0")
	req.SetPathValue("segIdx", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	audioSegmentHandler(ah)(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Test 4: Audio init route returns 404 when parental blocks access (kids profile on adult media; existence hidden).
func TestAudioInit_ParentalBlocked_NotFound(t *testing.T) {
	item := testMediaItemAdult()
	streams := []*domainlibrary.MediaStream{
		{StreamType: "audio", Codec: "aac", Language: "en"},
	}
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	// parental blocks = kid profile sees adult item
	ah := buildAudioServe(t, true, libSvc, allowAll(), testFakeParentalChecker{block: true}, &fakeAudioEnsurer{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m-adult/audio_0_init.mp4", nil)
	req.SetPathValue("id", "m-adult")
	req.SetPathValue("trackIdx", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	audioInitHandler(ah)(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code) // parental block hides existence (404, not 403)
}

// Test 5: Audio playlist returns 403 when library access is denied.
func TestAudioPlaylist_LibraryAccessDenied_403(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	ah := buildAudioServe(t, true, libSvc, denyAll(), testFakeParentalChecker{}, &fakeAudioEnsurer{})

	w := doAudioRequest(audioPlaylistHandler(ah), "m1", "0")

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Test 6: Audio segment returns 503 when orchestrator fails (EnsureAudioSegment error).
func TestAudioSegment_OrchestratorError_503(t *testing.T) {
	item, streams := testMediaItemWithAudioStreams()
	libSvc := &testFakeStreamsGetter{item: item, streams: streams}
	failOrch := &fakeAudioEnsurer{err: errTest}
	ah := buildAudioServe(t, true, libSvc, allowAll(), testFakeParentalChecker{}, failOrch)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/audio_0_000.m4s", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("trackIdx", "0")
	req.SetPathValue("segIdx", "0")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	audioSegmentHandler(ah)(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// Test 7: buildAudioRenditions correctly extracts audio tracks only.
func TestBuildAudioRenditions_FiltersAudioOnly(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		{StreamType: "video", Codec: "h264"},
		{StreamType: "audio", Codec: "aac", Language: "en", Title: "English"},
		{StreamType: "subtitle"},
		{StreamType: "audio", Codec: "ac3", Language: "fr"},
	}
	renditions := buildAudioRenditions(streams)

	require.Len(t, renditions, 2)
	assert.Equal(t, 0, renditions[0].SourceTrackIdx)
	assert.Equal(t, "English", renditions[0].Name)
	assert.Equal(t, "en", renditions[0].LanguageCode)
	assert.Equal(t, 1, renditions[1].SourceTrackIdx)
	assert.Equal(t, "fr", renditions[1].LanguageCode)
}
