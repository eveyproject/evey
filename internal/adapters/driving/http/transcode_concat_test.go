package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

// chainStreamsGetter chains NextEpisodeOf/GetMediaStreams by media ID - the
// shape resolveEpisodeChain actually walks (unlike prewarmStreamsGetter's
// single fixed "next", which can't represent a multi-hop chain).
type chainStreamsGetter struct {
	items       map[string]*domainlibrary.MediaItem
	streamsByID map[string][]*domainlibrary.MediaStream
	nextByID    map[string]*domainlibrary.MediaItem
}

func (f *chainStreamsGetter) GetMediaItem(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	return f.items[id], nil
}
func (f *chainStreamsGetter) GetMediaStreams(_ context.Context, id string) ([]*domainlibrary.MediaStream, error) {
	return f.streamsByID[id], nil
}
func (f *chainStreamsGetter) NextEpisodeOf(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	return f.nextByID[id], nil
}

func episode(id, seasonID string) *domainlibrary.MediaItem {
	return &domainlibrary.MediaItem{
		ID:       shared.MediaItemID(id),
		ParentID: seasonID,
		File:     &domainlibrary.MediaFile{Path: "/media/" + id + ".mkv", Size: 1 << 20},
	}
}

// concatReq mirrors onDemandReq's client-capability query params (copy
// eligibility needs them to resolve to "copy" rather than the safe-default
// "transcode") plus whatever extra is passed (?gapless=1, ...).
func concatReq(extra string) *http.Request {
	q := "?videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4"
	if extra != "" {
		q += "&" + strings.TrimPrefix(extra, "?")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/e1/hls/master.m3u8"+q, nil)
	return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
}

type fakeConcatPackager struct {
	plan       *fakeConcatPlan
	err        error
	calls      int
	lastSrcs   []string
	lastPrefix string
}

func (p *fakeConcatPackager) Plan(_ context.Context, sources []string, opts apptranscoding.ConcatOptions) (apptranscoding.ConcatPlan, error) {
	p.calls++
	p.lastSrcs = sources
	p.lastPrefix = opts.URLPrefix
	if p.err != nil {
		return nil, p.err
	}
	return p.plan, nil
}

type fakeConcatPlan struct{ resources map[string][]byte }

func (p *fakeConcatPlan) Resource(_ context.Context, name string) ([]byte, string, error) {
	if d, ok := p.resources[name]; ok {
		return d, "application/vnd.apple.mpegurl", nil
	}
	return nil, "", fmt.Errorf("fake: no resource %q", name)
}
func (p *fakeConcatPlan) Resources() []string { return []string{"master.m3u8"} }
func (p *fakeConcatPlan) NumParts() int       { return 2 }

// masterPlan is the plan serveMaster resolves for e1 and hands to serveConcatMaster.
func masterPlan(t *testing.T, h *hlsVODServe, e1 *domainlibrary.MediaItem) *requestPlan {
	t.Helper()
	rp, err := h.probePlan(concatReq("?gapless=1"), "e1", e1, copyEligibleAV())
	require.NoError(t, err)
	return rp
}

// --- serveConcatMaster: fall-through cases ---

func TestServeConcatMaster_FeatureOff(t *testing.T) {
	h := &hlsVODServe{} // onDemandConcat nil
	ok := h.serveConcatMaster(httptest.NewRecorder(), concatReq("?gapless=1"), "e1", episode("e1", "s1"), nil, noopSpan())
	assert.False(t, ok)
}

func TestServeConcatMaster_NotRequested(t *testing.T) {
	pkg := &fakeConcatPackager{}
	h := &hlsVODServe{onDemandConcat: pkg}
	ok := h.serveConcatMaster(httptest.NewRecorder(), concatReq(""), "e1", episode("e1", "s1"), nil, noopSpan())
	assert.False(t, ok, "no ?gapless= param: must not even try")
	assert.Zero(t, pkg.calls)
}

func TestServeConcatMaster_EncryptionOnRefuses(t *testing.T) {
	pkg := &fakeConcatPackager{}
	h := &hlsVODServe{onDemandConcat: pkg, onDemandEncrypt: true}
	ok := h.serveConcatMaster(httptest.NewRecorder(), concatReq("?gapless=1"), "e1", episode("e1", "s1"), nil, noopSpan())
	assert.False(t, ok, "concat v1 has no encrypted variant")
	assert.Zero(t, pkg.calls)
}

func TestServeConcatMaster_NoNextEpisodeFallsThrough(t *testing.T) {
	e1 := episode("e1", "s1")
	fake := &chainStreamsGetter{
		items:       map[string]*domainlibrary.MediaItem{"e1": e1},
		streamsByID: map[string][]*domainlibrary.MediaStream{"e1": copyEligibleAV()},
		nextByID:    map[string]*domainlibrary.MediaItem{}, // series finale
	}
	pkg := &fakeConcatPackager{}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.onDemandConcat = pkg
	h.libSvc = fake
	ok := h.serveConcatMaster(httptest.NewRecorder(), concatReq("?gapless=1"), "e1", e1, masterPlan(t, h, e1), noopSpan())
	assert.False(t, ok, "a chain of 1 (no next episode) is not a concat session")
	assert.Zero(t, pkg.calls)
}

// --- resolveEpisodeChain ---

func TestResolveEpisodeChain_StopsAtSeasonBoundary(t *testing.T) {
	e1, e2 := episode("e1", "s1"), episode("e2", "s1")
	e2next := episode("e1-of-s2", "s2") // NextEpisodeOf crosses into season 2
	fake := &chainStreamsGetter{
		items: map[string]*domainlibrary.MediaItem{"e1": e1, "e2": e2},
		streamsByID: map[string][]*domainlibrary.MediaStream{
			"e1": copyEligibleAV(), "e2": copyEligibleAV(),
		},
		nextByID: map[string]*domainlibrary.MediaItem{"e1": e2, "e2": e2next},
	}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.libSvc = fake
	chain := h.resolveEpisodeChain(concatReq("?gapless=1"), e1)
	require.Len(t, chain, 2, "stops before the next-season episode")
	assert.Equal(t, "e2", string(chain[1].ID))
}

func TestResolveEpisodeChain_StopsAtIncompatibleEpisode(t *testing.T) {
	e1, e2, e3 := episode("e1", "s1"), episode("e2", "s1"), episode("e3", "s1")
	// e2 has an HEVC video track the copy-eligibility gate would refuse
	// (fed as-is here; episodeConcatEligible's own decision/eligibility
	// plumbing is exercised - not re-derived by this test).
	incompatible := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "mpeg2video"}, // unsupported for copy
	}
	fake := &chainStreamsGetter{
		items: map[string]*domainlibrary.MediaItem{"e1": e1, "e2": e2, "e3": e3},
		streamsByID: map[string][]*domainlibrary.MediaStream{
			"e1": copyEligibleAV(), "e2": incompatible, "e3": copyEligibleAV(),
		},
		nextByID: map[string]*domainlibrary.MediaItem{"e1": e2, "e2": e3},
	}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.libSvc = fake
	chain := h.resolveEpisodeChain(concatReq("?gapless=1"), e1)
	require.Len(t, chain, 1, "e2 ineligible: chain stops there, e3 never reached")
	assert.Equal(t, "e1", string(chain[0].ID))
}

