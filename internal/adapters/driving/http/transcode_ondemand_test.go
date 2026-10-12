package httpapi

// Tests for the on-demand HLS/DASH copy rung (mp4.PlanHLS) routing: the
// eligibility gate (video+audio copy, all audio tracks CMAF-carriable) and the
// serve path (master + prefixed sub-resource), plus the fail-soft fallback to
// the ffmpeg pipeline when the source can't be packaged.

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domainplayback "github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// --- fakes ---

type fakeOnDemandPlan struct {
	resources map[string][]byte
	dash      bool
	sourceID  string
	// refuse makes every Resource fail with ErrSourceChanged, as a plan whose
	// file was replaced under it.
	refuse bool
}

func (p *fakeOnDemandPlan) Resource(_ context.Context, name string) ([]byte, string, error) {
	if p.refuse {
		return nil, "", fmt.Errorf("fake: %w", apptranscoding.ErrSourceChanged)
	}
	if d, ok := p.resources[name]; ok {
		return d, "application/vnd.apple.mpegurl", nil
	}
	return nil, "", fmt.Errorf("fake: no resource %q", name)
}
func (p *fakeOnDemandPlan) Resources() []string { return []string{"master.m3u8"} }
func (p *fakeOnDemandPlan) NumSegments() int    { return 1 }
func (p *fakeOnDemandPlan) HasDASH() bool       { return p.dash }
func (p *fakeOnDemandPlan) SourceID() string    { return p.sourceID }

type fakeOnDemandPackager struct {
	plan     *fakeOnDemandPlan
	err      error
	calls    int
	lastOpts apptranscoding.OnDemandOptions
	// replanned is what Replan returns (the plan of the replaced file).
	replanned *fakeOnDemandPlan
	replans   int
}

func (p *fakeOnDemandPackager) Replan(_ context.Context, _ apptranscoding.OnDemandPlan) (apptranscoding.OnDemandPlan, error) {
	p.replans++
	if p.replanned == nil {
		return nil, fmt.Errorf("fake: no replanned plan")
	}
	p.plan = p.replanned
	return p.replanned, nil
}

func (p *fakeOnDemandPackager) Plan(_ context.Context, _ string, opts apptranscoding.OnDemandOptions) (apptranscoding.OnDemandPlan, error) {
	p.calls++
	p.lastOpts = opts
	if p.err != nil {
		return nil, p.err
	}
	return p.plan, nil
}

// copyEligibleAV is one H.264 video + one AAC audio track - the source the
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

// planFor is the request's plan as serveMaster resolves it: the strategy the
// packager master EXECUTES (it decides nothing itself).
func planFor(t *testing.T, h *hlsVODServe, r *http.Request, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) *requestPlan {
	t.Helper()
	rp, err := h.probePlan(r, "m1", item, streams)
	require.NoError(t, err)
	return rp
}

// The delivery strategy is the PLAN's (resolveDelivery): the handler reads it
// off its own resolution, with its wiring (packager, audio pipeline) as facts.
func TestHandlerPlan_DeliveryStrategy(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.audioRenditionEnabled = true
	h.audio = &audioServe{}

	d := planFor(t, h, onDemandReq(), onDemandItem(), copyEligibleAV()).Plan.Delivery
	assert.Equal(t, apptranscoding.DeliveryStrategy{Video: apptranscoding.EngineMkvgo, Audio: apptranscoding.EngineMkvgo}, d, "h264 copy + aac: the packager's presentation")

	dts := copyEligibleAV()
	dts[1].Codec = "dts"
	d = planFor(t, h, onDemandReq(), onDemandItem(), dts).Plan.Delivery
	assert.Equal(t, apptranscoding.DeliveryStrategy{Video: apptranscoding.EngineMkvgo, Audio: apptranscoding.EngineFFmpeg}, d, "a non-carriable track (dts): the hybrid master")

	noKF := copyEligibleAV()
	noKF[0].Keyframes = nil
	d = planFor(t, h, onDemandReq(), onDemandItem(), noKF).Plan.Delivery
	assert.Equal(t, apptranscoding.EngineFFmpeg, d.Video, "no keyframe index: not a copy, ffmpeg")

	unwired := newCopyTestHandler(t)
	d = planFor(t, unwired, onDemandReq(), onDemandItem(), copyEligibleAV()).Plan.Delivery
	assert.Equal(t, apptranscoding.EngineFFmpeg, d.Video, "no packager wired: ffmpeg")
}

// --- serve ---

func TestTryServePlanMaster_ServesPlan(t *testing.T) {
	h := newCopyTestHandler(t)
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		resources: map[string][]byte{"master.m3u8": []byte("#EXTM3U\nod_playlist.m3u8\n")},
	}}
	h.onDemand = pkg

	w := httptest.NewRecorder()
	served, _ := h.servePackagerMaster(w, onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), planFor(t, h, onDemandReq(), onDemandItem(), copyEligibleAV()), noopSpan())
	assert.True(t, served, "eligible + plan built → served from the packager")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "od_playlist.m3u8")
	assert.Equal(t, 1, pkg.calls)
}

// The master render is where the served path is known for sure: the packager
// copy master records mkvgo on both tracks, keyed to the token's profile so the
// admin panel names the machinery instead of guessing it from the decision.
func TestTryServePlanMaster_RecordsTheServedPath(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		resources: map[string][]byte{"master.m3u8": []byte("#EXTM3U\n")},
	}}
	var gotProfile, gotMedia string
	var gotPath domainplayback.ServingPath
	h.servingRecord = func(_ context.Context, profileID, mediaID string, p domainplayback.ServingPath) {
		gotProfile, gotMedia, gotPath = profileID, mediaID, p
	}

	served, _ := h.servePackagerMaster(httptest.NewRecorder(), onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), planFor(t, h, onDemandReq(), onDemandItem(), copyEligibleAV()), noopSpan())

	assert.True(t, served)
	assert.Equal(t, "p1", gotProfile)
	assert.Equal(t, "m1", gotMedia)
	assert.Equal(t, domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "copy", AudioEngine: "mkvgo"}, gotPath)
}

// The wiring that closes the blind spot: serving from the packager must make
// the media count as active playback. Without it the anti-starvation guards see
// an idle server (no ffmpeg session exists on this path) and let background
// workers compete with the viewer for the same disk.
func TestServePlanResource_MarksPlaybackActive(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		resources: map[string][]byte{"master.m3u8": []byte("#EXTM3U\n")},
	}}
	act := apptranscoding.NewPlaybackActivity(time.Minute)
	h.playbackActivity = act

	assert.Equal(t, 0, act.ActiveSessionCount(), "nothing served yet")

	served, _ := h.servePackagerMaster(httptest.NewRecorder(), onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), planFor(t, h, onDemandReq(), onDemandItem(), copyEligibleAV()), noopSpan())

	assert.True(t, served)
	assert.Equal(t, 1, act.ActiveSessionCount(), "an mkvgo-served playback must register as active")
}

// A fallthrough to ffmpeg must NOT register on-demand activity: that playback
// spawns a real session, which the producer counters already see. Counting it
// here too would double it.
func TestServePlanResource_NoActivityOnFallthrough(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{err: fmt.Errorf("no Cues index")}
	act := apptranscoding.NewPlaybackActivity(time.Minute)
	h.playbackActivity = act

	served, _ := h.servePackagerMaster(httptest.NewRecorder(), onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), planFor(t, h, onDemandReq(), onDemandItem(), copyEligibleAV()), noopSpan())

	assert.False(t, served)
	assert.Equal(t, 0, act.ActiveSessionCount(), "a plan that never served must not count as playback")
}

