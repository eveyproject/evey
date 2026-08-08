package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// --- shared helpers ---

// ============================================================
// HLS cache tests
// ============================================================

func TestCachedLibraryAccessChecker_CacheHit(t *testing.T) {
	inner := allowAll()
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	ok, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err != nil || !ok {
		t.Fatalf("expected access, got ok=%v err=%v", ok, err)
	}
	// second call with inner changed to deny - should still return cached true
	inner.allowed = false
	ok2, _ := cached.HasAccess(context.Background(), "u1", "lib1")
	if !ok2 {
		t.Error("expected cached access=true")
	}
}

func TestCachedLibraryAccessChecker_DeniedCacheHit(t *testing.T) {
	inner := denyAll()
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	ok, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err != nil || ok {
		t.Fatalf("expected denied, got ok=%v err=%v", ok, err)
	}
}

func TestCachedLibraryAccessChecker_Error(t *testing.T) {
	inner := &fakeLibraryAccessChecker{err: errors.New("db error")}
	cached := NewCachedLibraryAccessChecker(inner)
	defer cached.Close()

	_, err := cached.HasAccess(context.Background(), "u1", "lib1")
	if err == nil {
		t.Error("expected error")
	}
}

// claimsFromContext tests removed (share_handler migrated to gRPC)

// ============================================================
// transcode parseParams tests
// ============================================================

func TestTranscodeParseParams_Default(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := h.parseParams(req)
	if p.audioTrack != 0 {
		t.Errorf("expected audioTrack=0, got %d", p.audioTrack)
	}
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0, got %d", p.videoScale)
	}
	if p.subtitleTrack != -1 {
		t.Errorf("expected subtitleTrack=-1, got %d", p.subtitleTrack)
	}
	if p.remuxOnly {
		t.Error("expected remuxOnly=false")
	}
	if p.startTimeSec != 0 {
		t.Errorf("expected startTimeSec=0, got %f", p.startTimeSec)
	}
}

func TestTranscodeParseParams_AllSet(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?audioTrack=2&videoScale=720&subtitleTrack=1&remux=1&startTime=30.5", nil)
	p := h.parseParams(req)
	if p.audioTrack != 2 {
		t.Errorf("expected audioTrack=2, got %d", p.audioTrack)
	}
	if p.videoScale != 720 {
		t.Errorf("expected videoScale=720, got %d", p.videoScale)
	}
	if p.subtitleTrack != 1 {
		t.Errorf("expected subtitleTrack=1, got %d", p.subtitleTrack)
	}
	if !p.remuxOnly {
		t.Error("expected remuxOnly=true")
	}
	if p.startTimeSec != 30.5 {
		t.Errorf("expected startTimeSec=30.5, got %f", p.startTimeSec)
	}
}

func TestTranscodeParseParams_InvalidValues(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?audioTrack=abc&videoScale=-1&subtitleTrack=-5&startTime=bad", nil)
	p := h.parseParams(req)
	if p.audioTrack != 0 {
		t.Errorf("expected audioTrack=0 for invalid, got %d", p.audioTrack)
	}
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0 for invalid, got %d", p.videoScale)
	}
	if p.subtitleTrack != -1 {
		t.Errorf("expected subtitleTrack=-1 for invalid, got %d", p.subtitleTrack)
	}
}

func TestTranscodeParseParams_VideoScaleTooHigh(t *testing.T) {
	h := &transcodeServe{}
	req := httptest.NewRequest(http.MethodGet, "/test?videoScale=9999", nil)
	p := h.parseParams(req)
	if p.videoScale != 0 {
		t.Errorf("expected videoScale=0 for >4320, got %d", p.videoScale)
	}
}

// ============================================================
// transcode analyzeStreams tests
// ============================================================

func TestAnalyzeStreams_H264_AAC_DirectPlay(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if a.needsVideoTranscode {
		t.Error("h264 should not need video transcode")
	}
	if a.needsAudioTranscode {
		t.Error("aac should not need audio transcode")
	}
}

func TestAnalyzeStreams_HEVC_NeedsTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if !a.needsVideoTranscode {
		t.Error("hevc should need video transcode")
	}
}

