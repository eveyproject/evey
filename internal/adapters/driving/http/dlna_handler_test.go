package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appdlna "github.com/eveyproject/evey/internal/app/dlna"
)

type fakeDLNAClient struct {
	renderers   []appdlna.Renderer
	discoverErr error
	playErr     error
	stopErr     error
	pauseErr    error

	lastRendererID string
	lastMediaURL   string
	lastTitle      string
}

func (f *fakeDLNAClient) Discover(_ context.Context) ([]appdlna.Renderer, error) {
	return f.renderers, f.discoverErr
}

func (f *fakeDLNAClient) Play(_ context.Context, rendererID, mediaURL, title string) error {
	f.lastRendererID = rendererID
	f.lastMediaURL = mediaURL
	f.lastTitle = title
	return f.playErr
}

func (f *fakeDLNAClient) Stop(_ context.Context, rendererID string) error {
	f.lastRendererID = rendererID
	return f.stopErr
}

func (f *fakeDLNAClient) Pause(_ context.Context, rendererID string) error {
	f.lastRendererID = rendererID
	return f.pauseErr
}

func TestDLNARenderersHandler_OK(t *testing.T) {
	fake := &fakeDLNAClient{renderers: []appdlna.Renderer{{ID: "r1", Name: "Living Room"}}}
	h := dlnaRenderersHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/dlna/renderers", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"r1"`) {
		t.Errorf("body missing renderer id: %s", w.Body.String())
	}
}

func TestDLNARenderersHandler_NilReturnsEmptyArray(t *testing.T) {
	fake := &fakeDLNAClient{renderers: nil}
	h := dlnaRenderersHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/dlna/renderers", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("want [], got %q", body)
	}
}

func TestDLNARenderersHandler_Error(t *testing.T) {
	fake := &fakeDLNAClient{discoverErr: errors.New("boom")}
	h := dlnaRenderersHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/dlna/renderers", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestDLNAPlayHandler_OK(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaPlayHandler(fake)

	body := strings.NewReader(`{"rendererId":"r1","mediaUrl":"http://x/y.mp4","title":"T"}`)
	req := httptest.NewRequest(http.MethodPost, "/dlna/play", body)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.lastRendererID != "r1" || fake.lastMediaURL != "http://x/y.mp4" || fake.lastTitle != "T" {
		t.Errorf("unexpected forwarded args: %+v", fake)
	}
}

func TestDLNAPlayHandler_BadJSON(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaPlayHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/play", strings.NewReader(`not-json`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestDLNAPlayHandler_MissingFields(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaPlayHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/play", strings.NewReader(`{"rendererId":"r1"}`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestDLNAPlayHandler_ServiceError(t *testing.T) {
	fake := &fakeDLNAClient{playErr: errors.New("boom")}
	h := dlnaPlayHandler(fake)

	body := strings.NewReader(`{"rendererId":"r1","mediaUrl":"http://x/y.mp4"}`)
	req := httptest.NewRequest(http.MethodPost, "/dlna/play", body)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestDLNAStopHandler_OK(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaStopHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/stop", strings.NewReader(`{"rendererId":"r1"}`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if fake.lastRendererID != "r1" {
		t.Errorf("want r1, got %q", fake.lastRendererID)
	}
}

func TestDLNAStopHandler_BadJSON(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaStopHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/stop", strings.NewReader(`{`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestDLNAStopHandler_MissingRendererID(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaStopHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/stop", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestDLNAStopHandler_ServiceError(t *testing.T) {
	fake := &fakeDLNAClient{stopErr: errors.New("boom")}
	h := dlnaStopHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/stop", strings.NewReader(`{"rendererId":"r1"}`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestDLNAPauseHandler_OK(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaPauseHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/pause", strings.NewReader(`{"rendererId":"r1"}`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if fake.lastRendererID != "r1" {
		t.Errorf("want r1, got %q", fake.lastRendererID)
	}
}

func TestDLNAPauseHandler_BadJSON(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaPauseHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/pause", strings.NewReader(`{`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestDLNAPauseHandler_MissingRendererID(t *testing.T) {
	fake := &fakeDLNAClient{}
	h := dlnaPauseHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/pause", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestDLNAPauseHandler_ServiceError(t *testing.T) {
	fake := &fakeDLNAClient{pauseErr: errors.New("boom")}
	h := dlnaPauseHandler(fake)

	req := httptest.NewRequest(http.MethodPost, "/dlna/pause", strings.NewReader(`{"rendererId":"r1"}`))
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
