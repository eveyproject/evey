package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
)

// countingPrefsGetter counts how many times the profile preferences are
// fetched - each call is a Postgres round-trip on the segment hot path.
type countingPrefsGetter struct{ calls atomic.Int32 }

func (c *countingPrefsGetter) GetProfilePreferences(_ context.Context, _ string) (map[string]string, error) {
	c.calls.Add(1)
	return map[string]string{}, nil
}

// A segment request must resolve its playback plan ONCE: decision, ladder,
// fingerprint and producer all read the same resolution, and the profile
// preferences behind them are fetched a single time. Before the plan
// refactor the segment path re-derived the decision up to three times
// (bound check + fingerprint + producer pick), each pulling the prefs from
// Postgres again - ~7 fetches per segment, every few seconds, per viewer.
func TestServeSegment_LoadsProfilePrefsOnce(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})
	prefs := &countingPrefsGetter{}
	h.profilePrefs = prefs

	w := doVODRequest(h, "m1", "stream_0_000.m4s")
	// Cache miss → 503 after the wait budget; the request still ran the whole
	// plan resolution + producer path, which is what we count.
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	assert.Equal(t, int32(1), prefs.calls.Load(),
		"one segment request must load the profile prefs exactly once")
}

// The manifest path too: one master request = one prefs fetch. The old
// buildManifestInput derived the decision twice (once directly, once inside
// the sealed fingerprint hash), doubling every input fetch.
func TestServeMaster_LoadsProfilePrefsOnce(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})
	prefs := &countingPrefsGetter{}
	h.profilePrefs = prefs

	w := doVODRequest(h, "m1", "master.m3u8")
	require.Equal(t, http.StatusOK, w.Code)

	assert.Equal(t, int32(1), prefs.calls.Load(),
		"one master request must load the profile prefs exactly once")
}

// Across a playback burst the prefs are cached too: N segment requests from
// the same profile inside the TTL window fetch the profile prefs ONCE, not
// once per request - the last per-segment Postgres round-trip gone.
func TestSegmentBurst_LoadsProfilePrefsOnceAcrossRequests(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})
	prefs := &countingPrefsGetter{}
	h.profilePrefs = prefs
	prefsCache, _ := pkgcache.NewWithMaxSize[apptranscoding.TranscodingPreferences](0, mediaLookupMax)
	h.prefsLookup = prefsCache

	for seg := range 3 {
		w := doVODRequest(h, "m1", "stream_0_00"+strconv.Itoa(seg)+".m4s")
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
	}

	assert.Equal(t, int32(1), prefs.calls.Load(),
		"a burst of segment requests must load the profile prefs once, not once per request")
}

// resolvePlan's app-layer projection (apptranscoding.PlaybackPlan) must be
// coherent with the fingerprint the very same resolution produced - it is
// the object a future app-layer resolver will return, so its fields cannot
// drift from what the adapter actually serves.
func TestResolvePlan_PopulatesPlaybackPlan(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
	}
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item}, allowAll(), testFakeParentalChecker{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0_000.m4s", nil)
	req = withHTTPClaims(req, testClaims(false))

	rp, err := h.resolvePlan(req, "m1", item, nil)
	require.NoError(t, err)

	assert.Equal(t, rp.FP.Hash(), rp.FPHash, "cached hash must be the fingerprint's")
	assert.Equal(t, rp.FPHash, rp.Plan.FingerprintHash)
	assert.Equal(t, rp.Decision, rp.Plan.Decision)
	assert.Equal(t, rp.Profiles, rp.Plan.Ladder)
	assert.Equal(t, rp.FP.VideoCodec, rp.Plan.VideoCodec)
	assert.Equal(t, rp.FP.HWAccel, rp.Plan.HWAccel)
	assert.Equal(t, apptranscoding.LadderClassOf(rp.FP), rp.Plan.Class)
	assert.False(t, rp.IsCopy)
	assert.Equal(t, apptranscoding.PackagerTranscodeFFmpeg, rp.Plan.Packager,
		"a transcode resolution bills the ffmpeg transcode packager")
	assert.True(t, rp.Plan.NeedsVideoEncode(), "a transcode plan spends encode")
}

// A copy-eligible request resolves to the ffmpeg-copy packager and is
// flagged IsCopy - the routing bit every consumer (bound check, shed,
// producer pick) reads instead of re-deriving.
func TestResolvePlan_CopyRequest(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur, ContainerFormat: "matroska"},
	}
	streams := copyEligibleStreams()
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	copyProd := apptranscoding.NewSoftwareProducer(h.cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil), nil)
	h.copyProducer = copyProd

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/stream_0_000.m4s", nil)
	req = withHTTPClaims(req, testClaims(false))

	rp, err := h.resolvePlan(req, "m1", item, streams)
	require.NoError(t, err)

	assert.True(t, rp.IsCopy)
	assert.Equal(t, apptranscoding.PackagerCopyFFmpeg, rp.Plan.Packager)
	assert.False(t, rp.Plan.NeedsVideoEncode(), "a copy plan never spends encode")
	assert.True(t, rp.Plan.UsesFFmpeg(), "the ffmpeg copy rung still spawns a process")
}

type stubUserBitrate struct{ kbps int }

func (s stubUserBitrate) GetUserBitrateLimit(context.Context, string) (int, error) {
	return s.kbps, nil
}

// The remote ceiling bites only on a public client; the admin per-user limit
// always does; the tightest of the two wins, and no signal means no cap.
func TestRequestBitrateCapKbps(t *testing.T) {
	remote := func() int { return 20000 }
	lan := httptest.NewRequest("GET", "/hls/master.m3u8", nil)
	lan.RemoteAddr = "192.168.1.20:5000"
	public := httptest.NewRequest("GET", "/hls/master.m3u8", nil)
	public.RemoteAddr = "203.0.113.9:5000"

	h := &hlsVODServe{maxVideoBitrateRemoteKbps: remote}
	assert.Equal(t, 0, h.requestBitrateCapKbps(lan))
	assert.Equal(t, 20000, h.requestBitrateCapKbps(public))
	unknown := httptest.NewRequest("GET", "/hls/master.m3u8", nil)
	unknown.RemoteAddr = ""
	assert.Equal(t, 0, h.requestBitrateCapKbps(unknown), "no signal at all: an unknown address counts as LAN")

	withUser := withHTTPClaims(public, &domainuser.Claims{UserID: "u1", ProfileID: "p1"})
	h.userBitrate = stubUserBitrate{kbps: 8000}
	assert.Equal(t, 8000, h.requestBitrateCapKbps(withUser), "the per-user limit is the tightest")
	h.userBitrate = stubUserBitrate{kbps: 50000}
	assert.Equal(t, 20000, h.requestBitrateCapKbps(withUser), "the remote ceiling is the tightest")
	assert.Equal(t, 50000, h.requestBitrateCapKbps(withHTTPClaims(lan, &domainuser.Claims{UserID: "u1"})), "on the LAN only the user limit applies")
}