func TestResolveEpisodeChain_ParentalBlockedStops(t *testing.T) {
	e1, e2 := episode("e1", "s1"), episode("e2", "s1")
	fake := &chainStreamsGetter{
		items:       map[string]*domainlibrary.MediaItem{"e1": e1, "e2": e2},
		streamsByID: map[string][]*domainlibrary.MediaStream{"e1": copyEligibleAV(), "e2": copyEligibleAV()},
		nextByID:    map[string]*domainlibrary.MediaItem{"e1": e2},
	}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.libSvc = fake
	h.parental = testFakeParentalChecker{block: true}
	chain := h.resolveEpisodeChain(concatReq("?gapless=1"), e1)
	assert.Len(t, chain, 1, "e2 parental-blocked for this profile: not chained in")
}

// --- serveConcatMaster: success path ---

func TestServeConcatMaster_BuildsChainAndServes(t *testing.T) {
	e1, e2 := episode("e1", "s1"), episode("e2", "s1")
	fake := &chainStreamsGetter{
		items:       map[string]*domainlibrary.MediaItem{"e1": e1, "e2": e2},
		streamsByID: map[string][]*domainlibrary.MediaStream{"e1": copyEligibleAV(), "e2": copyEligibleAV()},
		nextByID:    map[string]*domainlibrary.MediaItem{"e1": e2},
	}
	pkg := &fakeConcatPackager{plan: &fakeConcatPlan{resources: map[string][]byte{"master.m3u8": []byte("#EXTM3U\n")}}}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.onDemandConcat = pkg
	h.libSvc = fake

	w := httptest.NewRecorder()
	ok := h.serveConcatMaster(w, concatReq("?gapless=1"), "e1", e1, masterPlan(t, h, e1), noopSpan())
	require.True(t, ok)
	assert.Equal(t, []string{"/media/e1.mkv", "/media/e2.mkv"}, pkg.lastSrcs)
	assert.Equal(t, concatPrefix, pkg.lastPrefix)
	assert.Contains(t, w.Body.String(), "#EXTM3U")
	assert.Equal(t, "mkvgo-concat", w.Header().Get("X-Evey-Served-By"))
}

