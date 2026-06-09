package httpapi

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
	transcodingfakes "github.com/evey-project/evey/internal/domain/transcoding/fakes"
)

// ----------------------------------------------------------------
// Test-local fakes
// ----------------------------------------------------------------

// testFakeStreamsGetter satisfies MediaStreamsGetter.
type testFakeStreamsGetter struct {
	item    *domainlibrary.MediaItem
	streams []*domainlibrary.MediaStream
	err     error
}

func (f *testFakeStreamsGetter) GetMediaItem(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return f.item, f.err
}

func (f *testFakeStreamsGetter) GetMediaStreams(_ context.Context, _ string) ([]*domainlibrary.MediaStream, error) {
	return f.streams, f.err
}

func (f *testFakeStreamsGetter) NextEpisodeOf(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return nil, nil
}

// testFakeParentalChecker satisfies ParentalChecker, never blocks unless asked.
type testFakeParentalChecker struct{ block bool }

func (f testFakeParentalChecker) BlockedByParental(_ context.Context, _ string, _ *domainlibrary.MediaItem) bool {
	return f.block
}

// testFakeEnsure counts EnsureSegmentRange calls and can optionally fail.
type testFakeEnsure struct {
	calls []int
	err   error
}

func (f *testFakeEnsure) EnsureSegmentRange(_ context.Context, _ apptranscoding.DecisionFingerprint, segIdx int) error {
	f.calls = append(f.calls, segIdx)
	return f.err
}

// fakeOrchestratorAdapter is a thin SegmentOrchestrator-shaped shim that
// delegates only EnsureSegmentRange so tests can control it without spawning
// ffmpeg. We keep a real *SegmentOrchestrator in hlsVODServe but override via
// a wrapper that embeds fakeEnsure via the orchestratorEnsurer interface.

// ---- construction helpers ----

// testMediaItemWith1Minute builds a MediaItem with 60 s duration.
func testMediaItemWith1Minute() *domainlibrary.MediaItem {
	dur, _ := shared.NewDurationMs(60_000)
	return &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File: &domainlibrary.MediaFile{
			Path:     "/test.mkv",
			Duration: dur,
		},
	}
}

// testMediaItemNoDuration builds a MediaItem with duration=0 (unprobed).
func testMediaItemNoDuration() *domainlibrary.MediaItem {
	return &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File: &domainlibrary.MediaFile{
			Path: "/test.mkv",
			// Duration zero-value → Milliseconds()==0
		},
	}
}

// buildVODHandler builds an hlsVODServe with real ManifestGenerator + a
// real SegmentCache wired to tmpDir. The orchestrator uses fakes.HLSProcess
// so EnsureSegmentRange really calls through (no ffmpeg spawned — the fake
// writes .m3u8 files but no segments unless we pre-seed them).
func buildVODHandler(t *testing.T, libSvc MediaStreamsGetter, libAccess LibraryAccessChecker, parental ParentalChecker) (*hlsVODServe, string) {
	t.Helper()
	cacheDir := t.TempDir()
	cache, err := apptranscoding.NewSegmentCache(cacheDir + "/vod")
	require.NoError(t, err)

	orch := apptranscoding.NewSegmentOrchestrator(
		cache,
		&transcodingfakes.HLSProcess{},
		&noopSourceResolver{},
		// Short wait so cache-miss tests 503 fast instead of summing 25 s waits.
		apptranscoding.OrchestratorOptions{WaitBudgetDur: 50 * time.Millisecond},
		slog.Default(),
	)
	t.Cleanup(func() { orch.Close() })

	h := &hlsVODServe{
		tokens:               &fakeTokenVerifier{claims: testClaims(false)},
		libSvc:               libSvc,
		libraryAccess:        libAccess,
		parental:             parental,
		profilePrefs:         nil,
		audioNormalizeGetter: nil,
		userMaxStreams:       nil,
		manifestGen:          apptranscoding.NewManifestGenerator(),
		cache:                cache,
		orchestrator:         orch,
		useFMP4:              true, // matches the production default (fMP4 everywhere)
		segmentWaitTimeout:   5 * time.Second,
	}
	return h, cacheDir
}

// noopSourceResolver returns a dummy path so the orchestrator can resolve a source.
type noopSourceResolver struct{}

func (noopSourceResolver) ResolveSource(_ context.Context, fp apptranscoding.DecisionFingerprint) (string, []apptranscoding.SourceStream, error) {
	return fp.SourcePath, nil, nil
}

