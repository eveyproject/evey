package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// fakeAsyncDetector signals when DetectIntro is invoked so tests can wait on
// the detached goroutine without time.Sleep.
type fakeAsyncDetector struct {
	called       chan string
	seasonCalled chan string
	seriesCalled chan string
	err          error
	panic        bool
}

func newFakeAsyncDetector() *fakeAsyncDetector {
	return &fakeAsyncDetector{
		called:       make(chan string, 1),
		seasonCalled: make(chan string, 1),
		seriesCalled: make(chan string, 1),
	}
}

func (f *fakeAsyncDetector) DetectSeason(_ context.Context, seasonID string) error {
	select {
	case f.seasonCalled <- seasonID:
	default:
	}
	return f.err
}

func (f *fakeAsyncDetector) DetectSeries(_ context.Context, seriesID string) (int, error) {
	select {
	case f.seriesCalled <- seriesID:
	default:
	}
	return 1, f.err
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

func (f *fakeAsyncDetector) DetectIntroDryRun(_ context.Context, _ string) ([]domainlibrary.MediaSegment, error) {
	return nil, nil
}
func (f *fakeAsyncDetector) BackfillLibrary(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (f *fakeAsyncDetector) SetManualSegment(_ context.Context, _, _ string, _, _ int64) error {
	return nil
}
func (f *fakeAsyncDetector) DeleteSegment(_ context.Context, _, _ string) error { return nil }
func (f *fakeAsyncDetector) PropagateSegment(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

// newIntroDetectionHandler builds the handler under test with a resolvable
// media item (mi-1 in lib-1) and a nil access checker: the route is
// admin-gated and checkLibraryAccess short-circuits for admins, so the
// checker is only consulted by the (impossible today) non-admin path.
func newIntroDetectionHandler(gen TokenVerifier, det IntroDetectionTrigger) http.HandlerFunc {
	libSvc := &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{
		ID:        shared.MediaItemID("m42"),
		LibraryID: shared.LibraryID("lib-1"),
	}}
	return introDetectionHandler(gen, det, libSvc, nil)
}

// --- tests ---

func TestIntroDetectionHandler_NoAuth_401(t *testing.T) {
	gen, _, _ := segAdminTokens(t)
	h := newIntroDetectionHandler(gen, newFakeAsyncDetector())
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
	h := newIntroDetectionHandler(gen, newFakeAsyncDetector())
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
	h := newIntroDetectionHandler(gen, newFakeAsyncDetector())
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
	h := newIntroDetectionHandler(gen, detector)
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
	h := newIntroDetectionHandler(gen, detector)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/m1/detect-segments", nil)
	req.SetPathValue("id", "m1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()

	var wg sync.WaitGroup
	wg.Go(func() {
		// If the panic isn't recovered, this goroutine + test process die.
		h.ServeHTTP(w, req)
	})
	wg.Wait()

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202 even when async path panics, got %d", w.Code)
	}
	// Give detached goroutine a moment to panic+recover. The test passes if
	// the process is still alive (no panic propagated). 200ms is enough for
	// the goroutine to enter DetectIntro and panic; the recover handles the rest.
	time.Sleep(200 * time.Millisecond)
}

// The admin intro-test tool targets seasons and series directly: a season id
// must run the SEASON pipeline (cluster included), not the per-episode
// DetectIntro that a season has no file for.
func TestIntroDetectionHandler_SeasonID_RunsSeasonDetection(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	detector := newFakeAsyncDetector()
	libSvc := &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{
		ID: "season-1", LibraryID: "lib-1", Type: domainlibrary.Season,
	}}
	h := introDetectionHandler(gen, detector, libSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/season-1/detect-segments", nil)
	req.SetPathValue("id", "season-1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	select {
	case got := <-detector.seasonCalled:
		if got != "season-1" {
			t.Errorf("expected DetectSeason on season-1, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DetectSeason never called")
	}
	select {
	case got := <-detector.called:
		t.Fatalf("DetectIntro must not fire for a season id, got %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestIntroDetectionHandler_SeriesID_RunsSeriesDetection(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	detector := newFakeAsyncDetector()
	libSvc := &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{
		ID: "series-1", LibraryID: "lib-1", Type: domainlibrary.Series,
	}}
	h := introDetectionHandler(gen, detector, libSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/series-1/detect-segments", nil)
	req.SetPathValue("id", "series-1")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	select {
	case got := <-detector.seriesCalled:
		if got != "series-1" {
			t.Errorf("expected DetectSeries on series-1, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DetectSeries never called")
	}
}

// Dry-run stays file-scoped: a container has no file to analyse, and the UI
// samples a concrete episode itself. 400, not a misleading empty result.
func TestIntroDetectionHandler_DryRunOnContainer_400(t *testing.T) {
	gen, adminTok, _ := segAdminTokens(t)
	for _, typ := range []domainlibrary.MediaItemType{domainlibrary.Season, domainlibrary.Series} {
		libSvc := &fakeMediaStreamsGetter{item: &domainlibrary.MediaItem{
			ID: "c-1", LibraryID: "lib-1", Type: typ,
		}}
		h := introDetectionHandler(gen, newFakeAsyncDetector(), libSvc, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/c-1/detect-segments?dry_run=true", nil)
		req.SetPathValue("id", "c-1")
		req.Header.Set("Authorization", "Bearer "+adminTok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: dry-run on a container must 400, got %d", typ, w.Code)
		}
	}
}

func TestIntroDetectionHandler_UnknownMedia_404(t *testing.T) {
	// An id that resolves to no media item must be a 404 up front, not a
	// fire-and-forget goroutine that fails later in the logs.
	gen, adminTok, _ := segAdminTokens(t)
	detector := newFakeAsyncDetector()
	libSvc := &fakeMediaStreamsGetter{itemErr: errors.New("not found")}
	h := introDetectionHandler(gen, detector, libSvc, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/mi-ghost/detect-segments", nil)
	req.SetPathValue("id", "mi-ghost")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown media, got %d", w.Code)
	}
	select {
	case got := <-detector.called:
		t.Fatalf("DetectIntro must not fire for an unknown id, got %q", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestIntroDetectionHandler_DetectErrorLogged(t *testing.T) {
	// When DetectIntro returns an error, the handler should still return 202
	// to the client (it's already returned) and log via slog. We can't easily
	// capture the log, but we verify the call happened and no panic.
	gen, adminTok, _ := segAdminTokens(t)
	detector := &fakeAsyncDetector{err: errors.New("synthetic failure"), called: make(chan string, 1)}
	h := newIntroDetectionHandler(gen, detector)
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
