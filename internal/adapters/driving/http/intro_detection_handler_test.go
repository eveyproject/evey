package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeAsyncDetector signals when DetectIntro is invoked so tests can wait on
// the detached goroutine without time.Sleep.
type fakeAsyncDetector struct {
	called chan string
	err    error
	panic  bool
}

func newFakeAsyncDetector() *fakeAsyncDetector {
	return &fakeAsyncDetector{called: make(chan string, 1)}
}

func (f *fakeAsyncDetector) DetectIntro(_ context.Context, mediaID string) error {
	if f.panic {
		panic("synthetic intro panic")
	}
	select {
	case f.called <- mediaID:
	default:
	}
	return f.err
}

func (f *fakeAsyncDetector) SetManualSegment(_ context.Context, _, _ string, _, _ int64) error {
	return nil
}
func (f *fakeAsyncDetector) DeleteSegment(_ context.Context, _, _ string) error { return nil }
func (f *fakeAsyncDetector) PropagateSegment(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

// --- tests ---

func TestIntroDetectionHandler_NoAuth_401(t *testing.T) {
	gen, _, _ := segAdminTokens(t)
	h := introDetectionHandler(gen, newFakeAsyncDetector())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/detect-segments", nil)
	req.SetPathValue("id", "m1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestIntroDetectionHandler_NonAdmin_403(t *testing.T) {
	gen, _, userTok := segAdminTokens(t)
	h := introDetectionHandler(gen, newFakeAsyncDetector())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/detect-segments", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestIntroDetectionHandler_MissingMediaID_400(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	h := introDetectionHandler(gen, newFakeAsyncDetector())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media//detect-segments", nil)
	// intentionally no SetPathValue
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestIntroDetectionHandler_Returns202_AndAsyncFires(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	detector := newFakeAsyncDetector()
	h := introDetectionHandler(gen, detector)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m42/detect-segments", nil)
	req.SetPathValue("id", "m42")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202 returned immediately, got %d", w.Code)
	}
	// Wait for the detached goroutine; bounded so we don't hang the suite.
	select {
	case got := <-detector.called:
		if got != "m42" {
			t.Errorf("expected DetectIntro to receive m42, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DetectIntro never called by detached goroutine")
	}
}

func TestIntroDetectionHandler_GoroutinePanicRecovered(t *testing.T) {
	// Ensure that a panic inside DetectIntro is recovered by the handler's
	// deferred recover and does NOT crash the test process.
	gen, adminTok, _ := segAdminTokens(t)
	detector := &fakeAsyncDetector{panic: true, called: make(chan string, 1)}
	h := introDetectionHandler(gen, detector)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/detect-segments", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// If the panic isn't recovered, this goroutine + test process die.
		h.ServeHTTP(w, req)
	}()
	wg.Wait()

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 even when async path panics, got %d", w.Code)
	}
	// Give detached goroutine a moment to panic+recover. The test passes if
	// the process is still alive (no panic propagated). 200ms is enough for
	// the goroutine to enter DetectIntro and panic; the recover handles the rest.
	time.Sleep(200 * time.Millisecond)
}

func TestIntroDetectionHandler_DetectErrorLogged(t *testing.T) {
	// When DetectIntro returns an error, the handler should still return 202
	// to the client (it's already returned) and log via slog. We can't easily
	// capture the log, but we verify the call happened and no panic.
	gen, adminTok, _ := segAdminTokens(t)
	detector := &fakeAsyncDetector{err: errors.New("synthetic failure"), called: make(chan string, 1)}
	h := introDetectionHandler(gen, detector)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m99/detect-segments", nil)
	req.SetPathValue("id", "m99")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	select {
	case got := <-detector.called:
		if got != "m99" {
			t.Errorf("expected DetectIntro on m99, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DetectIntro never called")
	}
}
