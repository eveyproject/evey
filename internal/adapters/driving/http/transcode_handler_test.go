package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

func TestToPlaybackInfoDTO_PreservesWireContract(t *testing.T) {
	d := &domaintranscoding.PlaybackDecision{
		PlayMethod: "transcode", VideoDecision: "transcode", AudioDecision: "copy",
		SubtitleDecision: "none", TranscodeReasons: []string{"video_codec_unsupported"},
		VideoCodec: "h264", AudioCodec: "aac", Container: "mp4",
		VideoBitrate: 4000, AudioBitrate: 192, MaxWidth: 1920, MaxHeight: 1080, MaxAudioChannels: 2,
	}
	b, err := json.Marshal(toPlaybackInfoDTO(d))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// The DTO must keep the historical snake_case keys the endpoint shipped.
	for _, k := range []string{"play_method", "video_decision", "audio_decision", "subtitle_decision", "transcode_reasons", "video_codec", "audio_codec", "container", "video_bitrate", "audio_bitrate", "max_width", "max_height", "max_audio_channels"} {
		if _, ok := got[k]; !ok {
			t.Errorf("playback-info JSON missing key %q", k)
		}
	}
	if got["play_method"] != "transcode" || got["video_codec"] != "h264" {
		t.Errorf("DTO values not mapped: %v", got)
	}
}

func newTranscodeServe(libSvc MediaStreamsGetter, la LibraryAccessChecker) http.HandlerFunc {
	return transcodeHandler(transcodeServe{
		libSvc:        libSvc,
		baseURL:       "http://localhost",
		libraryAccess: la,
		throttle:      apptranscoding.NewTranscodeThrottle(),
	})
}

func TestTranscodeServe_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	inner := newTranscodeServe(&fakeMediaStreamsGetter{}, allowAll())
	h := httpAuthMiddleware(verifier)(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTranscodeServe_InvalidToken_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	inner := newTranscodeServe(&fakeMediaStreamsGetter{}, allowAll())
	h := httpAuthMiddleware(verifier)(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTranscodeHandler_MediaNotFound_404(t *testing.T) {
	h := newTranscodeServe(
		&fakeMediaStreamsGetter{itemErr: errTest},
		allowAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTranscodeHandler_IDOR_Forbidden(t *testing.T) {
	h := newTranscodeServe(
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "restricted", File: &domainlibrary.MediaFile{Path: "/a"}}},
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPlaybackInfoHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	inner := playbackInfoHandler(&fakeMediaStreamsGetter{}, allowAll(), nil, nil)
	h := httpAuthMiddleware(verifier)(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlaybackInfoHandler_MediaNotFound_404(t *testing.T) {
	h := playbackInfoHandler(
		&fakeMediaStreamsGetter{itemErr: errTest},
		allowAll(),
		nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
