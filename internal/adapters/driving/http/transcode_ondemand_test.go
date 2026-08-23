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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
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
func (p *fakeOnDemandPlan) HasDASH() bool       { return p.dash }

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

// decisionFor mirrors tryServePlanMaster: one decision per request, handed to
// both eligibility gates.
func decisionFor(h *hlsVODServe, r *http.Request, item *domainlibrary.MediaItem, streams []*domainlibrary.MediaStream) domaintranscoding.PlaybackDecision {
	return h.makeDecision(r.Context(), streams, containerOf(item), sourcePathOf(item), r)
}

func TestPlanHLSVideoEligible(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	d := decisionFor(h, onDemandReq(), onDemandItem(), copyEligibleAV())
	assert.True(t, h.planHLSVideoEligible(d, onDemandItem(), copyEligibleAV()),
		"h264 copy video → video-eligible")

	// Feature not wired → never eligible (falls through to ffmpeg).
	h2 := newCopyTestHandler(t)
	d2 := decisionFor(h2, onDemandReq(), onDemandItem(), copyEligibleAV())
	assert.False(t, h2.planHLSVideoEligible(d2, onDemandItem(), copyEligibleAV()),
		"onDemand nil → not eligible")
}

func TestAudioAllCopyEligible(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	d := decisionFor(h, onDemandReq(), onDemandItem(), copyEligibleAV())
	assert.True(t, h.audioAllCopyEligible(context.Background(), d, copyEligibleAV()),
		"aac audio, copy decision → all-copy eligible (full packager path)")

	// A non-carriable track (dts) → not all-copy → hybrid path instead.
	streams := copyEligibleAV()
	streams[1].Codec = "dts"
	dDts := decisionFor(h, onDemandReq(), onDemandItem(), streams)
	assert.False(t, h.audioAllCopyEligible(context.Background(), dDts, streams),
		"a non-carriable audio track (dts) → hybrid (Evey re-encodes), not full copy")
}

func TestPlanHLSVideoEligible_NoKeyframes(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}

	streams := copyEligibleAV()
	streams[0].Keyframes = nil // not keyframe-backfilled → isCopyRequest is false
	d := decisionFor(h, onDemandReq(), onDemandItem(), streams)
	assert.False(t, h.planHLSVideoEligible(d, onDemandItem(), streams),
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

	served := h.tryServePlanMaster(httptest.NewRecorder(), onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), noopSpan())

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

	served := h.tryServePlanMaster(httptest.NewRecorder(), onDemandReq(), "m1", onDemandItem(), copyEligibleAV(), noopSpan())

	assert.False(t, served)
	assert.Equal(t, 0, act.ActiveSessionCount(), "a plan that never served must not count as playback")
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
	off, err := h.buildWasmAudioMaster(mkReq(), "m1", item, streams, true, true, 1)
	require.NoError(t, err)
	assert.Contains(t, off, `#EVEY-AUDIO-URI:"od_audio2.m3u8?audioTrack=1`, "untrimmed: k-th track is audio{k+1} and ?audioTrack rides along")

	// Trim ON: the plan keeps one track, renumbered to audio1, whatever the ordinal.
	h.onDemandTrackTrim = true
	on, err := h.buildWasmAudioMaster(mkReq(), "m1", item, streams, true, true, 1)
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
	master, err := h.buildWasmAudioMaster(mkReq(), "m1", item, streams, true, true, 1)
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
	master, err = h.buildWasmAudioMaster(mkReq(), "m1", item, streams, true, true, 1)
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
	_, err = h.buildPlan(onDemandReq(), "mi-1", item, copyEligibleAV())
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

	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV())
	require.NoError(t, err)
	assert.Len(t, pkg.lastOpts.AESKey, 16, "AES key threaded into the plan options")
	assert.Equal(t, "key", pkg.lastOpts.AESKeyURI)

	// Encryption off → no key in the options.
	pkg2 := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h2 := &hlsVODServe{onDemand: pkg2}
	_, err = h2.buildPlan(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV())
	require.NoError(t, err)
	assert.Empty(t, pkg2.lastOpts.AESKey, "no AES key when encryption is off")
}

// --- track trim (transcoding.hls_ondemand_track_trim) ---

func TestBuildPlan_TrackTrimOff_NoRestriction(t *testing.T) {
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, libSvc: &testFakeStreamsGetter{streams: copyEligibleAV()}}
	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV())
	require.NoError(t, err)
	assert.Nil(t, pkg.lastOpts.KeepAudioTrack, "flag off: today's untrimmed behaviour")
}

func TestBuildPlan_TrackTrimOn_ResolvesAudioTrack(t *testing.T) {
	streams := copyEligibleAV() // one audio track (ordinal 0)
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: true, libSvc: &testFakeStreamsGetter{streams: streams}}
	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), copyEligibleAV())
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

	_, err := h.buildPlan(req, "mi-1", onDemandItem(), streams)
	require.NoError(t, err)
	require.NotNil(t, pkg.lastOpts.KeepAudioTrack)
	assert.Equal(t, 1, *pkg.lastOpts.KeepAudioTrack, "explicit ?audioTrack= wins over the profile default, same as the ffmpeg pipeline")
}