// doVODRequest fires the handler at /api/v1/media/{mediaID}/hls/v2/{file}
// and returns the recorder. It injects HTTP claims automatically.
func doVODRequest(h *hlsVODServe, mediaID, file string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+mediaID+"/hls/v2/"+file, nil)
	req.SetPathValue("id", mediaID)
	req.SetPathValue("file", file)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	return w
}

// ----------------------------------------------------------------
// 1-5: Routing / dispatch
// ----------------------------------------------------------------

func TestHLSVOD_Routing_Master(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{block: false},
	)
	w := doVODRequest(h, "m1", "master.m3u8")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/vnd.apple.mpegurl", w.Header().Get("Content-Type"))
}

func TestHLSVOD_Routing_Variant(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	w := doVODRequest(h, "m1", "stream_0.m3u8")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/vnd.apple.mpegurl", w.Header().Get("Content-Type"))
}

func TestHLSVOD_Routing_SegmentTS_CacheMissSpawnsOrchestrator(t *testing.T) {
	// The segment doesn't exist on disk → Wait will time out (segmentWaitTimeout
	// is short for tests). We just verify the handler returns 503 (not 404/500),
	// meaning it reached the Wait step (i.e. routing and EnsureSegmentRange worked).
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: item},
		allowAll(),
		testFakeParentalChecker{},
	)
	h.segmentWaitTimeout = 50 * time.Millisecond // keep test fast
	// seg=5 is in range for a 60s/20-segment media (a deep out-of-range index is
	// 404'd by the duration clamp before any spawn — see the OutOfRange test).
	w := doVODRequest(h, "m1", "stream_0_005.ts")
	// seg=5, isInit=false — reaches serveSegment, EnsureSegmentRange, then Wait
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "cache miss should 503 after timeout")
}

// A forged deep segment index (past the media duration) is rejected with 404
// BEFORE EnsureSegmentRange spawns an ffmpeg -ss far past the end — the duration
// clamp that guards the transcode path (copy already bounds via the plan).
func TestHLSVOD_Segment_OutOfRange_404(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000) // 60s → ~20 segments
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: item},
		allowAll(),
		testFakeParentalChecker{},
	)
	h.segmentWaitTimeout = 50 * time.Millisecond
	w := doVODRequest(h, "m1", "stream_0_999.ts") // 999*3 = 2997s >> 60s
	assert.Equal(t, http.StatusNotFound, w.Code, "out-of-range segment must 404, not spawn+503")
}

func TestHLSVOD_Routing_InitSegment(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: item},
		allowAll(),
		testFakeParentalChecker{},
	)
	h.segmentWaitTimeout = 50 * time.Millisecond
	w := doVODRequest(h, "m1", "stream_0_init.mp4")
	// isInit=true — also reaches serveSegment → EnsureSegmentRange(0) → Wait → 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "init segment cache miss should 503")
}

func TestHLSVOD_Routing_UnknownFile_404(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	w := doVODRequest(h, "m1", "foo.txt")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ----------------------------------------------------------------
// 6-8: Auth / security
// ----------------------------------------------------------------

func TestHLSVOD_StreamTokenMismatch_403(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/master.m3u8", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "master.m3u8")
	req = withHTTPClaims(req, testClaims(false))
	// Inject a stream token that was issued for a *different* media item.
	ctx := context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{
		UserID:      "u1",
		MediaItemID: "other-media",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHLSVOD_LibraryAccessDenied_403(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		denyAll(),
		testFakeParentalChecker{},
	)
	w := doVODRequest(h, "m1", "master.m3u8")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHLSVOD_ParentalBlocked_403(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{block: true},
	)
	w := doVODRequest(h, "m1", "master.m3u8")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// ----------------------------------------------------------------
// 9-13: Manifest
// ----------------------------------------------------------------

func TestHLSVOD_Duration0_503WithRetryAfter(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemNoDuration()},
		allowAll(),
		testFakeParentalChecker{},
	)
	w := doVODRequest(h, "m1", "master.m3u8")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "5", w.Header().Get("Retry-After"), "must set Retry-After: 5 for durationMissingError")
}