func TestTryServePlanMaster_FallsBackOnBuildError(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{err: fmt.Errorf("no Cues index")}

	w := httptest.NewRecorder()
	served, _ := h.servePackagerMaster(w, onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), planFor(t, h, onDemandReq(), onDemandItem(), copyEligibleAV()), noopSpan())
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

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams, planFor(t, h, onDemandReq(), item, streams))
	require.NoError(t, err)
	assert.Contains(t, master, "od_playlist.m3u8", "video variant points at the packager")
	assert.Contains(t, master, "#EXT-X-MEDIA:TYPE=AUDIO", "Evey audio rendition group present")
	assert.Contains(t, master, "mp4a.40.2", "audio advertised as AAC (re-encoded), not dts")
	assert.NotContains(t, master, "stream_0.m3u8", "video is NOT served by the ffmpeg pipeline")
}

// The wasm-audio master must name the mkvgo audio rendition CORRECTLY: without
// track-trim the k-th audio track is audio{k+1}; with trim the plan keeps only
// the resolved track and renumbers it to audio1. And ?audioTrack must ride the
// URI so the sub-resource rebuilds the same trimmed plan. Guards the review's
// HIGH finding (wrong index → 404 → silent fallback; missing param → wrong lang).
func TestBuildWasmAudioMaster_AudioRenditionURI(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000}}
	ch := func(n int) *int { return &n }
	vw, vh, lvl := 1920, 1080, 40
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Profile: "High", Width: &vw, Height: &vh, Level: &lvl, Keyframes: []int64{0, 6000, 12000}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: ch(6), IsDefault: true},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: ch(6)},
	}
	mkReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?wasmAudio=1&audioTrack=1&videoCodecs=h264&audioCodecs=aac&containers=mp4", nil)
		return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	}

	// Trim OFF: audioTrack=1 (2nd audio track) → the 2nd rendition, od_audio2.
	h.onDemandTrackTrim = false
	off, err := h.buildWasmAudioMaster(mkReq(), "m1", item, streams, planFor(t, h, mkReq(), item, streams), true, true, 1, nil)
	require.NoError(t, err)
	assert.Contains(t, off, `#EVEY-AUDIO-URI:"od_audio2.m3u8?audioTrack=1`, "untrimmed: k-th track is audio{k+1} and ?audioTrack rides along")

	// Trim ON: the plan keeps one track, renumbered to audio1, whatever the ordinal.
	h.onDemandTrackTrim = true
	on, err := h.buildWasmAudioMaster(mkReq(), "m1", item, streams, planFor(t, h, mkReq(), item, streams), true, true, 1, nil)
	require.NoError(t, err)
	assert.Contains(t, on, `#EVEY-AUDIO-URI:"od_audio1.m3u8?audioTrack=1`, "trimmed: the kept track is renumbered to audio1")
	assert.NotContains(t, on, "od_audio2.m3u8", "a trimmed plan has no audio2 to reference")
}

// The wasm-audio master must ALSO name the AAC rendition of the same track
// (#EVEY-AUDIO-AAC-URI) so the client can start with native decode while the
// wasm decoder boots (bi-source relay), and that URI must NOT carry wasmAudio=1
// (with it, the audio handler serves the AC-3 copy, not AAC). Only emitted when
// the audio-rendition feature is on - its handlers 404 otherwise.
func TestBuildWasmAudioMaster_AacRelayURI(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000}}
	ch := func(n int) *int { return &n }
	vw, vh, lvl := 1920, 1080, 40
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Profile: "High", Width: &vw, Height: &vh, Level: &lvl, Keyframes: []int64{0, 6000, 12000}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: ch(6), IsDefault: true},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: ch(6)},
	}
	mkReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?wasmAudio=1&audioTrack=1&videoCodecs=h264&audioCodecs=aac&containers=mp4", nil)
		return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	}

	h.audioRenditionEnabled = true
	master, err := h.buildWasmAudioMaster(mkReq(), "m1", item, streams, planFor(t, h, mkReq(), item, streams), true, true, 1, nil)
	require.NoError(t, err)
	assert.Contains(t, master, `#EVEY-AUDIO-AAC-URI:"audio_1.m3u8?`, "relay tag names the AAC rendition of the SAME track")
	aacLine := ""
	for line := range strings.SplitSeq(master, "\n") {
		if strings.HasPrefix(line, "#EVEY-AUDIO-AAC-URI:") {
			aacLine = line
		}
	}
	assert.Contains(t, aacLine, "audioTrack=1", "decision params ride the AAC URI")
	assert.NotContains(t, aacLine, "wasmAudio", "wasmAudio=1 on the AAC URI would serve the AC-3 copy instead")

	// Feature off: the AAC handlers 404, so the tag must not be advertised.
	h.audioRenditionEnabled = false
	master, err = h.buildWasmAudioMaster(mkReq(), "m1", item, streams, planFor(t, h, mkReq(), item, streams), true, true, 1, nil)
	require.NoError(t, err)
	assert.NotContains(t, master, "#EVEY-AUDIO-AAC-URI")
}

func TestServeOnDemandResource_Segment(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		resources: map[string][]byte{"seg00001.m4s": []byte("segbytes")},
	}}

	// The dispatcher strips the od_ prefix before Resource().
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, onDemandReq(), "m1", "od_seg00001.m4s", onDemandItem(), nil, noopSpan())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "segbytes", w.Body.String())
}

func TestServeOnDemandResource_FeatureOff404(t *testing.T) {
	h := newCopyTestHandler(t) // onDemand nil
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, onDemandReq(), "m1", "od_seg00001.m4s", onDemandItem(), nil, noopSpan())
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestServeOnDemandResource_UnknownName404(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{resources: map[string][]byte{}}}
	w := httptest.NewRecorder()
	h.serveOnDemandResource(w, onDemandReq(), "m1", "od_bogus.m4s", onDemandItem(), nil, noopSpan())
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

// --- AES-128 key rotation (forward secrecy) ---

func TestDeriveHLSKeyN(t *testing.T) {
	h := &hlsVODServe{hlsAESSecret: []byte("0123456789abcdef0123456789abcdef")}
	k0 := h.deriveHLSKeyN("mi-1", 0)
	assert.Len(t, k0, 16)
	assert.Equal(t, k0, h.deriveHLSKeyN("mi-1", 0), "deterministic per (media, index)")
	assert.NotEqual(t, k0, h.deriveHLSKeyN("mi-1", 1), "different period → different key")
	assert.NotEqual(t, k0, h.deriveHLSKey("mi-1"), "rotated key differs from the single-key derivation")
}

func TestServeHLSKey_RotationIndex(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	h := &hlsVODServe{onDemandEncrypt: true, hlsAESSecret: secret, hlsKeyRotateSegments: 10}
	keyReq := func(q string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/hls/mi-1/key"+q, nil)
		w := httptest.NewRecorder()
		h.serveHLSKey(w, req, "mi-1", noopSpan())
		return w
	}

	w2 := keyReq("?i=2")
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, h.deriveHLSKeyN("mi-1", 2), w2.Body.Bytes(), "serves the requested period's key")
	assert.NotEqual(t, h.deriveHLSKeyN("mi-1", 3), w2.Body.Bytes(), "period 2 != period 3")

	// No index → the single-key derivation (backward compatible).
	assert.Equal(t, h.deriveHLSKey("mi-1"), keyReq("").Body.Bytes())

	// Malformed / out-of-range indices are rejected, not silently derived.
	for _, bad := range []string{"?i=abc", "?i=-1", "?i=99999"} {
		assert.Equal(t, http.StatusBadRequest, keyReq(bad).Code, bad)
	}
}