func TestAnalyzeStreams_RemuxOnly_NoVideoTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "aac", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", true, domaintranscoding.DefaultWebProfile, -1)
	if a.needsVideoTranscode {
		t.Error("remux mode should not trigger video transcode")
	}
}

func TestAnalyzeStreams_NonAACAudio_NeedsTranscode(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "dts", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if !a.needsAudioTranscode {
		t.Error("dts should need audio transcode")
	}
}

func TestAnalyzeStreams_OpusAudio_NoCopy(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "opus", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if a.needsAudioTranscode {
		t.Error("opus should not need audio transcode")
	}
}

func TestAnalyzeStreams_MP3Audio_NoCopy(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "h264"},
			{StreamType: "audio", Codec: "mp3", IsDefault: true},
		},
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if a.needsAudioTranscode {
		t.Error("mp3 should not need audio transcode")
	}
}

func TestAnalyzeStreams_StreamsError(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		sErr: errors.New("db error"),
	}}
	a := h.analyzeStreams(context.Background(), "m1", false, domaintranscoding.DefaultWebProfile, -1)
	if a.streams != nil {
		t.Error("expected nil streams on error")
	}
	if !a.needsAudioTranscode {
		t.Error("default should be needsAudioTranscode=true")
	}
}

func TestAnalyzeStreams_HEVC_EAC3_Firefox_TranscodeBoth(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "eac3", IsDefault: true},
		},
	}}
	firefox := domaintranscoding.ClientProfile{
		VideoCodecs: []string{"h264", "vp9"},
		AudioCodecs: []string{"aac", "mp3", "opus", "flac"},
	}
	a := h.analyzeStreams(context.Background(), "m1", false, firefox, -1)
	if !a.needsVideoTranscode {
		t.Error("HEVC should need transcode on Firefox")
	}
	if !a.needsAudioTranscode {
		t.Error("EAC3 should need transcode on Firefox")
	}
}

func TestAnalyzeStreams_HEVC_EAC3_Safari_CopyBoth(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "eac3", IsDefault: true},
		},
	}}
	safari := domaintranscoding.ClientProfile{
		VideoCodecs: []string{"h264", "hevc", "vp9"},
		AudioCodecs: []string{"aac", "mp3", "opus", "flac", "ac3", "eac3"},
	}
	a := h.analyzeStreams(context.Background(), "m1", false, safari, -1)
	if a.needsVideoTranscode {
		t.Error("HEVC should copy on Safari")
	}
	if a.needsAudioTranscode {
		t.Error("EAC3 should copy on Safari")
	}
}

func TestAnalyzeStreams_HEVC_EAC3_PartialSupport(t *testing.T) {
	h := &transcodeServe{libSvc: &fakeMediaStreamsGetter{
		item: testMediaItem("lib1"),
		streams: []*domainlibrary.MediaStream{
			{StreamType: "video", Codec: "hevc"},
			{StreamType: "audio", Codec: "eac3", IsDefault: true},
		},
	}}
	client := domaintranscoding.ClientProfile{
		VideoCodecs: []string{"h264", "hevc"},
		AudioCodecs: []string{"aac", "mp3", "opus"},
	}
	a := h.analyzeStreams(context.Background(), "m1", false, client, -1)
	if a.needsVideoTranscode {
		t.Error("HEVC should copy when client supports it")
	}
	if !a.needsAudioTranscode {
		t.Error("EAC3 should transcode when client doesn't support it")
	}
}

func TestHDRQuerySupported_AcceptsBothClientForms(t *testing.T) {
	cases := map[string]bool{"/t?hdr=1": true, "/t?hdr=true": true, "/t?hdr=0": false, "/t?hdr=false": false, "/t": false}
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if got := hdrQuerySupported(req); got != want {
			t.Errorf("hdrQuerySupported(%q) = %v, want %v", target, got, want)
		}
	}
}

// ============================================================
// Subtitle list with items
// ============================================================

// ============================================================
// HLS segment handler auth tests
// ============================================================

// HLSSegmentHandler_QueryToken skipped - requires non-nil HLSManager to avoid panic after auth

// ============================================================
// Transcode handler auth tests
// ============================================================

