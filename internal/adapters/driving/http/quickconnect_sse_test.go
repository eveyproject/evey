package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/pkg/quickconnecthub"
)

// syncRecorder is a goroutine-safe ResponseWriter+Flusher for SSE tests.
type syncRecorder struct {
	mu   sync.Mutex
	h    http.Header
	code int
	buf  strings.Builder
}

func newSyncRecorder() *syncRecorder {
	return &syncRecorder{h: make(http.Header), code: http.StatusOK}
}

func (r *syncRecorder) Header() http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.h
}
func (r *syncRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}
func (r *syncRecorder) WriteHeader(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.code = code
}
func (r *syncRecorder) Flush() {}
func (r *syncRecorder) body() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}
func (r *syncRecorder) getHeader(k string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.h.Get(k)
}
func (r *syncRecorder) status() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.code
}

func TestQuickConnectSSE_MissingCode(t *testing.T) {
	hub := quickconnecthub.New()
	h := quickConnectSSEHandler(hub, nil)

	// No router → PathValue("code") returns "".
	req := httptest.NewRequest(http.MethodGet, "/api/quickconnect/sse/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing code") {
		t.Fatalf("expected 'missing code' in body, got %q", rec.Body.String())
	}
}

// nonFlusherWriter wraps ResponseWriter without implementing http.Flusher.
type nonFlusherWriter struct {
	h    http.Header
	code int
	buf  strings.Builder
}

func (w *nonFlusherWriter) Header() http.Header {
	if w.h == nil {
		w.h = make(http.Header)
	}
	return w.h
}
func (w *nonFlusherWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *nonFlusherWriter) WriteHeader(code int)        { w.code = code }

func TestQuickConnectSSE_StreamingUnsupported(t *testing.T) {
	hub := quickconnecthub.New()
	h := quickConnectSSEHandler(hub, nil)

	mux := http.NewServeMux()
	mux.Handle("/qc/{code}", h)

	req := httptest.NewRequest(http.MethodGet, "/qc/abc123", nil)
	w := &nonFlusherWriter{}
	mux.ServeHTTP(w, req)

	if w.code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.code)
	}
	if !strings.Contains(w.buf.String(), "streaming unsupported") {
		t.Fatalf("expected 'streaming unsupported', got %q", w.buf.String())
	}
}

func TestQuickConnectSSE_InitialPingAndContextCancel(t *testing.T) {
	hub := quickconnecthub.New()
	h := quickConnectSSEHandler(hub, nil)

	mux := http.NewServeMux()
	mux.Handle("/qc/{code}", h)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/qc/abc123", nil).WithContext(ctx)
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.body(), ": ping") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after context cancel")
	}

	if !strings.Contains(rec.body(), ": ping") {
		t.Fatalf("expected initial ping in body, got %q", rec.body())
	}
	if ct := rec.getHeader("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("want text/event-stream, got %q", ct)
	}
	if cc := rec.getHeader("Cache-Control"); cc != "no-cache" {
		t.Fatalf("want no-cache, got %q", cc)
	}
}

func TestQuickConnectSSE_UnsubscribesOnExit(t *testing.T) {
	hub := quickconnecthub.New()
	h := quickConnectSSEHandler(hub, nil)

	mux := http.NewServeMux()
	mux.Handle("/qc/{code}", h)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/qc/zzz", nil).WithContext(ctx)
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.body(), ": ping") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-done

	// Notify should not block or panic on an unsubscribed code.
	hub.Notify("zzz")
}

// fakeChecker implements qcTokenChecker for tests.
type fakeChecker struct {
	mu         sync.Mutex
	access     string
	refresh    string
	authorized bool
	err        error
	calls      int
	lastCode   string
	lastPersis bool
}

func (f *fakeChecker) checkAndGenerateTokens(_ context.Context, code string, persistent bool) (string, string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastCode = code
	f.lastPersis = persistent
	return f.access, f.refresh, f.authorized, f.err
}

// Notify path: authorized → SSE emits access+refresh tokens then exits.
func TestQuickConnectSSE_NotifyAuthorized(t *testing.T) {
	hub := quickconnecthub.New()
	checker := &fakeChecker{access: "acc", refresh: "ref", authorized: true}
	h := quickConnectSSEHandler(hub, checker)

	mux := http.NewServeMux()
	mux.Handle("/qc/{code}", h)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/qc/code123", nil).WithContext(ctx)
	req.Header.Set("X-Persistent-Session", "true")
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.body(), ": ping") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	hub.Notify("code123")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after notify+authorized")
	}

	body := rec.body()
	if !strings.Contains(body, `"event":"authorized"`) {
		t.Fatalf("expected authorized event, got %q", body)
	}
	if !strings.Contains(body, `"accessToken":"acc"`) {
		t.Fatalf("expected accessToken, got %q", body)
	}
	if !strings.Contains(body, `"refreshToken":"ref"`) {
		t.Fatalf("expected refreshToken, got %q", body)
	}
	checker.mu.Lock()
	if checker.calls != 1 || checker.lastCode != "code123" || !checker.lastPersis {
		t.Fatalf("unexpected checker state: %+v", checker)
	}
	checker.mu.Unlock()
}

// Notify path: error or not-authorized → SSE emits error event then exits.
func TestQuickConnectSSE_NotifyNotAuthorized(t *testing.T) {
	hub := quickconnecthub.New()
	checker := &fakeChecker{authorized: false}
	h := quickConnectSSEHandler(hub, checker)

	mux := http.NewServeMux()
	mux.Handle("/qc/{code}", h)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/qc/codex", nil).WithContext(ctx)
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(rec, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.body(), ": ping") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	hub.Notify("codex")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after notify+unauthorized")
	}

	if !strings.Contains(rec.body(), `"event":"error"`) {
		t.Fatalf("expected error event, got %q", rec.body())
	}
}

// Concurrent subscribers on the same code: all receive the notify without deadlock.
// Run with -race to catch data races.
func TestQuickConnectSSE_ConcurrentSubscribersSameCode(t *testing.T) {
	hub := quickconnecthub.New()
	checker := &fakeChecker{access: "a", refresh: "r", authorized: true}
	h := quickConnectSSEHandler(hub, checker)

	mux := http.NewServeMux()
	mux.Handle("/qc/{code}", h)

	const subs = 10
	var wg sync.WaitGroup
	recs := make([]*syncRecorder, subs)
	ctxs := make([]context.CancelFunc, subs)

	for i := 0; i < subs; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = cancel
		rec := newSyncRecorder()
		recs[i] = rec
		req := httptest.NewRequest(http.MethodGet, "/qc/shared", nil).WithContext(ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			mux.ServeHTTP(rec, req)
		}()
	}
	defer func() {
		for _, c := range ctxs {
			c()
		}
	}()

	// Wait for all subs to emit initial ping (proves they're subscribed).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ready := 0
		for _, r := range recs {
			if strings.Contains(r.body(), ": ping") {
				ready++
			}
		}
		if ready == subs {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	hub.Notify("shared")

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(3 * time.Second):
		t.Fatal("not all subscribers exited after notify")
	}

	for i, r := range recs {
		if !strings.Contains(r.body(), `"event":"authorized"`) {
			t.Errorf("sub %d missing authorized event: %q", i, r.body())
		}
	}
}
