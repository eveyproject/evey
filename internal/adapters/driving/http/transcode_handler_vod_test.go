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
// so EnsureSegmentRange really calls through (no ffmpeg spawned - the fake
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
		// Wired like production so the supersede path is the one under test,
		// not a nil-guard; reclaimHash stays nil (no producer to tear down)
		// until a test injects one.
		streamHashes: apptranscoding.NewStreamHashRegistry(0, 0),
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
	// 404'd by the duration clamp before any spawn - see the OutOfRange test).
	w := doVODRequest(h, "m1", "stream_0_005.ts")
	// seg=5, isInit=false - reaches serveSegment, EnsureSegmentRange, then Wait
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "cache miss should 503 after timeout")
}

// A forged deep segment index (past the media duration) is rejected with 404
// BEFORE EnsureSegmentRange spawns an ffmpeg -ss far past the end - the duration
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

// Regression (prod: a series episode, a beta tester stalled at 46:43): a COPY rung
// cuts on source keyframes, which are routinely SHORTER than the transcode grid,
// so it lists MORE segments than ceil(dur/grid). The forged-index guard used a
// grid bound and false-404'd every copy segment past ceil(dur/grid) - the tail of
// the file. The guard must SKIP copy requests; they range-check a forged index
// cheaply downstream via the plan's len(segs), with no ffmpeg -ss spawn.
func TestHLSVOD_Segment_CopyRung_DeepIndexNotFalse404(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000) // 60s → ceil(dur/3s) = 20 transcode segments
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File: &domainlibrary.MediaFile{
			Path:            "/test.mkv",
			ContainerFormat: "matroska",
			Duration:        dur,
			// Size zero → the copy disk guard fails open → copy allowed.
		},
	}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: item, streams: copyEligibleStreams()},
		allowAll(),
		testFakeParentalChecker{},
	)
	// Wire the copy path so isCopyRequest can return true (the guard's skip gate).
	copyProd := apptranscoding.NewSoftwareProducer(h.cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil), nil)
	h.copyProducer = copyProd
	h.segmentWaitTimeout = 50 * time.Millisecond

	// Deep index far past the 20-segment transcode grid - a legitimate copy tail
	// segment. Old code: 404 "segment index out of range". Fixed: guard skipped, so
	// the request reaches the producer and 503s on the (empty) cache instead.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/media/m1/hls/v2/stream_0_999.m4s?videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4,fmp4", nil)
	req.SetPathValue("id", "m1")
	req.SetPathValue("file", "stream_0_999.m4s")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.serveHTTP(w, req)

	assert.NotContains(t, w.Body.String(), "segment index out of range",
		"copy-rung deep index must not be false-404'd by the transcode grid guard")
	assert.NotEqual(t, http.StatusNotFound, w.Code,
		"copy-rung deep index reaches the producer (503 on cache miss), not the out-of-range 404")
}

// A forged variant index (past the rung count) is rejected with 404 before
// EnsureSegmentRange, so the HW orchestrator never spawns a rafale that would
// write stream_99_* (a wasted session + full WaitBudget for the client).
func TestHLSVOD_Segment_VariantOutOfRange_404(t *testing.T) {
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
	w := doVODRequest(h, "m1", "stream_99_000.ts") // variant 99 >> rung count
	assert.Equal(t, http.StatusNotFound, w.Code, "out-of-range variant must 404, not spawn+503")
}

// At the shared video-encode session cap, EnsureSegmentRange returns
// ErrVideoCapacityExhausted and the handler must shed load with 503 +
// Retry-After: 5 + a distinct body, not the generic capacity/500 branch.
func TestHLSVOD_Segment_VideoCapacityExhausted_503WithRetryAfter5(t *testing.T) {
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

	limiter := apptranscoding.NewVideoSessionLimiter(1)
	require.True(t, limiter.TryAcquire("some-other-stream", apptranscoding.LadderClassSoftware, 1080), "pre-occupy the single video-encode slot with a DIFFERENT stream")
	h.orchestrator.WithVideoSessionLimiter(limiter)

	w := doVODRequest(h, "m1", "stream_0_005.ts")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "5", w.Header().Get("Retry-After"))
	// The clients key their plain-language wording on this code, never on the
	// English message: a viewer must read "too many things playing at once",
	// not "transcoding capacity".
	assert.Equal(t, ErrCodeTooManyStreams, w.Header().Get("X-Evey-Error"))
	assert.Contains(t, w.Body.String(), `"code":"too_many_streams"`)
	assert.Contains(t, w.Body.String(), `"retryAfterSec":5`)
}