// The first link is judged on the plan the master already resolved, not on a
// second resolution of the same episode: a master plan that is not the
// packager's whole presentation keeps the episode out of the chain even though
// its streams alone would resolve to copy.
func TestServeConcatMaster_JudgesFirstEpisodeOnMasterPlan(t *testing.T) {
	e1, e2 := episode("e1", "s1"), episode("e2", "s1")
	fake := &chainStreamsGetter{
		items:       map[string]*domainlibrary.MediaItem{"e1": e1, "e2": e2},
		streamsByID: map[string][]*domainlibrary.MediaStream{"e1": copyEligibleAV(), "e2": copyEligibleAV()},
		nextByID:    map[string]*domainlibrary.MediaItem{"e1": e2},
	}
	pkg := &fakeConcatPackager{plan: &fakeConcatPlan{resources: map[string][]byte{"master.m3u8": []byte("#EXTM3U\n")}}}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.onDemandConcat = pkg
	h.libSvc = fake

	rp := masterPlan(t, h, e1)
	require.Equal(t, apptranscoding.EngineMkvgo, rp.Plan.Delivery.Video, "fixture: e1's own streams resolve to copy")
	rp.Plan.Delivery.Video = apptranscoding.EngineFFmpeg
	ok := h.serveConcatMaster(httptest.NewRecorder(), concatReq("?gapless=1"), "e1", e1, rp, noopSpan())
	assert.False(t, ok, "the master's plan decides, not a re-resolution")
	assert.Zero(t, pkg.calls)
}

func TestServeConcatMaster_PlanBuildFailsFallsThrough(t *testing.T) {
	e1, e2 := episode("e1", "s1"), episode("e2", "s1")
	fake := &chainStreamsGetter{
		items:       map[string]*domainlibrary.MediaItem{"e1": e1, "e2": e2},
		streamsByID: map[string][]*domainlibrary.MediaStream{"e1": copyEligibleAV(), "e2": copyEligibleAV()},
		nextByID:    map[string]*domainlibrary.MediaItem{"e1": e2},
	}
	pkg := &fakeConcatPackager{err: fmt.Errorf("incompatible audio layout")}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.onDemandConcat = pkg
	h.libSvc = fake
	ok := h.serveConcatMaster(httptest.NewRecorder(), concatReq("?gapless=1"), "e1", e1, masterPlan(t, h, e1), noopSpan())
	assert.False(t, ok, "a mkvgo-level incompatibility (e.g. audio order) falls back cleanly")
}

// --- serveConcatResource: sub-resource routing ---

func TestServeConcatResource_StripsPrefixAndDecodesPartPath(t *testing.T) {
	e1, e2 := episode("e1", "s1"), episode("e2", "s1")
	fake := &chainStreamsGetter{
		items:       map[string]*domainlibrary.MediaItem{"e1": e1, "e2": e2},
		streamsByID: map[string][]*domainlibrary.MediaStream{"e1": copyEligibleAV(), "e2": copyEligibleAV()},
		nextByID:    map[string]*domainlibrary.MediaItem{"e1": e2},
	}
	pkg := &fakeConcatPackager{plan: &fakeConcatPlan{resources: map[string][]byte{"playlist.m3u8": []byte("data")}}}
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.onDemandConcat = pkg
	h.libSvc = fake

	w := httptest.NewRecorder()
	h.serveConcatResource(w, concatReq(""), "e1", concatPrefix+"playlist.m3u8", e1, noopSpan())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "data", w.Body.String())
}

func TestServeConcatResource_FeatureOff404s(t *testing.T) {
	h := &hlsVODServe{}
	w := httptest.NewRecorder()
	h.serveConcatResource(w, concatReq(""), "e1", concatPrefix+"playlist.m3u8", episode("e1", "s1"), noopSpan())
	assert.Equal(t, http.StatusNotFound, w.Code)
}
