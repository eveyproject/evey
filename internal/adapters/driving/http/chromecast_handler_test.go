package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appcast "github.com/eveyproject/evey/internal/app/chromecast"
)

// fakeCastClient is a test double for castClient.
type fakeCastClient struct {
	devices []appcast.Device
	status  map[string]interface{}
	err     error

	lastUUID, lastURL, lastTitle, lastContentType string
	lastPosition                                  int
	played, stopped, paused, seeked               bool
}

func (f *fakeCastClient) Discover(_ context.Context, _ string) ([]appcast.Device, error) {
	return f.devices, f.err
}
func (f *fakeCastClient) Play(_ context.Context, uuid, url, title, ct string) error {
	f.played, f.lastUUID, f.lastURL, f.lastTitle, f.lastContentType = true, uuid, url, title, ct
	return f.err
}
func (f *fakeCastClient) Stop(_ context.Context, uuid string) error {
	f.stopped, f.lastUUID = true, uuid
	return f.err
}
func (f *fakeCastClient) Pause(_ context.Context, uuid string) error {
	f.paused, f.lastUUID = true, uuid
	return f.err
}
func (f *fakeCastClient) Seek(_ context.Context, uuid string, pos int) error {
	f.seeked, f.lastUUID, f.lastPosition = true, uuid, pos
	return f.err
}
func (f *fakeCastClient) Status(_ context.Context, uuid string) (map[string]interface{}, error) {
	f.lastUUID = uuid
	return f.status, f.err
}

func doCast(h http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

// --- castDevicesHandler ---

func TestCastDevicesHandler_Success(t *testing.T) {
	fc := &fakeCastClient{devices: []appcast.Device{{UUID: "u1", Name: "living"}}}
	w := doCast(castDevicesHandler(fc), http.MethodGet, "/cast/devices", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"u1"`) {
		t.Errorf("missing uuid in body: %s", w.Body.String())
	}
}

func TestCastDevicesHandler_NilReturnsEmptyArray(t *testing.T) {
	fc := &fakeCastClient{devices: nil}
	w := doCast(castDevicesHandler(fc), http.MethodGet, "/cast/devices", "")
	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("want [], got %q", body)
	}
}

func TestCastDevicesHandler_Error(t *testing.T) {
	fc := &fakeCastClient{err: errors.New("boom")}
	w := doCast(castDevicesHandler(fc), http.MethodGet, "/cast/devices", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("code=%d", w.Code)
	}
}

// --- castPlayHandler ---

func TestCastPlayHandler_Success(t *testing.T) {
	fc := &fakeCastClient{}
	body := `{"deviceUuid":"u1","mediaUrl":"http://x/m.mp4","title":"T","contentType":"video/mp4"}`
	w := doCast(castPlayHandler(fc), http.MethodPost, "/cast/play", body)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if !fc.played || fc.lastUUID != "u1" || fc.lastURL != "http://x/m.mp4" {
		t.Errorf("play not invoked with expected args: %+v", fc)
	}
}

func TestCastPlayHandler_BadJSON(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castPlayHandler(fc), http.MethodPost, "/cast/play", `{bad`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastPlayHandler_MissingFields(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castPlayHandler(fc), http.MethodPost, "/cast/play", `{"deviceUuid":"u1"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastPlayHandler_ServiceError(t *testing.T) {
	fc := &fakeCastClient{err: errors.New("boom")}
	body := `{"deviceUuid":"u1","mediaUrl":"http://x"}`
	w := doCast(castPlayHandler(fc), http.MethodPost, "/cast/play", body)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("code=%d", w.Code)
	}
}

// --- castStopHandler ---

func TestCastStopHandler_Success(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castStopHandler(fc), http.MethodPost, "/cast/stop", `{"deviceUuid":"u1"}`)
	if w.Code != http.StatusOK || !fc.stopped {
		t.Errorf("code=%d stopped=%v", w.Code, fc.stopped)
	}
}

func TestCastStopHandler_BadJSON(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castStopHandler(fc), http.MethodPost, "/cast/stop", `{bad`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastStopHandler_MissingUUID(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castStopHandler(fc), http.MethodPost, "/cast/stop", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastStopHandler_ServiceError(t *testing.T) {
	fc := &fakeCastClient{err: errors.New("boom")}
	w := doCast(castStopHandler(fc), http.MethodPost, "/cast/stop", `{"deviceUuid":"u1"}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("code=%d", w.Code)
	}
}

// --- castPauseHandler ---

func TestCastPauseHandler_Success(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castPauseHandler(fc), http.MethodPost, "/cast/pause", `{"deviceUuid":"u1"}`)
	if w.Code != http.StatusOK || !fc.paused {
		t.Errorf("code=%d paused=%v", w.Code, fc.paused)
	}
}

func TestCastPauseHandler_BadJSON(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castPauseHandler(fc), http.MethodPost, "/cast/pause", `{bad`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastPauseHandler_MissingUUID(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castPauseHandler(fc), http.MethodPost, "/cast/pause", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastPauseHandler_ServiceError(t *testing.T) {
	fc := &fakeCastClient{err: errors.New("boom")}
	w := doCast(castPauseHandler(fc), http.MethodPost, "/cast/pause", `{"deviceUuid":"u1"}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("code=%d", w.Code)
	}
}

// --- castSeekHandler ---

func TestCastSeekHandler_Success(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castSeekHandler(fc), http.MethodPost, "/cast/seek", `{"deviceUuid":"u1","positionSec":42}`)
	if w.Code != http.StatusOK || !fc.seeked || fc.lastPosition != 42 {
		t.Errorf("code=%d seeked=%v pos=%d", w.Code, fc.seeked, fc.lastPosition)
	}
}

func TestCastSeekHandler_BadJSON(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castSeekHandler(fc), http.MethodPost, "/cast/seek", `{bad`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastSeekHandler_MissingUUID(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castSeekHandler(fc), http.MethodPost, "/cast/seek", `{"positionSec":1}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastSeekHandler_ServiceError(t *testing.T) {
	fc := &fakeCastClient{err: errors.New("boom")}
	w := doCast(castSeekHandler(fc), http.MethodPost, "/cast/seek", `{"deviceUuid":"u1","positionSec":1}`)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("code=%d", w.Code)
	}
}

// --- castStatusHandler ---

func TestCastStatusHandler_Success(t *testing.T) {
	fc := &fakeCastClient{status: map[string]interface{}{"state": "PLAYING"}}
	w := doCast(castStatusHandler(fc), http.MethodGet, "/cast/status?deviceUuid=u1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"PLAYING"`) {
		t.Errorf("missing state: %s", w.Body.String())
	}
	if fc.lastUUID != "u1" {
		t.Errorf("uuid=%s", fc.lastUUID)
	}
}

func TestCastStatusHandler_MissingUUID(t *testing.T) {
	fc := &fakeCastClient{}
	w := doCast(castStatusHandler(fc), http.MethodGet, "/cast/status", "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("code=%d", w.Code)
	}
}

func TestCastStatusHandler_ServiceError(t *testing.T) {
	fc := &fakeCastClient{err: errors.New("boom")}
	w := doCast(castStatusHandler(fc), http.MethodGet, "/cast/status?deviceUuid=u1", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("code=%d", w.Code)
	}
}

// compile-time check: *appcast.Service satisfies castClient.
var _ castClient = (*appcast.Service)(nil)