// The precision the shed path owes its users: a source that only lacks its
// keyframe index would be served by the ZERO-COST copy rung (no encode slot at
// all) as soon as the index lands. Such a viewer is competing with nobody, so
// telling them "too many streams" is simply false. They get file_not_ready,
// and the index walk is nudged so their retry succeeds.
func TestHLSVOD_Segment_CopyEligibleSourceNeverBlamesOtherViewers(t *testing.T) {
	w, hgt, level := 1920, 1080, 40
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur, ContainerFormat: "matroska"},
	}
	// H.264 High@L4.0, copy-eligible in every respect EXCEPT the missing index.
	streams := []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo,
		Codec:      "h264",
		Profile:    "High",
		Width:      &w,
		Height:     &hgt,
		Level:      &level,
		Keyframes:  nil,
	}}

	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: item, streams: streams},
		allowAll(),
		testFakeParentalChecker{},
	)
	h.segmentWaitTimeout = 50 * time.Millisecond

	warmed := make(chan string, 1)
	h.keyframeWarmer = func(_ context.Context, mediaID, _ string) bool {
		select {
		case warmed <- mediaID:
		default:
		}
		return false
	}

	limiter := apptranscoding.NewVideoSessionLimiter(1)
	require.True(t, limiter.TryAcquire("some-other-stream", apptranscoding.LadderClassSoftware, 1080), "someone else holds the only slot")
	h.orchestrator.WithVideoSessionLimiter(limiter)

	resp := doVODRequest(h, "m1", "stream_0_005.ts")

	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
	assert.Equal(t, ErrCodeFileNotReady, resp.Header().Get("X-Evey-Error"),
		"a copy-eligible source must never be told the host is full")
	assert.NotContains(t, resp.Body.String(), ErrCodeTooManyStreams)
	assert.Equal(t, "10", resp.Header().Get("Retry-After"))

	select {
	case id := <-warmed:
		assert.Equal(t, "m1", id, "the index walk must be nudged so the retry succeeds")
	case <-time.After(2 * time.Second):
		t.Fatal("the shed path must nudge the keyframe index walk")
	}
}

// A native player (mobile, TV) never exposes a segment's status code, so a
// host at capacity would surface as a generic "playback failed". Shedding the
// VARIANT PLAYLIST instead tells every client, before it commits to the
// stream, exactly what is happening.
func TestHLSVOD_VariantPlaylist_ShedsEarlyWhenHostIsFull(t *testing.T) {
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

	// The probe refuses everything: stand in for a host with no budget left.
	h.videoAdmission = func(string, apptranscoding.DecisionFingerprint, int) bool { return false }

	w := doVODRequest(h, "m1", "stream_0.m3u8")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, ErrCodeTooManyStreams, w.Header().Get("X-Evey-Error"))
	assert.Equal(t, "5", w.Header().Get("Retry-After"))
	assert.NotContains(t, w.Body.String(), "#EXTM3U", "a shed request must not hand out a playlist")
}

func TestHLSVOD_VariantPlaylist_ServedNormallyWhenAdmitted(t *testing.T) {
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
	h.videoAdmission = func(string, apptranscoding.DecisionFingerprint, int) bool { return true }

	w := doVODRequest(h, "m1", "stream_0.m3u8")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "#EXTM3U")
}