func TestBuildPlan_RotationPassesKeys(t *testing.T) {
	dur, err := shared.NewDurationMs(600000) // 10 min → ~100 segments at 6s
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/media/movie.mkv", Duration: dur}}

	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, onDemandEncrypt: true, hlsAESSecret: []byte("0123456789abcdef0123456789abcdef"), hlsKeyRotateSegments: 10}
	_, err = h.buildPlan(onDemandReq(), "mi-1", item, copyEligibleAV(), h.planOf(onDemandReq(), "mi-1", item, copyEligibleAV()))
	require.NoError(t, err)

	assert.Empty(t, pkg.lastOpts.AESKey, "rotation supersedes the single key")
	assert.Equal(t, 10, pkg.lastOpts.AESRotateSegments)
	assert.GreaterOrEqual(t, len(pkg.lastOpts.AESKeys), 2, "at least two rotation keys")
	assert.Equal(t, "key?i=0", pkg.lastOpts.AESKeys[0].URI)
	assert.Equal(t, h.deriveHLSKeyN("mi-1", 0), pkg.lastOpts.AESKeys[0].Key)
	assert.Equal(t, "key?i=1", pkg.lastOpts.AESKeys[1].URI)
}

func TestBuildPlan_EncryptPassesAESOptions(t *testing.T) {
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, onDemandEncrypt: true, hlsAESSecret: []byte("0123456789abcdef0123456789abcdef")}

	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV(), h.planOf(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV()))
	require.NoError(t, err)
	assert.Len(t, pkg.lastOpts.AESKey, 16, "AES key threaded into the plan options")
	assert.Equal(t, "key", pkg.lastOpts.AESKeyURI)

	// Encryption off → no key in the options.
	pkg2 := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h2 := &hlsVODServe{onDemand: pkg2}
	_, err = h2.buildPlan(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV(), h2.planOf(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV()))
	require.NoError(t, err)
	assert.Empty(t, pkg2.lastOpts.AESKey, "no AES key when encryption is off")
}

// --- track trim (transcoding.hls_ondemand_track_trim) ---

func TestBuildPlan_TrackTrimOff_NoRestriction(t *testing.T) {
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, libSvc: &testFakeStreamsGetter{streams: copyEligibleAV()}}
	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV(), h.planOf(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV()))
	require.NoError(t, err)
	assert.Nil(t, pkg.lastOpts.KeepAudioTrack, "flag off: today's untrimmed behaviour")
}

func TestBuildPlan_TrackTrimOn_ResolvesAudioTrack(t *testing.T) {
	streams := copyEligibleAV() // one audio track (ordinal 0)
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: true, libSvc: &testFakeStreamsGetter{streams: streams}}
	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV(), h.planOf(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV()))
	require.NoError(t, err)
	require.NotNil(t, pkg.lastOpts.KeepAudioTrack)
	assert.Equal(t, 0, *pkg.lastOpts.KeepAudioTrack)
}

func TestBuildPlan_TrackTrimOn_HonoursExplicitAudioTrackQuery(t *testing.T) {
	w, hgt, lvl, ch := 1920, 1080, 40, 2
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Profile: "High", Width: &w, Height: &hgt, Level: &lvl, Keyframes: []int64{0}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Channels: &ch, IsDefault: true},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac", Channels: &ch, Language: "fre"},
	}
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: true, libSvc: &testFakeStreamsGetter{streams: streams}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?audioTrack=1", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))

	_, err := h.buildPlan(req, "mi-1", onDemandItem(), streams, h.planOf(req, "mi-1", onDemandItem(), streams))
	require.NoError(t, err)
	require.NotNil(t, pkg.lastOpts.KeepAudioTrack)
	assert.Equal(t, 1, *pkg.lastOpts.KeepAudioTrack, "explicit ?audioTrack= wins over the profile default, same as the ffmpeg pipeline")
}

func TestBuildPlan_TrackTrimOn_NoStreamsServesUntrimmed(t *testing.T) {
	// The caller normally passes commonAuth's streams; an empty slice (edge:
	// unprobed media) must serve every track, never refuse the plan.
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: true}
	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), nil, h.planOf(onDemandReq(), "mi-1", onDemandItem(), nil))
	require.NoError(t, err, "missing streams must not refuse the plan")
	assert.Nil(t, pkg.lastOpts.KeepAudioTrack, "best-effort: falls back to every track")
}

func noopSpan() trace.Span { return trace.SpanFromContext(context.Background()) }

// --- Chromecast cast-token propagation into on-demand manifest URIs ---

func TestRebakeManifestURIs_CastToken(t *testing.T) {
	in := "#EXTM3U\n" +
		"#EXT-X-KEY:METHOD=AES-128,URI=\"key\",IV=0x00\n" +
		"#EXT-X-MAP:URI=\"od_init.mp4\"\n" +
		"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"a\",URI=\"od_audio.m3u8\"\n" +
		"#EXTINF:6.0,\n" +
		"od_seg00001.m4s\n"
	out := rebakeManifestURIs(in, "?token=CASTTOK")
	for _, want := range []string{
		"URI=\"key?token=CASTTOK\"",
		"URI=\"od_init.mp4?token=CASTTOK\"",
		"URI=\"od_audio.m3u8?token=CASTTOK\"",
		"od_seg00001.m4s?token=CASTTOK",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in rebaked manifest, got:\n%s", want, out)
		}
	}
	// Idempotent: a URI that already has a query is left alone.
	again := rebakeManifestURIs(out, "?token=CASTTOK")
	if strings.Contains(again, "?token=CASTTOK?token=CASTTOK") {
		t.Errorf("double-appended token on second pass:\n%s", again)
	}
	// Master reference to a child playlist gets suffixed too.
	master := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nod_playlist.m3u8\n"
	if mo := rebakeManifestURIs(master, "?token=T"); !strings.Contains(mo, "od_playlist.m3u8?token=T") {
		t.Errorf("master child not suffixed:\n%s", mo)
	}
	// Empty suffix is a no-op.
	if rebakeManifestURIs(in, "") != in {
		t.Error("empty suffix should be a no-op")
	}
}

func TestRebakeDASHManifestURIs_CastToken(t *testing.T) {
	in := `<SegmentTemplate initialization="od_init.mp4" media="od_$Number$.m4s"></SegmentTemplate>`
	out := rebakeDASHManifestURIs(in, "?token=CASTTOK")
	if !strings.Contains(out, `initialization="od_init.mp4?token=CASTTOK"`) {
		t.Errorf("init not suffixed: %s", out)
	}
	if !strings.Contains(out, `media="od_$Number$.m4s?token=CASTTOK"`) {
		t.Errorf("media template not suffixed: %s", out)
	}
	// Already-queried attribute is left alone.
	q := `media="od_$Number$.m4s?x=1"`
	if rebakeDASHManifestURIs(q, "?token=T") != q {
		t.Error("already-queried media attr should be untouched")
	}
}

