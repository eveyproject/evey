package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// flushable writer used to prove statusRecorder.Flush forwards.
type flushableRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (f *flushableRecorder) Flush() {
	f.flushed++
	f.ResponseRecorder.Flush()
}

func TestStatusRecorder_Flush_ForwardsToFlusher(t *testing.T) {
	t.Parallel()
	inner := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}
	rec.Flush()
	if inner.flushed != 1 {
		t.Fatalf("want 1 flush, got %d", inner.flushed)
	}
}

func TestStatusRecorder_Flush_NoFlusher_NoPanic(t *testing.T) {
	t.Parallel()
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	// httptest.ResponseRecorder implements Flusher too - use a minimal writer.
	nonFlusher := &nonFlushRW{header: make(http.Header)}
	rec.ResponseWriter = nonFlusher
	rec.Flush() // must not panic
}

type nonFlushRW struct {
	header http.Header
	code   int
	body   []byte
}

func (w *nonFlushRW) Header() http.Header         { return w.header }
func (w *nonFlushRW) Write(p []byte) (int, error) { w.body = append(w.body, p...); return len(p), nil }
func (w *nonFlushRW) WriteHeader(c int)           { w.code = c }