// Prod 2026-09-13 21:53: a host WITH route evidence turned every master away
// ("host at video capacity" on an idle server). The shed probe asked the REAL
// limiter with a zero route, which the route model refused as "uncovered". The
// probe must price the resolved route exactly as the spawn does, through a
// limiter that carries route evidence.
func TestHLSVOD_Master_AdmittedByRealLimiterWithRouteEvidence(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{},
	)
	// prod-host's evidence shape: the software route this test's plan resolves to
	// (libx264 sdr) plus a GPU route that must not matter here.
	samples := []domaintranscoding.RouteSample{
		{Key: domaintranscoding.RouteCostKey{Encoder: "libx264", Route: domaintranscoding.RouteCostSDR}, Height: 1080, FPS: 120.8, CPUCores: 5.06, SourceFPS: 24},
		{Key: domaintranscoding.RouteCostKey{Backend: "qsv", Encoder: "h264_qsv", Route: domaintranscoding.RouteCostSDR}, Height: 2160, FPS: 74.1, CPUCores: 0.21, SourceFPS: 24},
	}
	limiter := apptranscoding.NewVideoSessionLimiter(0).
		WithRouteModel(apptranscoding.NewRouteCostModel(samples, 7))
	h.videoAdmission = videoAdmissionOf(limiter)

	w := doVODRequest(h, "m1", "master.m3u8")

	require.Equal(t, http.StatusOK, w.Code, "an idle host with route evidence must serve the master, body: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), "#EXTM3U")
	assert.Empty(t, w.Header().Get("X-Evey-Error"))
}

// The master is what a native mobile/TV player fetches first (and all our
// preflight probe ever sees). It must shed too - but only when NO rung fits,
// never merely because the tallest one does not.
// The rung forcing feature, seen from the shared shaping head: when the
// admission shaper grants a ceiling below the ladder's top, BOTH the manifest
// a player receives and the fingerprint every segment request rebuilds must
// shape the same trimmed ladder - one drifting from the other is a permanent
// cache miss and wrong bytes under stable URLs.
func TestHLSVOD_ForcedCeiling_ShapesManifestAndFingerprintAlike(t *testing.T) {
	w, hgt := 3840, 2160
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	streams := []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo,
		Codec:      "hevc", // forces the transcode path (h264 client caps)
		Width:      &w,
		Height:     &hgt,
	}}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})

	var pinKeys []string
	var boundHashes []string
	h.admissionRoute = func(key string, req apptranscoding.RouteAdmission) apptranscoding.RouteGrant {
		pinKeys = append(pinKeys, key)
		// the host can only afford the short rung
		return apptranscoding.RouteGrant{Class: req.Natural, Ceiling: 720, OK: true}
	}
	h.admissionBind = func(_ string, fpHash string) { boundHashes = append(boundHashes, fpHash) }

	// Manifest side: the master must not advertise anything above the ceiling.
	master := doVODRequest(h, "m1", "master.m3u8")
	require.Equal(t, http.StatusOK, master.Code)
	assert.NotContains(t, master.Body.String(), "RESOLUTION=1920x1080",
		"the manifest must not promise a rung the granted ceiling excludes")

	// Segment side: the fingerprint shapes the SAME trimmed ladder...
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0_000.ts", nil)
	req = withHTTPClaims(req, testClaims(false))
	fp, err := h.buildFingerprint(req, "m1", item, streams)
	require.NoError(t, err)
	for _, p := range fp.Profiles {
		assert.LessOrEqual(t, p.Height, 720, "no fingerprint profile may exceed the pinned ceiling")
	}

	// ...deterministically: same request, same hash, every time.
	fp2, err := h.buildFingerprint(req, "m1", item, streams)
	require.NoError(t, err)
	assert.Equal(t, fp.Hash(), fp2.Hash())

	// And the shaper learned which session the pin shaped, under the same key
	// the ceiling was asked with.
	require.NotEmpty(t, boundHashes)
	assert.Equal(t, fp.Hash(), boundHashes[len(boundHashes)-1])
	for _, k := range pinKeys[1:] {
		assert.Equal(t, pinKeys[0], k, "manifest- and segment-time must derive the same pin key")
	}
}

// Copy requests never take a capacity slot, so they must never be trimmed.
func TestHLSVOD_ForcedCeiling_NeverTouchesACopyRequest(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur, ContainerFormat: "matroska"},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: copyEligibleStreams()}, allowAll(), testFakeParentalChecker{})
	// Wire the copy path: isCopyRequest only answers true with a producer set.
	copyProd := apptranscoding.NewSoftwareProducer(h.cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil), nil)
	h.copyProducer = copyProd
	h.admissionRoute = func(string, apptranscoding.RouteAdmission) apptranscoding.RouteGrant {
		t.Fatal("the shaper must not be consulted for a copy request")
		return apptranscoding.RouteGrant{}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0_000.ts", nil)
	req = withHTTPClaims(req, testClaims(false))
	_, err := h.buildFingerprint(req, "m1", item, copyEligibleStreams())
	require.NoError(t, err)
}