// The rebaked manifest stays XML: the '&' of a multi-parameter suffix is
// escaped in the attribute (Media3 refused the raw form as malformed), and
// a '$' in the suffix is taken as is, not as a replacement group.
func TestRebakeDASHManifestURIs_StaysWellFormedXML(t *testing.T) {
	in := `<MPD><SegmentTemplate initialization="od_init.mp4" media="od_seg$Number%05d$.m4s"/></MPD>`
	out := rebakeDASHManifestURIs(in, "?v=v0.32.0&token=T&audioCodecs=aac&x=$1")
	assert.Contains(t, out, `initialization="od_init.mp4?v=v0.32.0&amp;token=T&amp;audioCodecs=aac&amp;x=$1"`)
	assert.Contains(t, out, `media="od_seg$Number%05d$.m4s?v=v0.32.0&amp;token=T&amp;audioCodecs=aac&amp;x=$1"`)
	var doc struct{}
	require.NoError(t, xml.Unmarshal([]byte(out), &doc))
}

// The mkvgo packager rung must NOT inherit the ffmpeg copy rung's preconditions
// (a wired copyProducer, cache room for a full copy): mkvgo is STATELESS. The
// plan's strategy reads the packager's own gate (VideoCopyEligible), so a
// disabled ffmpeg copy rung leaves the video on the packager.
func TestHandlerPlan_PackagerIndependentOfFFmpegCopyRung(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.copyProducer = nil // operator disabled the ffmpeg copy/remux rung

	d := planFor(t, h, onDemandReq(), onDemandItem(), copyEligibleAV()).Plan.Delivery
	assert.Equal(t, apptranscoding.EngineMkvgo, d.Video, "mkvgo packages without ffmpeg: a disabled copy rung must not disable it")
}

// countingStreamsGetter fails the test intent if the plan builder re-fetches
// streams: commonAuth already fetched them for this very request.
type countingStreamsGetter struct {
	testFakeStreamsGetter
	calls atomic.Int32
}

func (c *countingStreamsGetter) GetMediaStreams(ctx context.Context, id string) ([]*domainlibrary.MediaStream, error) {
	c.calls.Add(1)
	return c.testFakeStreamsGetter.GetMediaStreams(ctx, id)
}

// The trim path used to re-query Postgres for the streams of EVERY on-demand
// sub-resource (playlist, init, each segment) although commonAuth had just
// fetched them for the same request (~3.4 free queries/s at 20 viewers).
// buildPlan must reuse the streams the caller already holds - zero lookups.
func TestBuildPlan_TrackTrimOn_ReusesAuthStreams_NoRefetch(t *testing.T) {
	streams := copyEligibleAV()
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	counter := &countingStreamsGetter{}
	h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: true, libSvc: counter}
	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), streams, h.planOf(onDemandReq(), "mi-1", onDemandItem(), streams))
	require.NoError(t, err)
	require.NotNil(t, pkg.lastOpts.KeepAudioTrack)
	assert.Equal(t, 0, *pkg.lastOpts.KeepAudioTrack)
	assert.Equal(t, int32(0), counter.calls.Load(), "buildPlan must reuse commonAuth's streams, not re-query")
}

// With the fallback rung enabled, the hybrid master advertises ONE low
// transcoded rung after the copy variant: the lowest profile of the same
// request's natural vd=transcode plan, addressed as stream_0 with THAT plan's
// own baked suffix (vd pin + fp seal). The copy path is otherwise
// single-variant - a throughput collapse then freezes the picture instead of
// degrading it, which is the failure this rung exists to absorb.
func TestBuildHybridMaster_FallbackRung(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts" // audio must be re-encoded → hybrid master

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams, planFor(t, h, onDemandReq(), item, streams))
	require.NoError(t, err)

	assert.Contains(t, master, "od_playlist.m3u8", "the copy variant is still served by the packager")
	assert.Contains(t, master, "stream_0.m3u8", "the fallback rung rides the ffmpeg ladder")
	assert.Less(t, strings.Index(master, "od_playlist.m3u8"), strings.Index(master, "stream_0.m3u8"),
		"copy stays the first listed variant")

	var fbURI string
	for line := range strings.SplitSeq(master, "\n") {
		if strings.HasPrefix(line, "stream_0.m3u8") {
			fbURI = line
		}
	}
	require.NotEmpty(t, fbURI)
	assert.Contains(t, fbURI, "vd=transcode", "the rung pins ITS plan's video decision")
	assert.Contains(t, fbURI, "fp=", "the rung seals ITS plan's fingerprint")
	assert.NotContains(t, fbURI, "vd=copy")
	assert.NotContains(t, fallbackInf(master), "AUDIO=", "its plan muxes its audio here: no group")
}

func fallbackInf(master string) string {
	lines := strings.Split(master, "\n")
	for i, line := range lines {
		if i > 0 && strings.HasPrefix(line, "stream_0.m3u8") {
			return lines[i-1]
		}
	}
	return ""
}

// With the delay probes wired (production), the rung's own plan serves its
// audio as renditions and its segments are video-only (measured on the dev
// server 2026-09-24: the rung's init holds one h264 track). The rung must then
// name this master's audio group, or a player on it has no sound (ExoPlayer)
// or plays a group the master never tied to it (hls.js).
func TestBuildHybridMaster_FallbackRung_VideoOnlyPlaysTheAudioGroup(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true
	h.frameRateProbe = func(context.Context, string) float64 { return 24 }
	h.encoderReorderProbe = func(context.Context, string, string, string, []string) (int, bool) { return 2, true }

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams, planFor(t, h, onDemandReq(), item, streams))
	require.NoError(t, err)
	inf := fallbackInf(master)
	require.NotEmpty(t, inf, "premise: the fallback rung is advertised")
	assert.Contains(t, inf, `AUDIO="aud"`)
	assert.Contains(t, inf, `CODECS="avc1.`)
	assert.Contains(t, inf, `,mp4a.40.2"`, "the group's codec, as on the copy rung")
}

// The rung must buy a real drop: on a source already lean enough that the
// lowest transcode saves nothing (peak within 2x), the master stays
// single-variant - an extra variant there is switching risk for zero relief.
func TestBuildHybridMaster_FallbackRung_SkippedOnLeanSource(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 1_500_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams, planFor(t, h, onDemandReq(), item, streams))
	require.NoError(t, err)
	assert.Contains(t, master, "od_playlist.m3u8")
	assert.NotContains(t, master, "stream_0.m3u8",
		"lowest transcode rung within 2x of the copy peak → no fallback advertised")
}

// Flag off = today's master, bit-for-bit single-variant (the operator opt-out,
// and the shape every pre-feature client was validated against).
func TestBuildHybridMaster_FallbackRung_FlagOff(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = false

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams, planFor(t, h, onDemandReq(), item, streams))
	require.NoError(t, err)
	assert.NotContains(t, master, "stream_0.m3u8")
}

// A cast receiver never gets the rung: CAF never dives onto an in-master rung
// by itself (Bouygtel4K 2026-08-03, variant pinned through 9 minutes of
// freeze - the receiver's abr=floor reload is the rescue path instead), and
// the mux/demux-asymmetric master it creates stalls the Chromecast HD's MPL
// before a single media fetch (2026-08-05: master fetched twice, then zero
// playlist/segment requests, no ERROR event, spinner forever - while the same
// TV played a muxed transcode ladder for 25 minutes the same afternoon).
func TestBuildHybridMaster_FallbackRung_SkippedForCast(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"

	req := onDemandReq()
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey,
		&shared.StreamToken{UserID: "u1", ProfileID: "p1", MediaItemID: "m1", Scope: shared.ScopeStream, Cast: true}))

	master, err := h.buildHybridMaster(req, "m1", item, streams, planFor(t, h, req, item, streams))
	require.NoError(t, err)
	assert.Contains(t, master, "od_playlist.m3u8", "the copy variant still serves")
	assert.NotContains(t, master, "stream_0.m3u8",
		"a cast master stays single-variant: the rung CAF never takes must not poison its MPL")
}