func TestBuildPlan_TrackTrimOn_NoStreamsServesUntrimmed(t *testing.T) {
	// The caller normally passes commonAuth's streams; an empty slice (edge:
	// unprobed media) must serve every track, never refuse the plan.
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	h := &hlsVODServe{onDemand: pkg, onDemandTrackTrim: true}
	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), nil)
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

// The mkvgo packager rung must NOT inherit the ffmpeg copy rung's preconditions.
// planHLSVideoEligible funnelled through isCopyRequest, which additionally
// requires (a) a wired copyProducer and (b) enough free cache to hold a full
// copy of the source. mkvgo is STATELESS - it packages on the fly and writes
// ZERO bytes to the cache (verified: serving a 12 MB od_ segment left the cache
// dir at 0 entries). So a small cache, or an operator disabling the ffmpeg copy
// rung, silently pushed copy-eligible titles all the way to a TRANSCODE - the
// most expensive path - on exactly the weak-NAS boxes those guards exist to
// protect.
func TestPlanHLSVideoEligible_IndependentOfFFmpegCopyRung(t *testing.T) {
	h := newCopyTestHandler(t)
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{}}
	// Operator disabled the ffmpeg copy/remux rung (transcoding.remux_copy=off).
	// mkvgo needs nothing from it.
	h.copyProducer = nil

	d := decisionFor(h, onDemandReq(), onDemandItem(), copyEligibleAV())
	assert.True(t, h.planHLSVideoEligible(d, onDemandItem(), copyEligibleAV()),
		"mkvgo packages without ffmpeg: a disabled copy rung must not disable it")
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
	_, err := h.buildPlan(onDemandReq(), "mi-1", onDemandItem(), streams)
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

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams)
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

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams)
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

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams)
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

	master, err := h.buildHybridMaster(req, "m1", item, streams)
	require.NoError(t, err)
	assert.Contains(t, master, "od_playlist.m3u8", "the copy variant still serves")
	assert.NotContains(t, master, "stream_0.m3u8",
		"a cast master stays single-variant: the rung CAF never takes must not poison its MPL")
}

// The invariant that matters: the fallback URI the master emitted must be
// SERVEABLE - the variant request re-derives its plan from the baked query
// alone and the recomputed fingerprint must equal the sealed fp, or
// refuseStaleFP answers 410 and the rung is dead on arrival (the exact
// coherence failure the seal exists to catch).
// ?abr=floor is the survival mode the cast receiver's starvation rescue asks
// for: the announced rung exists but CAF never dives onto it by itself
// (measured on the Bouygtel4K, 2026-08-03, variant pinned through 9 minutes
// of freeze), so the receiver reloads with this param and the server hands it
// a MONO-VARIANT master holding only the lowest transcoded rung.
func TestFloorMaster_ServesOnlyTheLowestRung(t *testing.T) {
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
	q := req.URL.Query()
	q.Set("abr", "floor")
	req.URL.RawQuery = q.Encode()

	master, ok := h.buildFloorMaster(req, "m1", item, streams)
	require.True(t, ok, "floor must resolve on a fat copy-eligible source")
	assert.Equal(t, 1, strings.Count(master, "#EXT-X-STREAM-INF"), "floor is mono-variant by definition")
	assert.NotContains(t, master, "od_playlist.m3u8", "the copy variant must NOT be offered in survival mode")
	assert.Contains(t, master, "stream_0.m3u8")
	assert.Contains(t, master, "vd=transcode", "the rung pins its plan")
	assert.Contains(t, master, "fp=", "the rung seals its fingerprint")
}

// Incident the owner 2026-08-23 : le cast a gele a tampon PLEIN sur un flux
// copie de 6,87 Mbps, et l'analyse a conclu que "la box n'a nulle part ou
// retomber". C'est vrai du MASTER (copyFallbackVariant renvoie nil pour tout
// recepteur cast, cf. TestBuildHybridMaster_FallbackRung_SkippedForCast) mais
// FAUX du serveur : la sortie de secours ?abr=floor existe et repond a un
// jeton cast comme a n'importe quel autre. Ce qui manquait n'etait pas la
// porte, c'etait quelqu'un pour frapper - la veille de famine du recepteur ne
// voit ni un tampon plein ni une position qui avance a 1,000x (mesure prod :
// 19:33:38 pos=332,653 -> 19:34:08 pos=340,021, buf=30/20 constant, et ZERO
// occurrence de "floor" dans les logs de la soiree).
func TestFloorMaster_StaysAvailableToACastReceiver(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 6_870_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"

	req := onDemandReq()
	q := req.URL.Query()
	q.Set("abr", "floor")
	req.URL.RawQuery = q.Encode()
	req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey,
		&shared.StreamToken{UserID: "u1", ProfileID: "p1", MediaItemID: "m1", Scope: shared.ScopeStream, Cast: true}))

	master, ok := h.buildFloorMaster(req, "m1", item, streams)
	require.True(t, ok, "un recepteur cast doit obtenir le barreau de survie quand il le demande")
	assert.Equal(t, 1, strings.Count(master, "#EXT-X-STREAM-INF"), "floor is mono-variant by definition")
	assert.NotContains(t, master, "od_playlist.m3u8", "le barreau copie ne doit pas etre offert en mode survie")
	assert.Contains(t, master, "vd=transcode")
}