func TestHLSVOD_Master_ShedsWhenHostIsFull(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})

	var probedHeights []int
	h.videoAdmission = func(_ string, _ apptranscoding.DecisionFingerprint, topHeight int) bool {
		probedHeights = append(probedHeights, topHeight)
		return false
	}

	w := doVODRequest(h, "m1", "master.m3u8")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, ErrCodeTooManyStreams, w.Header().Get("X-Evey-Error"))
	assert.NotContains(t, w.Body.String(), "#EXTM3U")
	require.NotEmpty(t, probedHeights)
	// The master probes EVERY rung it advertises and sheds only when none
	// admits: with per-class cost tables (hw is not monotonic in height), no
	// single rung is safely "the cheapest" a priori.
	assert.Greater(t, len(probedHeights), 1,
		"a full host must have been probed at every advertised rung, not one")
}

// Direct play / copy / remux never depend on transcode capacity: a host whose
// admission refuses every encode (a weak ARM box at its one floor session, or
// no encode budget at all) still serves a copy-eligible title's master.
func TestHLSVOD_Master_CopyServedWithNoTranscodeCapacity(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur, ContainerFormat: "matroska"},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: copyEligibleStreams()}, allowAll(), testFakeParentalChecker{})
	copyProd := apptranscoding.NewSoftwareProducer(h.cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil), nil)
	h.copyProducer = copyProd
	// The weakest calibration there is, and an admission that refuses every encode.
	h.capability = func() domaintranscoding.HostCapabilitySnapshot { return (domaintranscoding.SafeFallback()).Snapshot() }
	h.videoAdmission = func(string, apptranscoding.DecisionFingerprint, int) bool {
		t.Fatal("a copy request must never consult the encode admission")
		return false
	}

	w := doVODRequest(h, "m1", "master.m3u8")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "#EXTM3U")
	assert.Empty(t, w.Header().Get("X-Evey-Error"))
}

// While the server benchmarks its own encode capacity, a NEW encoded playback
// is shed with the dedicated `calibrating` code (a ~2 min Retry-After) so the
// client tells the viewer to wait a moment - never "too many streams". A copy
// request takes no encode slot and is served as usual.
func TestHLSVOD_Master_ShedsAsCalibratingWhileBenchmarking(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})
	benchmarking := true
	h.calibrating = func() bool { return benchmarking }
	h.videoAdmission = func(string, apptranscoding.DecisionFingerprint, int) bool {
		t.Fatal("a calibrating shed answers before the admission probe")
		return false
	}

	w := doVODRequest(h, "m1", "master.m3u8")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, ErrCodeCalibrating, w.Header().Get("X-Evey-Error"))
	assert.Equal(t, "120", w.Header().Get("Retry-After"))
	assert.Contains(t, w.Body.String(), `"code":"calibrating"`)
	assert.NotContains(t, w.Body.String(), "#EXTM3U")

	// Bench over: served normally.
	benchmarking = false
	h.videoAdmission = nil
	w = doVODRequest(h, "m1", "master.m3u8")
	assert.Equal(t, http.StatusOK, w.Code)

	// A copy-eligible title is remuxed, never encoded: no shed during a bench.
	copyItem := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur, ContainerFormat: "matroska"},
	}
	hc, _ := buildVODHandler(t, &testFakeStreamsGetter{item: copyItem, streams: copyEligibleStreams()}, allowAll(), testFakeParentalChecker{})
	copyProd := apptranscoding.NewSoftwareProducer(hc.cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil), nil)
	hc.copyProducer = copyProd
	hc.calibrating = func() bool { return true }
	w = doVODRequest(hc, "m1", "master.m3u8")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-Evey-Error"))
}

