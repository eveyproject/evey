package httpapi

// Tests for the on-demand HLS/DASH copy rung (mp4.PlanHLS) routing: the
// eligibility gate (video+audio copy, all audio tracks CMAF-carriable) and the
// serve path (master + prefixed sub-resource), plus the fail-soft fallback to
// the ffmpeg pipeline when the source can't be packaged.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainuser "github.com/evey-project/evey/internal/domain/user"
)

// --- fakes ---

type fakeOnDemandPlan struct {
	resources map[string][]byte
	dash      bool
}

func (p *fakeOnDemandPlan) Resource(_ context.Context, name string) ([]byte, string, error) {
	if d, ok := p.resources[name]; ok {
		return d, "application/vnd.apple.mpegurl", nil
	}
	return nil, "", fmt.Errorf("fake: no resource %q", name)
}
func (p *fakeOnDemandPlan) Resources() []string { return []string{"master.m3u8"} }
func (p *fakeOnDemandPlan) NumSegments() int    { return 1 }
func (p *fakeOnDemandPlan) HasDASH() bool        { return p.dash }

type fakeOnDemandPackager struct {
	plan  *fakeOnDemandPlan
	err   error
	calls int
}

func (p *fakeOnDemandPackager) Plan(_ context.Context, _ string, _ apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return p.plan, nil
}

// copyEligibleAV is one H.264 video + one AAC audio track — the source the
// on-demand rung packages verbatim (video copy + all-audio-carriable).
func copyEligibleAV() []*domainlibrary.MediaStream {
	w, hgt, level := 1920, 1080, 40
	ch := 2
	return []*domainlibrary.MediaStream{
		{
			StreamType: domainlibrary.StreamTypeVideo,
			Codec:      "h264",
			Profile:    "High",
			Width:      &w,
			Height:     &hgt,
			Level:      &level,
			Keyframes:  []int64{0, 6000, 12000},
		},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Channels: &ch, IsDefault: true},
	}
}

func onDemandItem() *domainlibrary.MediaItem {
	return &domainlibrary.MediaItem{
		File: &domainlibrary.MediaFile{Path: "/media/movie.mkv", Size: 1 << 20},
	}
}

func onDemandReq() *http.Request {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4", nil)
	return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
}

// --- eligibility ---

func TestPlanHLSEligible(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	assert.True(t, h.planHLSEligible(onDemandReq(), onDemandItem(), copyEligibleAV()),
		"h264 video + aac audio, both copy → on-demand eligible")

	// Feature not wired → never eligible (falls through to ffmpeg).
	h2 := newCopyTestHandler(t)
	assert.False(t, h2.planHLSEligible(onDemandReq(), onDemandItem(), copyEligibleAV()),
		"onDemand nil → not eligible")
}

func TestPlanHLSEligible_NonCarriableAudio(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	streams := copyEligibleAV()
	streams[1].Codec = "dts" // the packager would copy it verbatim; a client can't decode it
	assert.False(t, h.planHLSEligible(onDemandReq(), onDemandItem(), streams),
		"a non-carriable audio track (dts) forces the ffmpeg fallback")
}

func TestPlanHLSEligible_NoKeyframes(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	streams := copyEligibleAV()
	streams[0].Keyframes = nil // not keyframe-backfilled → isCopyRequest is false
	assert.False(t, h.planHLSEligible(onDemandReq(), onDemandItem(), streams),
		"no keyframe index → not copy → not on-demand eligible")
}

// --- serve ---

func TestTryServePlanMaster_ServesPlan(t *testing.T) {
	h := newCopyTestHandler(t)
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		resources: map[string][]byte{"master.m3u8": []byte("#EXTM3U\nod_playlist.m3u8\n")},
	}}
	h.onDemand = pkg

	w := httptest.NewRecorder()
	served := h.tryServePlanMaster(w, onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), noopSpan())
	assert.True(t, served, "eligible + plan built → served from the packager")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "od_playlist.m3u8")
	assert.Equal(t, 1, pkg.calls)
}

func TestTryServePlanMaster_FallsBackOnBuildError(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{err: fmt.Errorf("no Cues index")}

	w := httptest.NewRecorder()
	served := h.tryServePlanMaster(w, onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), noopSpan())
	assert.False(t, served, "a plan-build error falls through to the ffmpeg master (no response written)")
	assert.Equal(t, http.StatusOK, w.Code, "nothing written yet")
	assert.Empty(t, w.Body.String())
}

func TestServeOnDemandResource_Segment(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		resources: map[string][]byte{"seg00001.m4s": []byte("segbytes")},
	}}

	// The dispatcher strips the od_ prefix before Resource().
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, onDemandReq(), "m1", "od_seg00001.m4s", onDemandItem(), noopSpan())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "segbytes", w.Body.String())
}

func TestServeOnDemandResource_FeatureOff404(t *testing.T) {
	h := newCopyTestHandler(t) // onDemand nil
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, onDemandReq(), "m1", "od_seg00001.m4s", onDemandItem(), noopSpan())
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestServeOnDemandResource_UnknownName404(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{resources: map[string][]byte{}}}
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, onDemandReq(), "m1", "od_bogus.m4s", onDemandItem(), noopSpan())
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func noopSpan() trace.Span { return trace.SpanFromContext(context.Background()) }
