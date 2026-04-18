package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

func newTranscodeServe(tv TokenVerifier, libSvc MediaStreamsGetter, la LibraryAccessChecker) http.HandlerFunc {
	return transcodeHandler(transcodeServe{
		tokens:        tv,
		libSvc:        libSvc,
		baseURL:       "http://localhost",
		libraryAccess: la,
		throttle:      apptranscoding.NewTranscodeThrottle(),
	})
}

func TestTranscodeServe_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	inner := newTranscodeServe(verifier, &fakeMediaStreamsGetter{}, allowAll())
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
	inner := newTranscodeServe(verifier, &fakeMediaStreamsGetter{}, allowAll())
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
		&fakeTokenVerifier{claims: testClaims(false)},
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{ID: "m1", LibraryID: "restricted", File: &domainlibrary.MediaFile{Path: "/a"}}},
		denyAll(),
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/transcode", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestPlaybackInfoHandler_NoAuth_401(t *testing.T) {
	verifier := &fakeTokenVerifier{err: errTest}
	inner := playbackInfoHandler(verifier, &fakeMediaStreamsGetter{}, allowAll(), nil)
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
		&fakeTokenVerifier{claims: testClaims(false)},
		&fakeMediaStreamsGetter{itemErr: errTest},
		allowAll(),
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/playback-info", nil)
	req.SetPathValue("id", "m1")
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