// ladderClassFor (what admission prices) and LadderClassOf (what the spawn
// charges) must agree for every request shape, or one resource pool is billed
// while another is consumed. This locks the equivalence across the matrix.
func TestLadderClassFor_AgreesWithFingerprint(t *testing.T) {
	w, hgt := 3840, 2160
	dur, _ := shared.NewDurationMs(60_000)
	nvenc := domaintranscoding.EncodeCapability{
		HWBackend:      "nvenc",
		HWCodecs:       []string{"h264", "hevc"},
		HWDecodeCodecs: []string{"h264", "hevc", "hevc10"},
	}
	cases := []struct {
		name      string
		srcCodec  string
		hwEnabled bool
		query     string
	}{
		{"software h264", "h264", false, "?vd=transcode&ad=transcode"},
		{"hw h264", "h264", true, "?vd=transcode&ad=transcode"},
		{"hw hevc source", "hevc", true, "?vd=transcode&ad=transcode"},
		{"software hevc hdr-ish", "hevc", false, "?vd=transcode&ad=transcode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := &domainlibrary.MediaItem{
				ID:        "m1",
				LibraryID: "lib1",
				File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
			}
			streams := []*domainlibrary.MediaStream{{
				StreamType: domainlibrary.StreamTypeVideo,
				Codec:      tc.srcCodec,
				Width:      &w,
				Height:     &hgt,
			}}
			h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
			if tc.hwEnabled {
				h.capability = func() domaintranscoding.HostCapabilitySnapshot { return (nvenc).Snapshot() }
				h.hwAccelEnabled = func() bool { return true }
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0_000.ts"+tc.query, nil)
			req = withHTTPClaims(req, testClaims(false))

			rp, err := h.probePlan(req, "m1", item, streams)
			require.NoError(t, err)

			assert.Equal(t, apptranscoding.LadderClassOf(rp.FP), rp.Plan.Class,
				"admission must price the pool the spawn will charge: the plan's class and its fingerprint's agree")
		})
	}
}