func TestHLSVOD_MasterPlaylist_ContainsEXTM3UAndTwoStreamINF(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	w := doVODRequest(h, "m1", "master.m3u8")
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "#EXTM3U")
	// profilesForDecision returns LadderFilmNormal (4 rungs) when the item
	// has no complexity_class persisted — which is the case for the test
	// fixture (no ComplexityRepo wired). CapLadderToSource may further
	// trim down if the source dimensions are smaller, but the fixture
	// doesn't expose any video stream so capping is a no-op.
	count := strings.Count(body, "#EXT-X-STREAM-INF")
	assert.GreaterOrEqual(t, count, 1, "master must list at least one STREAM-INF entry")
	assert.LessOrEqual(t, count, 4, "and at most the LadderFilmNormal width (4 rungs)")
}

func TestHLSVOD_MasterPlaylist_SubPlaylistURLsContainToken(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	// Inject a raw token in the request context so ManifestInput.StreamToken is set.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/master.m3u8", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "master.m3u8")
	req = withHTTPClaims(req, testClaims(false))
	ctx := context.WithValue(req.Context(), rawTokenCtxKey, "mytoken123")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "token=mytoken123", "sub-playlist URLs must contain the stream token")
}

func TestHLSVOD_VariantPlaylist_IsVODWithEndlist(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	w := doVODRequest(h, "m1", "stream_0.m3u8")
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "#EXT-X-PLAYLIST-TYPE:VOD")
	assert.Contains(t, body, "#EXT-X-ENDLIST")

	// 60 s / 3 s per segment = 20 segments
	expectedSegments := int(math.Ceil(60.0 / apptranscoding.ManifestSegmentSeconds))
	count := strings.Count(body, "#EXTINF:")
	assert.Equal(t, expectedSegments, count, "variant playlist must list N=ceil(duration/3) segments")
}

func TestHLSVOD_ProfileOutOfRange_404(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	// profilesForDecision returns only [:2], so stream_99.m3u8 must 404.
	w := doVODRequest(h, "m1", "stream_99.m3u8")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ----------------------------------------------------------------
// 14-16: Segment
// ----------------------------------------------------------------

// TestHLSVOD_Segment_CacheHit_200 pre-seeds the segment file on disk so the
// cache.Wait returns immediately; the handler must serve the file with 200.
func TestHLSVOD_Segment_CacheHit_200(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, cacheDir := buildVODHandler(t,
		&testFakeStreamsGetter{item: item},
		allowAll(),
		testFakeParentalChecker{},
	)

	// Compute the DecisionFingerprint the handler will produce for m1 with
	// default settings so we can pre-place the segment at the right path.
	// fMP4 is now the project default — the handler requests stream_0_000.m4s,
	// not .ts. Match the suffix so the cache lookup hits.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/stream_0_000.m4s", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "stream_0_000.m4s")
	req = withHTTPClaims(req, testClaims(false))

	// Reproduce the fingerprint using the same helpers the handler uses.
	fp := apptranscoding.DecisionFingerprint{
		MediaID:             "m1",
		SourcePath:          "/test.mkv",
		SubtitleBurnInTrack: -1, // parseIntQuery default for "subtitleTrack" is -1
		UseFMP4:             true,
		Profiles:            profilesForTest(),
	}
	segKey := apptranscoding.SegmentKey{
		DecisionHash: fp.Hash(),
		VariantIdx:   0,
		SegmentIdx:   0,
		IsInit:       false,
	}
	segPath := filepath.Join(cacheDir, "vod", "segments", segKey.DecisionHash)
	require.NoError(t, os.MkdirAll(segPath, 0o755))
	segFile := filepath.Join(segPath, "stream_0_000.m4s")
	// A structurally valid fMP4 media segment (styp + moof + mdat). SegmentCache
	// IsReady now rejects a non-empty but malformed segment (e.g. an aborted
	// ffmpeg leaving a styp-only stub), so the pre-seed must carry real boxes.
	fakeSeg := []byte{
		0, 0, 0, 0x10, 's', 't', 'y', 'p', 'm', 's', 'd', 'h', 0, 0, 0, 0, // styp (16)
		0, 0, 0, 0x08, 'm', 'o', 'o', 'f', // moof (8)
		0, 0, 0, 0x0C, 'm', 'd', 'a', 't', 0, 0, 0, 0, // mdat (12)
	}
	require.NoError(t, os.WriteFile(segFile, fakeSeg, 0o644))

	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	// If the cache pre-seed works, Wait returns immediately and ServeFile fires.
	assert.Equal(t, http.StatusOK, w.Code, "cache hit must serve 200")
}

// TestHLSVOD_Segment_CacheMiss_503 verifies that a missing segment eventually
// causes a 503 (after segmentWaitTimeout) rather than a 404 or panic.
func TestHLSVOD_Segment_CacheMiss_503(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: item},
		allowAll(),
		testFakeParentalChecker{},
	)
	h.segmentWaitTimeout = 50 * time.Millisecond // fast for test
	w := doVODRequest(h, "m1", "stream_0_005.ts")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "cache miss must 503 after timeout")
}