func TestTranscodeHandler_NoAuth_401(t *testing.T) {
	v := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(v)(transcodeHandler(transcodeServe{
		libSvc: &fakeMediaStreamsGetter{}, baseURL: "http://localhost", libraryAccess: allowAll(),
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTranscodeHandler_InvalidToken_401(t *testing.T) {
	v := &fakeTokenVerifier{err: errTest}
	h := httpAuthMiddleware(v)(transcodeHandler(transcodeServe{
		libSvc: &fakeMediaStreamsGetter{}, baseURL: "http://localhost", libraryAccess: allowAll(),
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTranscodeHandler_NotFound_404(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		libSvc:        &fakeMediaStreamsGetter{itemErr: errTest},
		baseURL:       "http://localhost",
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTranscodeHandler_IDOR_403(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		libSvc:        &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib-x", File: &domainlibrary.MediaFile{Path: "/a"}}},
		baseURL:       "http://localhost",
		libraryAccess: denyAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTranscodeHandler_QueryToken(t *testing.T) {
	h := transcodeHandler(transcodeServe{
		libSvc:        &fakeMediaStreamsGetter{itemErr: errTest},
		baseURL:       "http://localhost",
		libraryAccess: allowAll(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth should pass with query token, got %d", w.Code)
	}
}

func TestTranscodeHandler_MaxStreams_429(t *testing.T) {
	throttle := apptranscoding.NewTranscodeThrottle()
	maxStreams := 1
	fakeMaxStreams := &fakeUserMaxStreams{max: &maxStreams}
	throttle.Acquire("u1", 0) // occupy one slot
	h := transcodeHandler(transcodeServe{
		libSvc:        &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/a"}}},
		baseURL:       "http://localhost",
		libraryAccess: allowAll(), throttle: throttle, userMaxStreams: fakeMaxStreams,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	throttle.Release("u1")
}

func TestBuildFFmpegArgs_VideoTranscode(t *testing.T) {
	h := &transcodeServe{}
	args := h.buildFFmpegArgs("/video.mkv", transcodeParams{subtitleTrack: -1}, transcodeAnalysis{needsVideoTranscode: true, needsAudioTranscode: true})
	hasLibx264 := false
	hasAAC := false
	for _, a := range args {
		if a == "libx264" {
			hasLibx264 = true
		}
		if a == "aac" {
			hasAAC = true
		}
	}
	if !hasLibx264 {
		t.Error("expected libx264 for video transcode")
	}
	if !hasAAC {
		t.Error("expected aac for audio transcode")
	}
}

// startTime is the only numeric query parameter on this path without an upper
// bound: audioTrack and subtitleTrack are clamped to the streams that exist,
// videoScale is bounded to 4320. strconv.ParseFloat accepts the literals "Inf",
// "inf" and "+Inf" WITHOUT an error, and +Inf passes a bare `f > 0`, so the
// value reached transcode_ffmpeg.go and was formatted into the ffmpeg command
// as `-ss +Inf`, which ffmpeg rejects - turning a nonsense query parameter into
// a failed transcode instead of a seek to the start.
//
// "1e400" is the near miss that already worked: ParseFloat returns +Inf there
// too, but WITH an out-of-range error, so the existing err check caught it.
func TestTranscodeParseParams_RejectsInfiniteStartTime(t *testing.T) {
	for _, raw := range []string{"Inf", "inf", "+Inf", "INF", "infinity"} {
		t.Run(raw, func(t *testing.T) {
			h := &transcodeServe{}
			req := httptest.NewRequest(http.MethodGet, "/test?startTime="+raw, nil)
			if got := h.parseParams(req).startTimeSec; got != 0 {
				t.Errorf("startTime=%q accepted as %v, want the 0 fallback", raw, got)
			}
		})
	}
}

func TestTranscodeParseParams_KeepsOrdinaryStartTimes(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want float64
	}{
		{"30.5", 30.5},
		{"0.001", 0.001},
		{"7200", 7200},
		// Rejected for other reasons, listed so a future bound change has to
		// state what it does to them.
		{"0", 0},
		{"-5", 0},
		{"NaN", 0},
		{"1e400", 0},
		{"abc", 0},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			h := &transcodeServe{}
			req := httptest.NewRequest(http.MethodGet, "/test?startTime="+tc.raw, nil)
			if got := h.parseParams(req).startTimeSec; got != tc.want {
				t.Errorf("startTime=%q -> %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
