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
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
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
	plan     *fakeOnDemandPlan
	err      error
	calls    int
	lastOpts apptranscoding.OnDemandOptions
}

func (p *fakeOnDemandPackager) Plan(_ context.Context, _ string, opts apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
	p.calls++
	p.lastOpts = opts
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

func TestPlanHLSVideoEligible(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	assert.True(t, h.planHLSVideoEligible(onDemandReq(), onDemandItem(), copyEligibleAV()),
		"h264 copy video → video-eligible")

	// Feature not wired → never eligible (falls through to ffmpeg).
	h2 := newCopyTestHandler(t)
	assert.False(t, h2.planHLSVideoEligible(onDemandReq(), onDemandItem(), copyEligibleAV()),
		"onDemand nil → not eligible")
}

func TestAudioAllCopyEligible(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	assert.True(t, h.audioAllCopyEligible(onDemandReq(), onDemandItem(), copyEligibleAV()),
		"aac audio, copy decision → all-copy eligible (full packager path)")

	// A non-carriable track (dts) → not all-copy → hybrid path instead.
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	assert.False(t, h.audioAllCopyEligible(onDemandReq(), onDemandItem(), streams),
		"a non-carriable audio track (dts) → hybrid (Evey re-encodes), not full copy")
}

func TestPlanHLSVideoEligible_NoKeyframes(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	streams := copyEligibleAV()
	streams[0].Keyframes = nil // not keyframe-backfilled → isCopyRequest is false
	assert.False(t, h.planHLSVideoEligible(onDemandReq(), onDemandItem(), streams),
		"no keyframe index → not copy → not video-eligible")
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

func TestBuildHybridMaster(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts" // audio must be re-encoded → hybrid master

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams)
	require.NoError(t, err)
	assert.Contains(t, master, "od_playlist.m3u8", "video variant points at the packager")
	assert.Contains(t, master, "#EXT-X-MEDIA:TYPE=AUDIO", "Evey audio rendition group present")
	assert.Contains(t, master, "mp4a.40.2", "audio advertised as AAC (re-encoded), not dts")
	assert.NotContains(t, master, "stream_0.m3u8", "video is NOT served by the ffmpeg pipeline")
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

// --- AES-128 encryption ---

func TestDeriveHLSKey(t *testing.T) {
	h := &hlsVODServe{hlsAESSecret: []byte("0123456789abcdef0123456789abcdef")}
	k1 := h.deriveHLSKey("mi-1")
	assert.Len(t, k1, 16, "AES-128 key is 16 bytes")
	assert.Equal(t, k1, h.deriveHLSKey("mi-1"), "deterministic per media")
	assert.NotEqual(t, k1, h.deriveHLSKey("mi-2"), "different media → different key")
}

func TestServeHLSKey_DisabledIs404(t *testing.T) {
	h := &hlsVODServe{} // onDemandEncrypt false, no secret
	w := httptest.NewRecorder()
	h.serveHLSKey(w, onDemandReq(), "mi-1", noopSpan())
	assert.Equal(t, http.StatusNotFound, w.Code, "key endpoint 404s unless encryption is on")
}

func TestServeHLSKey_ServesKeyBytes(t *testing.T) {
	h := &hlsVODServe{onDemandEncrypt: true, hlsAESSecret: []byte("0123456789abcdef0123456789abcdef")}
	w := httptest.NewRecorder()
	h.serveHLSKey(w, onDemandReq(), "mi-1", noopSpan())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, w.Body.Bytes(), 16, "serves the raw 16-byte key")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"), "key must not be cached")
	assert.Equal(t, h.deriveHLSKey("mi-1"), w.Body.Bytes(), "serves this media's derived key")
}

func TestBuildPlan_EncryptPassesAESOptions(t *testing.T) {
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, onDemandEncrypt: true, hlsAESSecret: []byte("0123456789abcdef0123456789abcdef")}

	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem())
	assert.NoError(t, err)
	assert.Len(t, pkg.lastOpts.AESKey, 16, "AES key threaded into the plan options")
	assert.Equal(t, "key", pkg.lastOpts.AESKeyURI)

	// Encryption off → no key in the options.
	pkg2 := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h2 := &hlsVODServe{onDemand: pkg2}
	_, err = h2.buildPlan(onDemandReq(), "mi-1", onDemandItem())
	assert.NoError(t, err)
	assert.Empty(t, pkg2.lastOpts.AESKey, "no AES key when encryption is off")
}

func noopSpan() trace.Span { return trace.SpanFromContext(context.Background()) }