// P5, end to end at the handler level: a hardware-natural stream whose GPU
// pool is full spills to the software route - the fingerprint says software
// (so the spawn bills the CPU pool), the ladder is the software producer's
// shape, and the spawn picks the software producer. All three must move
// together or the cache hash lies.
func TestHLSVOD_SpilledStream_FingerprintAndSpawnFollowTheSoftwareRoute(t *testing.T) {
	w, hgt := 3840, 2160
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	streams := []*domainlibrary.MediaStream{{
		StreamType: domainlibrary.StreamTypeVideo,
		Codec:      "h264",
		Width:      &w,
		Height:     &hgt,
	}}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	h.capability = func() domaintranscoding.HostCapabilitySnapshot {
		return domaintranscoding.EncodeCapability{
			HWBackend:      "nvenc",
			HWCodecs:       []string{"h264", "hevc"},
			HWDecodeCodecs: []string{"h264", "hevc", "hevc10"},
			HWMaxHeight:    2160,
		}.Snapshot()
	}
	h.hwAccelEnabled = func() bool { return true }
	// A real selector with sentinel producers, gated by the handler's real
	// resolveHW (same wiring as production): this stream routes hw naturally.
	swProd := &sentinelProducer{tag: "software"}
	hwProd := &sentinelProducer{tag: "hardware"}
	h.selector = apptranscoding.NewProducerSelector(swProd, hwProd)

	// The shaper says: GPU full, spilled to software, capped at 1080.
	h.admissionRoute = func(_ string, req apptranscoding.RouteAdmission) apptranscoding.RouteGrant {
		natural, fallback := req.Natural, req.FallbackHeights
		require.Equal(t, apptranscoding.LadderClassHW, natural, "this stream is hardware-natural")
		require.NotNil(t, fallback, "a hw-natural stream must offer the software fallback")
		require.NotEmpty(t, fallback(), "the software ladder must be shapeable")
		return apptranscoding.RouteGrant{Class: apptranscoding.LadderClassSoftware, Ceiling: 1080, OK: true}
	}
	h.admissionPinnedClass = func(string) (apptranscoding.LadderClass, bool) {
		return apptranscoding.LadderClassSoftware, true
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0_000.ts", nil)
	req = withHTTPClaims(req, testClaims(false))

	fp, err := h.buildFingerprint(req, "m1", item, streams)
	require.NoError(t, err)

	assert.Equal(t, apptranscoding.LadderClassSoftware, apptranscoding.LadderClassOf(fp),
		"the spilled fingerprint bills the CPU pool")
	assert.Empty(t, fp.HWAccel)
	for _, p := range fp.Profiles {
		assert.LessOrEqual(t, p.Height, 1080, "the granted ceiling caps the spilled ladder")
	}

	// Deterministic: same request, same hash.
	fp2, err := h.buildFingerprint(req, "m1", item, streams)
	require.NoError(t, err)
	assert.Equal(t, fp.Hash(), fp2.Hash())

	// The natural route was hardware (the admission stub above asserted
	// req.Natural == LadderClassHW) and the plan records the spill as such...
	rp, err := h.probePlan(req, "m1", item, streams)
	require.NoError(t, err)
	assert.Equal(t, apptranscoding.SpillPoolFull, rp.Plan.Route.Spill, "precondition: without the spill, this stream rides the GPU")

	// ...and that the spawn nevertheless goes to the software producer.
	prod := h.producerForRequest(req.Context(), req, streams, item)
	require.NotNil(t, prod)
	assert.Same(t, apptranscoding.StreamProducer(swProd), prod, "the spawn must ride the software route the fp describes")
}

func TestHLSVOD_Master_ServedWhenAdmitted(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})
	h.videoAdmission = func(string, apptranscoding.DecisionFingerprint, int) bool { return true }

	w := doVODRequest(h, "m1", "master.m3u8")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "#EXTM3U")
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
	// isInit=true - also reaches serveSegment → EnsureSegmentRange(0) → Wait → 503
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
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHLSVOD_ParentalBlocked_NotFound(t *testing.T) {
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute()},
		allowAll(),
		testFakeParentalChecker{block: true},
	)
	w := doVODRequest(h, "m1", "master.m3u8")
	assert.Equal(t, http.StatusNotFound, w.Code) // parental block hides existence (404, not 403)
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
	// has no complexity_class persisted - which is the case for the test
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
	// fMP4 is now the project default - the handler requests stream_0_000.m4s,
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
//     CalibrationService is wired - matches the handler's default)
//  4. BoostCappedTopRung - the 720p rung inherits the dropped 1080p
//     rung's bitrate budget (part of the cache fingerprint)
//
// If a test wires CapabilityService with a different cap, that test
// should build its own profile slice - this helper is the
// uncalibrated/default-path mirror.
func profilesForTest() []apptranscoding.ProfileFingerprint {
	src := domaintranscoding.LadderFor(domaintranscoding.ClassFilmNormal)
	capped := domaintranscoding.CapLadderToHeight(src, 720)
	src = domaintranscoding.BoostCappedTopRung(src, capped)
	// Levelled as the plan levels them: software route, rate unknown.
	src = domaintranscoding.AssignCodecLevels(src, "h264", 0, func(p domaintranscoding.HLSProfile) domaintranscoding.QualityTarget {
		return domaintranscoding.StaticQualityPolicy{}.TargetFor(p, domaintranscoding.EncodeRoute{})
	}, 0)
	out := make([]apptranscoding.ProfileFingerprint, len(src))
	for i, p := range src {
		out[i] = apptranscoding.ProfileFingerprint{
			Name:         p.Name,
			Width:        p.Width,
			Height:       p.Height,
			VideoBitrate: p.VideoBitrate,
			AudioBitrate: p.AudioBitrate,
			CodecLevel:   p.CodecLevel,
		}
	}
	return out
}

// ----------------------------------------------------------------
// Prewarm-next handler - security regressions from the audit.
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

// blockMediaParental blocks the specified media item ids only - used to
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
	// Caller has access ONLY to lib-a - same path the player would take
	// to start watching ep-a - but lib-b must remain off-limits.
	access := &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}
	h, _ := buildVODHandler(t, libSvc, access, testFakeParentalChecker{})

	w := doPrewarmRequest(h, "ep-a")
	assert.Equal(t, http.StatusNotFound, w.Code,
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
	// Block only the next episode - current ep stays viewable.
	parental := &blockMediaParental{blocked: map[string]bool{"ep-b": true}}
	h, _ := buildVODHandler(t, libSvc, access, parental)

	w := doPrewarmRequest(h, "ep-a")
	assert.Equal(t, http.StatusNotFound, w.Code,
		"prewarm-next must reject (404, existence hidden) when nextItem is parental-restricted for this profile")
}