// The variant URI must carry its baked query exactly ONCE. buildCappedMaster
// pre-suffixed VideoPlaylistURI and the manifest generator suffixed it again:
// prod (2026-08-05) emitted "...videoCodecs=h264?ad=transcode&..." - a second
// "?" mid-query - and every segment URI of the child playlist inherited the
// garbage "videoCodecs=h264%3Fad%3Dtranscode". It happened to still serve
// because the fp seal replays the sealed plan and the polluted param is
// ignored today; the first param that DOES enter the fp derivation turns this
// into a silent 410 on the whole rescue path.
func TestCappedMaster_VariantURICarriesItsQueryOnce(t *testing.T) {
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
	q := req.URL.Query()
	q.Set("abr", "floor")
	req.URL.RawQuery = q.Encode()

	master, ok := h.buildFloorMaster(req, "m1", item, streams)
	require.True(t, ok)
	for line := range strings.SplitSeq(master, "\n") {
		if !strings.HasPrefix(line, "stream_0.m3u8") {
			continue
		}
		assert.Equal(t, 1, strings.Count(line, "?"), "one URI, one query: %s", line)
		assert.Equal(t, 1, strings.Count(line, "vd=transcode"), "the baked suffix must not double: %s", line)
	}
}

// ?maxKbps=<measured> is the receiver's own bitrate measurement (its network
// stack included - old cast runtimes cap well under the wire speed, proven on
// the Bouygtel4K 2026-08-03). Copy-first stays the rule: the cap only bites
// when the measure cannot carry the copy, and then the server picks the
// HIGHEST rung that fits - mono-variant, so the receiver's own ABR (the
// atrophied muscle) is never needed.
func TestCappedMaster_CopyFirstWhenTheMeasureCarriesIt(t *testing.T) {
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
	q := req.URL.Query()
	q.Set("maxKbps", "12000")
	req.URL.RawQuery = q.Encode()

	_, ok := h.buildCappedMaster(req, "m1", item, streams, 12000)
	assert.False(t, ok, "a measure that carries the copy with margin must fall through to the ordinary master")
}

func TestCappedMaster_PicksTheHighestFittingRung(t *testing.T) {
	h := newCopyTestHandler(t)
	h.manifestGen = apptranscoding.NewManifestGenerator()
	h.audioRenditionEnabled = true
	h.copyFallbackRung = true

	dur, err := shared.NewDurationMs(60000)
	require.NoError(t, err)
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{
		Path: "/m.mkv", Size: 1 << 20, Duration: dur, Bitrate: 7_400_000,
	}}
	streams := copyEligibleAV()
	streams[1].Codec = "dts"

	const maxKbps = 6000
	req := onDemandReq()
	q := req.URL.Query()
	q.Set("maxKbps", "6000")
	req.URL.RawQuery = q.Encode()

	// Expected rung: the same rule the implementation must apply, computed
	// from the same natural transcode plan - no hardcoded ladder.
	rT := req.Clone(req.Context())
	qq := rT.URL.Query()
	qq.Set("vd", "transcode")
	rT.URL.RawQuery = qq.Encode()
	rpT, err := h.resolvePlan(rT, "m1", item, streams)
	require.NoError(t, err)
	require.NotEmpty(t, rpT.Profiles)
	want := 0
	for i, p := range rpT.Profiles {
		if (p.VideoBitrate+p.AudioBitrate)*12 <= maxKbps*10 {
			want = i
		}
	}

	master, ok := h.buildCappedMaster(req, "m1", item, streams, maxKbps)
	require.True(t, ok, "a 7.4 Mbps copy cannot ride a 6 Mbps measure - the cap must serve a rung")
	assert.Equal(t, 1, strings.Count(master, "#EXT-X-STREAM-INF"), "capped master is mono-variant")
	assert.Contains(t, master, fmt.Sprintf("stream_%d.m3u8", want),
		"the variant URI must carry the LADDER index of the chosen rung, or the fp seal serves the wrong profile")
	assert.NotContains(t, master, "od_playlist.m3u8")
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

// A lean source has no rung meaningfully below the copy: floor falls back to
// the ordinary master shapes rather than serving a pointless sidegrade.
func TestFloorMaster_RefusedOnLeanSource(t *testing.T) {
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

	req := onDemandReq()
	q := req.URL.Query()
	q.Set("abr", "floor")
	req.URL.RawQuery = q.Encode()

	_, ok := h.buildFloorMaster(req, "m1", item, streams)
	assert.False(t, ok)
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

	master, err := h.buildHybridMaster(onDemandReq(), "m1", item, streams)
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