// lineItem is a copyable 1080p source whose copy pushes bitrateBps down the
// line (the plan's SourcePeakKbps).
func lineItem(t *testing.T, bitrateBps int64) *domainlibrary.MediaItem {
	t.Helper()
	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	return &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: bitrateBps,
	}}
}

// lineReq is onDemandReq with the receiver's line constraints set.
func lineReq(maxKbps string, floor bool) *http.Request {
	req := onDemandReq()
	q := req.URL.Query()
	if maxKbps != "" {
		q.Set("maxKbps", maxKbps)
	}
	if floor {
		q.Set("abr", "floor")
	}
	req.URL.RawQuery = q.Encode()
	return req
}

// lineHandler is the copy handler with a manifest generator: the ffmpeg
// master path renders the fitted plan.
func lineHandler(t *testing.T) *hlsVODServe {
	t.Helper()
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true
	return h
}

// masterOf renders the ffmpeg master the handler serves for a resolved plan.
func masterOf(t *testing.T, h *hlsVODServe, req *http.Request, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream, rp *requestPlan) string {
	t.Helper()
	in, err := h.buildManifestInputWith(req, "m1", item, streams, rp)
	require.NoError(t, err)
	master, err := h.manifestGen.Master(in)
	require.NoError(t, err)
	return master
}

// ?abr=floor is the survival mode the cast receiver's starvation rescue asks
// for: the announced rung exists but CAF never dives onto it by itself
// (measured on the Bouygtel4K, 2026-08-03, variant pinned through 9 minutes
// of freeze), so the receiver reloads with this param and the PLAN fits
// itself to the lowest transcoded rung; the master the adapter renders is
// MONO-VARIANT and the variant URI carries the rung's LADDER index.
func TestLineFit_FloorServesOnlyTheLowestRung(t *testing.T) {
	h := lineHandler(t)
	item := lineItem(t, 4_000_000)
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	req := lineReq("", true)

	rp := planFor(t, h, req, item, streams)
	require.False(t, rp.IsCopy, "floor must resolve to a transcode on a fat copy-eligible source")
	assert.Equal(t, apptranscoding.LineFit{Single: true, Rung: 0}, rp.Plan.Fit)
	assert.Contains(t, rp.Decision.TranscodeReasons, "abr_floor_requested")

	master := masterOf(t, h, req, item, streams, rp)
	assert.Equal(t, 1, strings.Count(master, "#EXT-X-STREAM-INF"), "floor is mono-variant by definition")
	assert.NotContains(t, master, "od_playlist.m3u8", "the copy variant must NOT be offered in survival mode")
	assert.Contains(t, master, "stream_0.m3u8")
	assert.Contains(t, master, "vd=transcode", "the rung pins its plan")
	assert.Contains(t, master, "abr=floor", "the constraint is a plan input: baked like the pins")
	assert.Contains(t, master, "fp=", "the rung seals its fingerprint")
}

// Incident cast du 2026-08-23 : le cast a gele a tampon PLEIN sur un flux
// copie de 6,87 Mbps, et l'analyse a conclu que "la box n'a nulle part ou
// retomber". C'est vrai du MASTER (copyFallbackVariant renvoie nil pour tout
// recepteur cast, cf. TestBuildHybridMaster_FallbackRung_SkippedForCast) mais
// FAUX du serveur : la sortie de secours ?abr=floor existe et repond a un
// jeton cast comme a n'importe quel autre.
func TestLineFit_FloorStaysAvailableToACastReceiver(t *testing.T) {
	h := lineHandler(t)
	item := lineItem(t, 6_870_000)
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	req := lineReq("", true)
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey,
		&shared.StreamToken{UserID: "u1", ProfileID: "p1", MediaItemID: "m1", Scope: shared.ScopeStream, Cast: true}))

	rp := planFor(t, h, req, item, streams)
	require.True(t, rp.Plan.Fit.Single, "un recepteur cast doit obtenir le barreau de survie quand il le demande")
	master := masterOf(t, h, req, item, streams, rp)
	assert.Equal(t, 1, strings.Count(master, "#EXT-X-STREAM-INF"))
	assert.NotContains(t, master, "od_playlist.m3u8", "le barreau copie ne doit pas etre offert en mode survie")
	assert.Contains(t, master, "vd=transcode")
}

// The variant URI must carry its baked query exactly ONCE (prod 2026-08-05:
// a pre-suffixed URI doubled it, "...?ad=transcode&...?vd=..." inherited by
// every segment URI of the rescue path). And it must be SERVEABLE: the child
// request re-derives its plan from the baked query alone and the recomputed
// fingerprint must equal the sealed one, or refuseStaleFP answers 410 and the
// rung is dead on arrival.
func TestLineFit_VariantURICarriesItsQueryOnceAndResolvesTheSealedPlan(t *testing.T) {
	h := lineHandler(t)
	item := lineItem(t, 4_000_000)
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	req := lineReq("", true)

	rp := planFor(t, h, req, item, streams)
	require.True(t, rp.Plan.Fit.Single)
	master := masterOf(t, h, req, item, streams, rp)
	var variant string
	for line := range strings.SplitSeq(master, "\n") {
		if strings.HasPrefix(line, "stream_0.m3u8") {
			variant = line
		}
	}
	require.NotEmpty(t, variant)
	assert.Equal(t, 1, strings.Count(variant, "?"), "one URI, one query: %s", variant)
	assert.Equal(t, 1, strings.Count(variant, "vd=transcode"), "the baked suffix must not double: %s", variant)

	child := onDemandReq()
	child.URL.RawQuery = variant[strings.Index(variant, "?")+1:]
	crp := planFor(t, h, child, item, streams)
	assert.Equal(t, rp.FPHash, crp.FPHash, "the child resolves the plan the master sealed")
	assert.False(t, crp.Plan.Fit.Single, "a child indexes the full ladder; the master's verdict rides its pin")
}

// ?maxKbps=<measured> is the receiver's own bitrate measurement (its network
// stack included - old cast runtimes cap well under the wire speed, proven on
// the Bouygtel4K 2026-08-03). Copy-first stays the rule: the line only bites
// when it cannot carry the copy.
func TestLineFit_CopyFirstWhenTheLineCarriesIt(t *testing.T) {
	h := lineHandler(t)
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	rp := planFor(t, h, lineReq("12000", false), lineItem(t, 4_000_000), streams)
	assert.True(t, rp.IsCopy, "a measure that carries the copy with margin leaves the copy plan alone")
	assert.False(t, rp.Plan.Fit.Single)
}