// The mirror case: the CURRENT episode is the one hidden. A kid blocked on it
// must not use prewarm-next as a pivot to reach the next one, which is viewable.
func TestPrewarmNext_CurrentItemParentalBlocked_NotFound(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	next := newEpisode("ep-b", "lib-a")
	libSvc := &prewarmStreamsGetter{
		items: map[string]*domainlibrary.MediaItem{"ep-a": cur, "ep-b": next},
		next:  next,
	}
	access := &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}
	parental := &blockMediaParental{blocked: map[string]bool{"ep-a": true}}
	h, _ := buildVODHandler(t, libSvc, access, parental)

	w := doPrewarmRequest(h, "ep-a")
	assert.Equal(t, http.StatusNotFound, w.Code,
		"prewarm-next must reject (404, existence hidden) when the current item is parental-restricted")
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
	assert.Equal(t, http.StatusNotFound, w.Code)
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

// The route patterns already match `\d+`, so the only way this parse fails is
// OVERFLOW - stream_99999999999999999999.m3u8. Every call site used to discard
// strconv's range error, and Atoi returns 0 alongside it, so an overflowing
// index silently became index 0: the bounds check passed and the server served
// rendition 0 instead of answering 404.
func TestAtoiIndex(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"0", 0},
		{"7", 7},
		// Overflows int64 - must NOT collapse to 0.
		{"99999999999999999999", -1},
		{"", -1},
		{"abc", -1},
	}
	for _, tc := range cases {
		if got := atoiIndex(tc.in); got != tc.want {
			t.Errorf("atoiIndex(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// The master's CODECS must describe the bytes the spawn will make. It used to
// re-derive the codec (EffectiveVideoCodec) instead of reading the resolved
// fingerprint's, so a hardware-natural HEVC stream spilled to the software
// route (GPU pool full / title blocked) advertised hvc1 over x264 avc1 bytes.
func TestHLSVOD_Master_CodecsFollowTheSpilledFingerprint(t *testing.T) {
	w, hgt := 3840, 2160
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur}}
	streams := []*domainlibrary.MediaStream{{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Width: &w, Height: &hgt}}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	h.capability = func() domaintranscoding.HostCapabilitySnapshot {
		return domaintranscoding.EncodeCapability{
			HWBackend: "nvenc", HWCodecs: []string{"h264", "hevc"}, HWDecodeCodecs: []string{"h264", "hevc", "hevc10"}, HWMaxHeight: 2160,
			ProbeVersion: domaintranscoding.CapabilityProbeVersion,
			HWRoutes:     []string{domaintranscoding.HWRouteSDR, domaintranscoding.HWRouteHEVC},
		}.Snapshot()
	}
	h.hwAccelEnabled = func() bool { return true }
	h.videoAdmission = func(string, apptranscoding.DecisionFingerprint, int) bool { return true }
	h.selector = apptranscoding.NewProducerSelector(&sentinelProducer{tag: "software"}, &sentinelProducer{tag: "hardware"})

	master := func() string {
		// vd=transcode: a copy-blocked title keeps a "copy" decision under a
		// transcode ladder and never takes the HEVC upgrade; the pin is how a
		// genuine transcode (or the copy master's fallback rung) resolves.
		rec := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4&vd=transcode")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return rec.Body.String()
	}
	assert.Contains(t, master(), `CODECS="hvc1.`, "premise: on the GPU this HEVC-capable client's ladder is HEVC")

	h.admissionRoute = func(_ string, _ apptranscoding.RouteAdmission) apptranscoding.RouteGrant {
		return apptranscoding.RouteGrant{Class: apptranscoding.LadderClassSoftware, Ceiling: 1080, OK: true}
	}
	h.admissionPinnedClass = func(string) (apptranscoding.LadderClass, bool) { return apptranscoding.LadderClassSoftware, true }
	spilled := master()
	assert.NotContains(t, spilled, `CODECS="hvc1.`, "spilled to x264, the master must not promise HEVC: %s", spilled)
	assert.Contains(t, spilled, `CODECS="avc1.`, "the master advertises the H.264 the software route encodes")
}