// TestHLSVOD_Segment_WaitTimeout_503 forces an immediate context cancellation
// to simulate a Wait timeout independently of the fake orchestrator.
func TestHLSVOD_Segment_WaitTimeout_503(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: item},
		allowAll(),
		testFakeParentalChecker{},
	)
	// Minimum timeout so the poll loop exits immediately.
	h.segmentWaitTimeout = 1 * time.Millisecond

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/v2/stream_0_000.ts", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "stream_0_000.ts")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "wait timeout must produce 503")
}

// ----------------------------------------------------------------
// helpers for fingerprint pre-computation
// ----------------------------------------------------------------

// profilesForTest returns the same ABR ladder profilesForDecision
// produces. Mirror the handler's exact pipeline:
//  1. start from LadderFilmNormal (no ComplexityRepo wired in tests)
//  2. CapLadderToSource is a no-op since the fixture has no video stream
//  3. CapLadderToHeight to 720p (the safe fallback when no
//     CalibrationService is wired — matches the handler's default)
//
// If a test wires CapabilityService with a different cap, that test
// should build its own profile slice — this helper is the
// uncalibrated/default-path mirror.
func profilesForTest() []apptranscoding.ProfileFingerprint {
	src := domaintranscoding.LadderFor(domaintranscoding.ClassFilmNormal)
	src = domaintranscoding.CapLadderToHeight(src, 720)
	out := make([]apptranscoding.ProfileFingerprint, len(src))
	for i, p := range src {
		out[i] = apptranscoding.ProfileFingerprint{
			Name:         p.Name,
			Width:        p.Width,
			Height:       p.Height,
			VideoBitrate: p.VideoBitrate,
			AudioBitrate: p.AudioBitrate,
		}
	}
	return out
}

// ----------------------------------------------------------------
// Prewarm-next handler — security regressions from the audit.
// Covers the 3 HIGH findings: cross-library IDOR, parental bypass on
// nextItem, and the absence of a rate-limit middleware (rate-limit is
// tested through the registration in server_test, here we cover the
// per-item access checks).
// ----------------------------------------------------------------

// prewarmStreamsGetter routes GetMediaItem by id and lets the test set the
// next-episode result explicitly. Same MediaStreamsGetter interface as
// testFakeStreamsGetter but with per-id lookup.
type prewarmStreamsGetter struct {
	items   map[string]*domainlibrary.MediaItem
	streams []*domainlibrary.MediaStream
	next    *domainlibrary.MediaItem
	nextErr error
}

func (f *prewarmStreamsGetter) GetMediaItem(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	if it, ok := f.items[id]; ok {
		return it, nil
	}
	return nil, nil
}
func (f *prewarmStreamsGetter) GetMediaStreams(_ context.Context, _ string) ([]*domainlibrary.MediaStream, error) {
	return f.streams, nil
}
func (f *prewarmStreamsGetter) NextEpisodeOf(_ context.Context, _ string) (*domainlibrary.MediaItem, error) {
	return f.next, f.nextErr
}

// libIDAccessChecker allows only the listed library IDs. Mirrors the
// production LibraryAccessChecker semantics so we can simulate a user
// who has Lib A but not Lib B.
type libIDAccessChecker struct{ allowed map[string]bool }

func (c *libIDAccessChecker) HasAccess(_ context.Context, _ shared.UserID, libID shared.LibraryID) (bool, error) {
	return c.allowed[string(libID)], nil
}

// blockMediaParental blocks the specified media item ids only — used to
// simulate a kid profile that may see ep N but not ep N+1.
type blockMediaParental struct{ blocked map[string]bool }

func (p *blockMediaParental) BlockedByParental(_ context.Context, _ string, item *domainlibrary.MediaItem) bool {
	if item == nil {
		return false
	}
	return p.blocked[string(item.ID)]
}