// When the line cannot carry the copy the plan fits itself to the HIGHEST
// rung that fits - mono-variant, so the receiver's own ABR (the atrophied
// muscle) is never needed - and the variant URI carries that rung's LADDER
// index: the plan's ladder is the natural transcode one, shared with any
// other transcode viewer of the title.
func TestLineFit_PicksTheHighestFittingRung(t *testing.T) {
	h := lineHandler(t)
	item := lineItem(t, 7_400_000)
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	const maxKbps = 6000

	// Expected rung: the same rule, computed from the same natural transcode
	// plan - no hardcoded ladder.
	rT := onDemandReq()
	qq := rT.URL.Query()
	qq.Set("vd", "transcode")
	rT.URL.RawQuery = qq.Encode()
	rpT := planFor(t, h, rT, item, streams)
	require.NotEmpty(t, rpT.Profiles)
	want := 0
	for i, p := range rpT.Profiles {
		if (p.VideoBitrate+p.AudioBitrate)*12 <= maxKbps*10 {
			want = i
		}
	}

	req := lineReq("6000", false)
	rp := planFor(t, h, req, item, streams)
	require.False(t, rp.IsCopy, "a 7.4 Mbps copy cannot ride a 6 Mbps measure - the plan must fit a rung")
	assert.Equal(t, apptranscoding.LineFit{Single: true, Rung: want}, rp.Plan.Fit)
	assert.Equal(t, rpT.FPHash, rp.FPHash, "the fitted plan IS the natural transcode plan: same sessions, same cache")
	assert.Contains(t, rp.Decision.TranscodeReasons, "client_line_exceeded")

	master := masterOf(t, h, req, item, streams, rp)
	assert.Equal(t, 1, strings.Count(master, "#EXT-X-STREAM-INF"), "fitted master is mono-variant")
	assert.Contains(t, master, fmt.Sprintf("stream_%d.m3u8", want),
		"the variant URI must carry the LADDER index of the chosen rung, or the fp seal serves the wrong profile")
	assert.Contains(t, master, "maxKbps=6000")
	assert.NotContains(t, master, "od_playlist.m3u8")
	// The DASH manifest names the same rung's fragments (stream_<rung>_*).
	in, err := h.buildManifestInputWith(req, "m1", item, streams, rp)
	require.NoError(t, err)
	require.Positive(t, want, "a rung above the first, or the index proves nothing")
	assert.Equal(t, want, in.FirstRungIndex)
}

func TestServeBitrateTest_StreamsTheRequestedSize(t *testing.T) {
	h := newCopyTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/bitrate-test?size=100000", nil)
	w := httptest.NewRecorder()
	h.serveBitrateTest(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 100000, w.Body.Len())
	assert.Equal(t, "no-store", w.Result().Header.Get("Cache-Control"))

	// The size is capped: a client cannot turn the measure into a bandwidth drain.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/bitrate-test?size=999999999", nil)
	w = httptest.NewRecorder()
	h.serveBitrateTest(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, bitrateTestMaxBytes, w.Body.Len())

	// No size: a sane default the receiver can time in ~1s on a slow link.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/bitrate-test", nil)
	w = httptest.NewRecorder()
	h.serveBitrateTest(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, bitrateTestDefaultBytes, w.Body.Len())
}

// A lean source has no rung meaningfully below the copy: the floor leaves the
// copy plan alone rather than fitting a pointless sidegrade.
func TestLineFit_FloorRefusedOnLeanSource(t *testing.T) {
	h := lineHandler(t)
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	rp := planFor(t, h, lineReq("", true), lineItem(t, 1_500_000), streams)
	assert.True(t, rp.IsCopy)
	assert.False(t, rp.Plan.Fit.Single)
}

func TestBuildHybridMaster_FallbackRung_URIResolvesCoherently(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 4_000_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams, planFor(t, h, onDemandReq(), item, streams))
	require.NoError(t, err)
	var fbURI string
	for line := range strings.SplitSeq(master, "\n") {
		if strings.HasPrefix(line, "stream_0.m3u8") {
			fbURI = line
		}
	}
	require.NotEmpty(t, fbURI, "premise: the fallback rung is advertised")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/"+fbURI, nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	w := httptest.NewRecorder()
	h.serveVariant(w, req, "m1", "stream_0.m3u8", item, streams, noopSpan())

	require.Equal(t, http.StatusOK, w.Code, "sealed fp must match the re-derived plan (410 = dead rung): %s", w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, "#EXT-X-PLAYLIST-TYPE:VOD")
	assert.Contains(t, body, "stream_0_", "segment URIs address rung 0")
	assert.Contains(t, body, "vd=transcode", "segments inherit the transcode pin")
}

// The exact shape of the 2026-09-08 report: a 4K remux whose audio is one
// TrueHD/Atmos track plus several AC-3 5.1 ones, played by a receiver that
// decodes AC-3 natively. audioAllCopyEligible is all-or-nothing, so the single
// un-carriable TrueHD track sent all three AC-3 tracks through Evey's AAC
// encoder - for a track the viewer was not playing. Trimming the presentation
// to the selected track is what lets it be carried verbatim.
func multiTrackTrueHDPlusAC3() []*domainlibrary.MediaStream {
	w, hgt, level := 3840, 2160, 153
	ch := 6
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
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "truehd", Channels: &ch},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: &ch, IsDefault: true},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3", Channels: &ch},
	}
}

// A receiver that decodes AC-3 and takes 6 channels, asking for the 2nd AC-3
// track (audio ordinal 2).
func ac3ReceiverReq() *http.Request {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264,hevc&audioCodecs=aac,ac3&containers=mp4&maxAudioChannels=6&audioTrack=2", nil)
	return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
}

// One uncarriable track (TrueHD) no longer poisons the rest: the plan trims
// the presentation to the selected AC-3 track and keeps it on the packager.
func TestHandlerPlan_TrimUnlocksThePackagerAudio(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.audioRenditionEnabled = true
	h.audio = &audioServe{}

	rp := planFor(t, h, ac3ReceiverReq(), onDemandItem(), multiTrackTrueHDPlusAC3())
	require.Equal(t, "copy", rp.Decision.AudioDecision, "an AC-3 5.1 track on an AC-3/6-channel receiver is a copy")
	assert.Equal(t, apptranscoding.DeliveryStrategy{Video: apptranscoding.EngineMkvgo, Audio: apptranscoding.EngineMkvgo, AudioTrimmed: true}, rp.Plan.Delivery,
		"trimming to the selected AC-3 track makes the presentation carriable verbatim")
}

func TestHandlerPlan_SelectedTrackItselfUncarriableIsHybrid(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.audioRenditionEnabled = true
	h.audio = &audioServe{}

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264&audioCodecs=aac,ac3,truehd&containers=mp4&maxAudioChannels=8&audioTrack=0", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))

	d := planFor(t, h, req, onDemandItem(), multiTrackTrueHDPlusAC3()).Plan.Delivery
	assert.Equal(t, apptranscoding.DeliveryStrategy{Video: apptranscoding.EngineMkvgo, Audio: apptranscoding.EngineFFmpeg}, d,
		"TrueHD is the selected track and CMAF cannot carry it - no trim rescues that: the hybrid master")
}

func TestHandlerPlan_StereoOnlyClientStillFolds(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h.audioRenditionEnabled = true
	h.audio = &audioServe{}

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264&audioCodecs=aac,ac3&containers=mp4&maxAudioChannels=2&audioTrack=2", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))

	rp := planFor(t, h, req, onDemandItem(), multiTrackTrueHDPlusAC3())
	require.NotEqual(t, "copy", rp.Decision.AudioDecision, "5.1 into a 2-channel output has to be folded")
	assert.Equal(t, apptranscoding.EngineFFmpeg, rp.Plan.Delivery.Audio, "a folded track is re-encoded, trimmed or not")
}

