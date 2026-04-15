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

// NOTE: The notification → checkAndGenerateTokens path is not tested here
// because `checker *qcCheckerAdapter` is a concrete type (not an interface),
// so we cannot fake it without refactoring quickconnect_sse.go (out of scope).
//
// Likewise the 2m10s timeout branch is not tested because the timeout
// duration is hardcoded — testing it would require either a 130s sleep
// or a refactor to inject the timeout.