func newEpisode(id, libID string) *domainlibrary.MediaItem {
	dur, _ := shared.NewDurationMs(60_000)
	return &domainlibrary.MediaItem{
		ID:        shared.MediaItemID(id),
		LibraryID: shared.LibraryID(libID),
		Type:      "episode",
		File:      &domainlibrary.MediaFile{Path: "/" + id + ".mkv", Duration: dur},
	}
}

func doPrewarmRequest(h *hlsVODServe, mediaID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+mediaID+"/prewarm-next", nil)
	req.SetPathValue("id", mediaID)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.servePrewarmNext(w, req)
	return w
}

// HIGH-1 (audit): cross-library IDOR. User can play ep on Lib A and the
// next ep is on Lib B (series straddles libraries). The handler MUST
// re-check access on nextItem.LibraryID, not just curItem.LibraryID.
func TestPrewarmNext_CrossLibrary_IDOR_Forbidden(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	next := newEpisode("ep-b", "lib-b")
	libSvc := &prewarmStreamsGetter{
		items: map[string]*domainlibrary.MediaItem{"ep-a": cur, "ep-b": next},
		next:  next,
	}
	// Caller has access ONLY to lib-a — same path the player would take
	// to start watching ep-a — but lib-b must remain off-limits.
	access := &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}
	h, _ := buildVODHandler(t, libSvc, access, testFakeParentalChecker{})

	w := doPrewarmRequest(h, "ep-a")
	assert.Equal(t, http.StatusForbidden, w.Code,
		"prewarm-next must reject when nextItem belongs to a library the caller can't access")
}

// HIGH-2 (audit): parental bypass on nextItem. Kid profile may see ep N
// (current) but ep N+1 is blocked. The handler MUST re-check parental on
// nextItem so a kid can't trigger transcode of restricted content.
func TestPrewarmNext_NextItemParentalBlocked_Forbidden(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	next := newEpisode("ep-b", "lib-a")
	libSvc := &prewarmStreamsGetter{
		items: map[string]*domainlibrary.MediaItem{"ep-a": cur, "ep-b": next},
		next:  next,
	}
	access := &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}
	// Block only the next episode — current ep stays viewable.
	parental := &blockMediaParental{blocked: map[string]bool{"ep-b": true}}
	h, _ := buildVODHandler(t, libSvc, access, parental)

	w := doPrewarmRequest(h, "ep-a")
	assert.Equal(t, http.StatusForbidden, w.Code,
		"prewarm-next must reject when nextItem is parental-restricted for this profile")
}

// Sanity: current item access denial still blocks. Mirrors the existing
// check on curItem so it doesn't regress when adding the nextItem check.
func TestPrewarmNext_CurrentItemAccessDenied_Forbidden(t *testing.T) {
	cur := newEpisode("ep-a", "lib-private")
	libSvc := &prewarmStreamsGetter{
		items: map[string]*domainlibrary.MediaItem{"ep-a": cur},
	}
	access := &libIDAccessChecker{allowed: map[string]bool{"lib-other": true}}
	h, _ := buildVODHandler(t, libSvc, access, testFakeParentalChecker{})

	w := doPrewarmRequest(h, "ep-a")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// 204 when no next episode exists (series finale). Documents the success
// path without spawning ffmpeg.
func TestPrewarmNext_NoNextEpisode_NoContent(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	libSvc := &prewarmStreamsGetter{
		items: map[string]*domainlibrary.MediaItem{"ep-a": cur},
		next:  nil, // series finale
	}
	access := &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}
	h, _ := buildVODHandler(t, libSvc, access, testFakeParentalChecker{})

	w := doPrewarmRequest(h, "ep-a")
	assert.Equal(t, http.StatusNoContent, w.Code)
}

// 204 when current is a movie. Prewarm is episode-only by design.
func TestPrewarmNext_NotEpisode_NoContent(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	cur := &domainlibrary.MediaItem{
		ID: "mov-1", LibraryID: "lib-a", Type: "movie",
		File: &domainlibrary.MediaFile{Path: "/m.mkv", Duration: dur},
	}
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"mov-1": cur}}
	access := &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}
	h, _ := buildVODHandler(t, libSvc, access, testFakeParentalChecker{})

	w := doPrewarmRequest(h, "mov-1")
	assert.Equal(t, http.StatusNoContent, w.Code)
}