// End to end through the serve path: the master comes from the packager (not
// the hybrid AAC one) and the plan it built was trimmed to the selected track.
func TestTryServePlanMaster_TrimsToTheCarriableTrack(t *testing.T) {
	h := newCopyTestHandler(t)
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{
		resources: map[string][]byte{"master.m3u8": []byte("#EXTM3U\nod_playlist.m3u8\n")},
	}}
	h.onDemand = pkg

	w := httptest.NewRecorder()
	served, _ := h.servePackagerMaster(w, ac3ReceiverReq(), "m1", onDemandItem(), multiTrackTrueHDPlusAC3(), planFor(t, h, ac3ReceiverReq(), onDemandItem(), multiTrackTrueHDPlusAC3()), noopSpan())

	assert.True(t, served, "the AC-3 track is carriable once the presentation is trimmed to it")
	assert.Contains(t, w.Body.String(), "od_playlist.m3u8", "served by the packager, no ffmpeg audio encoder")
	require.NotNil(t, pkg.lastOpts.KeepAudioTrack, "the plan must be trimmed, or the dropped TrueHD track would renumber the rest")
	assert.Equal(t, 2, *pkg.lastOpts.KeepAudioTrack, "trimmed to the REQUESTED track, not the first one")
}

// qsvHEVCCapability is the prod box of 2026-09-15 (UHD 630, QSV primary, HEVC
// route proven): the resolver upgrades an HEVC-capable client's transcode to a
// GPU HEVC on it.
func qsvHEVCCapability() domaintranscoding.EncodeCapability {
	routes := []string{domaintranscoding.HWRouteSDR, domaintranscoding.HWRouteHEVC, domaintranscoding.HWRouteSource10Bit, domaintranscoding.HWRouteDecoupled}
	return domaintranscoding.EncodeCapability{
		SoftwareMaxHeight: 1080, SoftwareHDRMaxHeight: 720, HWMaxHeight: 2160,
		ProbeVersion: domaintranscoding.CapabilityProbeVersion,
		HWBackend:    "qsv", HWDevice: "/dev/dri/renderD128", HWCodecs: []string{"h264", "hevc"},
		HWDecodeCodecs: []string{"h264", "hevc", "hevc10"}, HWRoutes: routes,
		Backends: []domaintranscoding.HWBackendCap{{
			Backend: "qsv", Device: "/dev/dri/renderD128", Codecs: []string{"h264", "hevc"},
			DecodeCodecs: []string{"h264", "hevc", "hevc10"}, Routes: routes,
		}},
	}
}

// A series episode, mobile, 2026-09-15: the hybrid master's fallback rung read
// CODECS="avc1…" while ffmpeg ran hevc_qsv under the very hash the master
// sealed - the HEVC-capable phone dropped from an H.264 copy onto an HEVC
// transcode and froze. The rung must (a) keep the copy rung's codec family and
// (b) advertise exactly what the plan its URL resolves to will encode.
func TestBuildHybridMaster_FallbackRung_KeepsCopyCodec_OnHEVCCapableGPUAndClient(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true
	h.hwAccelEnabled = func() bool { return true }
	h.capability = func() domaintranscoding.HostCapabilitySnapshot { return (qsvHEVCCapability()).Snapshot() }

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 12_000_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams, planFor(t, h, onDemandReq(), item, streams))
	require.NoError(t, err)
	var fbInf, fbURI string
	lines := strings.Split(master, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "stream_0.m3u8") {
			fbURI, fbInf = line, lines[i-1]
		}
	}
	require.NotEmpty(t, fbURI, "premise: the fallback rung is advertised")
	assert.Contains(t, fbURI, "vc=h264", "the rung pins the copy rung's codec")
	assert.Contains(t, fbInf, `CODECS="avc1.`, "the rung advertises the copy rung's codec family: %s", fbInf)

	// The URL re-resolved at segment time must encode what the master said.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/"+fbURI, nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
	fp, err := h.buildFingerprint(req, "m1", item, streams)
	require.NoError(t, err)
	assert.Equal(t, "h264", fp.VideoCodec, "the encoder's codec follows the advertised one")
	assert.Equal(t, "qsv", fp.HWAccel, "the pin changes the codec, not the GPU route")

	// Same client, same GPU, no pin: the natural transcode IS an HEVC one - the
	// case the rung must not inherit.
	nat, err := h.buildFingerprint(func() *http.Request {
		r := onDemandReq()
		q := r.URL.Query()
		q.Set("vd", "transcode")
		r.URL.RawQuery = q.Encode()
		return r
	}(), "m1", item, streams)
	require.NoError(t, err)
	assert.Equal(t, "hevc", nat.VideoCodec, "premise: without the pin the transcode is upgraded to HEVC")
}

// hevcSDRAV / hevcHDRAV: copy-eligible sources for the ABR-compat gate tests.
// Both carry a keyframe index (copy needs it) + a re-encode-forced audio track
// (dts) so the hybrid master path is exercised exactly like TestBuildHybridMaster_FallbackRung.
func hevcSDRAV() []*domainlibrary.MediaStream {
	w, hgt, level := 1920, 1080, 120
	ch := 2
	return []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Profile: "Main", Width: &w, Height: &hgt, Level: &level, Keyframes: []int64{0, 6000, 12000}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "dts", Channels: &ch, IsDefault: true},
	}
}

func hevcHDRAV() []*domainlibrary.MediaStream {
	w, hgt, level := 3840, 2160, 153
	ch := 2
	pq := "smpte2084"
	return []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Profile: "Main 10", Width: &w, Height: &hgt, Level: &level, ColorTransfer: &pq, Keyframes: []int64{0, 6000, 12000}},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "dts", Channels: &ch, IsDefault: true},
	}
}

// Same codec family, same range: an HEVC(SDR) copy gets an HEVC(SDR) fallback
// rung (the vc= pin keeps the family, software x265 encodes it SDR). The player
// may auto-drop onto it - same ABR group, no decoder swap - so it IS advertised.
// The "HEVC copy -> HEVC fallback: switch normal" leg of the freeze fix.
func TestBuildHybridMaster_FallbackRung_HEVCSource_SameGroupAdvertised(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 12_000_000}}

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, hevcSDRAV(), planFor(t, h, onDemandReq(), item, hevcSDRAV()))
	require.NoError(t, err)
	assert.Contains(t, master, "od_playlist.m3u8")
	assert.Contains(t, master, "stream_0.m3u8",
		"HEVC-SDR copy + HEVC-SDR fallback are the same ABR group → fallback advertised")
}

// Different ABR group: an HEVC HDR (10-bit PQ) copy rung next to a tonemapped
// SDR 8-bit low transcode. Same-or-different codec, but the dynamic range/bit
// depth differ, so an auto-drop would reconfigure the decoder and freeze the
// picture (Lanterns-class). The rung MUST be omitted - the master stays
// single-variant and any drop is an explicit source handoff, never ABR.
// The "H.264/HDR copy -> incompatible fallback: never a direct ABR switch" leg.
func TestBuildHybridMaster_FallbackRung_SkippedOnIncompatibleGroup(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 60_000_000}}

	// hdr=1 so the HDR HEVC source is copy-eligible (raw PQ passthrough); the
	// low transcode fallback tonemaps to SDR → a different ABR group.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/media/m1/hls/master.m3u8?videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4&hdr=1", nil)
	req = req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{}, &domainuser.Claims{UserID: "u1", ProfileID: "p1"}))

	master, err := h.buildHybridMaster(req, "m1", item, hevcHDRAV(), planFor(t, h, req, item, hevcHDRAV()))
	require.NoError(t, err)
	assert.Contains(t, master, "od_playlist.m3u8", "the HDR copy variant is still served")
	assert.NotContains(t, master, "stream_0.m3u8",
		"HDR copy + SDR fallback are different ABR groups → fallback omitted (no auto codec/range swap)")
}

// The plan decided the packager; the source does not package (no Cues index).
// serveMaster serves the ffmpeg path for the SAME plan and says so: the plan
// is not rewritten, the fallback is counted by reason, and it is not a
// disagreement (the adapter re-decided nothing - the runtime could not obey).
func TestServeMaster_PackagerUnbuildable_ExplicitFallbackKeepsThePlan(t *testing.T) {
	item := onDemandItem()
	item.ID = "m1"
	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item.File.Duration = dur
	streams := copyEligibleAV()
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	h.onDemand = &fakeOnDemandPackager{err: fmt.Errorf("no Cues index")}
	var recorded domainplayback.ServingPath
	h.servingRecord = func(_ context.Context, _, _ string, p domainplayback.ServingPath) { recorded = p }

	// The plan the handler resolves for this request names the packager.
	req := onDemandReq()
	rp, err := h.probePlan(req, "m1", item, streams)
	require.NoError(t, err)
	require.Equal(t, apptranscoding.EngineMkvgo, rp.Plan.Delivery.Video, "precondition: the plan's video engine is the packager")

	fallbacks := func() int {
		return int(testutil.ToFloat64(metrics.HLSVODDeliveryFallbacks.WithLabelValues(fallbackPackagerUnbuildable)))
	}
	disagreements := func() int {
		return int(testutil.ToFloat64(metrics.HLSVODDeliveryDisagreements.WithLabelValues("copy/mkvgo+mkvgo", "transcode/ffmpeg+ffmpeg")))
	}
	before, beforeDis := fallbacks(), disagreements()

	w := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4")

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "od_", "the ffmpeg path served the master")
	assert.Equal(t, before+1, fallbacks(), "the fallback is counted by reason")
	assert.Equal(t, beforeDis, disagreements(), "a declared fallback is not a disagreement")
	assert.Equal(t, "ffmpeg", recorded.VideoEngine, "the admin sees what was really served")
	again, err := h.probePlan(req, "m1", item, streams)
	require.NoError(t, err)
	assert.Equal(t, rp.Plan.Delivery, again.Plan.Delivery, "the plan is not rewritten by the runtime outcome")
}

// End to end: a floor request on a packager-served title is served by the
// ffmpeg path because the PLAN says so (Delivery ffmpeg, Fit on the lowest
// rung) - the adapter re-pinned nothing, recorded what it served, and the
// disagreement counter (an invariant) did not move.
func TestServeMaster_LineFit_TheFFmpegPathServesTheFittedPlan(t *testing.T) {
	item := onDemandItem()
	item.ID = "m1"
	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item.File.Duration = dur
	item.File.Bitrate = 6_870_000
	streams := copyEligibleAV()
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	var recorded domainplayback.ServingPath
	h.servingRecord = func(_ context.Context, _, _ string, p domainplayback.ServingPath) { recorded = p }

	natural, err := h.probePlan(onDemandReq(), "m1", item, streams)
	require.NoError(t, err)
	require.Equal(t, apptranscoding.EngineMkvgo, natural.Plan.Delivery.Video, "precondition: without the floor the packager serves this title")

	disagreements := func() int {
		return int(testutil.ToFloat64(metrics.HLSVODDeliveryDisagreements.WithLabelValues("copy/mkvgo+mkvgo", "transcode/ffmpeg+ffmpeg")))
	}
	before := disagreements()
	w := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4&abr=floor")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Equal(t, "ffmpeg", w.Header().Get("X-Evey-Served-By"))
	assert.NotContains(t, body, "od_", "the fitted plan is ffmpeg's, not the packager's")
	assert.Equal(t, 1, strings.Count(body, "#EXT-X-STREAM-INF"), "mono-variant")
	assert.Contains(t, body, "stream_0.m3u8")
	assert.Contains(t, body, "abr=floor")
	assert.Equal(t, "transcode", recorded.VideoDecision)
	assert.Equal(t, "ffmpeg", recorded.VideoEngine)
	assert.Equal(t, before, disagreements(), "the plan named ffmpeg and ffmpeg served: no disagreement")
}

// An adapter serving an engine the plan did not name, with no fallback
// declared, is an invariant violation: counted on the disagreement counter.
func TestCheckDelivery_UndeclaredDifferenceIsADisagreement(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	rp := planFor(t, h, onDemandReq(), onDemandItem(), copyEligibleAV())
	require.Equal(t, apptranscoding.EngineMkvgo, rp.Plan.Delivery.Video)

	served := domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "ffmpeg", AudioDecision: "copy", AudioEngine: "ffmpeg"}
	counter := metrics.HLSVODDeliveryDisagreements.WithLabelValues("copy/mkvgo+mkvgo", "copy/ffmpeg+ffmpeg")
	before := int(testutil.ToFloat64(counter))
	h.checkDelivery(onDemandReq(), "m1", rp, served, "")
	assert.Equal(t, before+1, int(testutil.ToFloat64(counter)))

	// The same difference with a declared fallback is a runtime outcome, not a disagreement.
	h.checkDelivery(onDemandReq(), "m1", rp, served, fallbackPackagerUnbuildable)
	assert.Equal(t, before+1, int(testutil.ToFloat64(counter)))
	// And a served path equal to the plan's is neither.
	h.checkDelivery(onDemandReq(), "m1", rp, rp.Plan.Delivery.ServingPath(rp.Plan.Route, rp.FP), "")
	assert.Equal(t, before+1, int(testutil.ToFloat64(counter)))
}

// A file stored in one-track blocks plays through the packager alone (mkvgo
// 0.33 reads it track by track): its whole presentation is served, anything
// ffmpeg would read - a hybrid audio, a packager that cannot build it, the
// ffmpeg segments - keeps the typed re-mux refusal.
func TestServeMaster_BlockOrderedFileOnlyThroughThePackager(t *testing.T) {
	blockOrdered := func() *domainlibrary.MediaItem {
		it := onDemandItem()
		it.File.InterleaveBackstepMs = 540_000
		return it
	}
	master := func(h *hlsVODServe, streams []*domainlibrary.MediaStream) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.serveMaster(w, onDemandReq(), "m1", blockOrdered(), streams, noopSpan())
		return w
	}

	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{resources: map[string][]byte{"master.m3u8": []byte("#EXTM3U\nod_playlist.m3u8\n")}}}
	h.audioRenditionEnabled = true
	h.audio = &audioServe{}
	w := master(h, copyEligibleAV())
	assert.Equal(t, http.StatusOK, w.Code, "the packager's whole presentation")
	assert.Contains(t, w.Body.String(), "od_playlist.m3u8")

	dts := copyEligibleAV()
	dts[1].Codec = "dts"
	w = master(h, dts)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "hybrid: ffmpeg would read the audio")
	assert.Equal(t, ErrCodeFileNeedsRemux, w.Header().Get("X-Evey-Error"))

	broken := newCopyTestHandler(t)
	broken.onDemand = &fakeOnDemandPackager{err: fmt.Errorf("mp4: cannot plan")}
	w = master(broken, copyEligibleAV())
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "the packager fell back: ffmpeg would serve it")

	for _, f := range []string{"master.m3u8", "manifest.mpd", "od_seg00001.m4s", "od_audio1.m3u8", "key"} {
		assert.True(t, blockOrderedServable(f), f)
	}
	for _, f := range []string{"stream_0.m3u8", "stream_0_001.m4s", "audio_0.m3u8", "audio_0_001.m4s", "cc_playlist.m3u8"} {
		assert.False(t, blockOrderedServable(f), f)
	}
}
