package netmeter

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddleware_CountsWrites(t *testing.T) {
	var m Meter
	h := m.Middleware(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 1000))
		_, _ = w.Write(bytes.Repeat([]byte("y"), 24))
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/seg.m4s", nil))
	if got := m.Total(); got != 1024 {
		t.Errorf("Total = %d, want 1024", got)
	}
}

// httptest.ResponseRecorder is an io.ReaderFrom via its bytes.Buffer? It is
// not - so this exercises the fallback copy path; the counting must still be
// exact.
func TestCountingWriter_ReadFromFallback(t *testing.T) {
	var m Meter
	rec := httptest.NewRecorder()
	cw := &countingWriter{ResponseWriter: rec, meter: &m}
	n, err := cw.ReadFrom(strings.NewReader(strings.Repeat("z", 4096)))
	if err != nil || n != 4096 {
		t.Fatalf("ReadFrom = (%d, %v), want (4096, nil)", n, err)
	}
	if got := m.Total(); got != 4096 {
		t.Errorf("Total = %d, want 4096", got)
	}
	if rec.Body.Len() != 4096 {
		t.Errorf("body = %d bytes, want 4096", rec.Body.Len())
	}
}

type flushRecorder struct {
	httptest.ResponseRecorder
	flushed bool
}

func (f *flushRecorder) Flush() { f.flushed = true }

func TestCountingWriter_ForwardsFlush(t *testing.T) {
	var m Meter
	rec := &flushRecorder{}
	cw := &countingWriter{ResponseWriter: rec, meter: &m}
	cw.Flush()
	if !rec.flushed {
		t.Error("Flush must reach the underlying writer (HLS progressive writes)")
	}
}

func TestNilMeter_MiddlewareIsPassthrough(t *testing.T) {
	var m *Meter
	called := false
	h := m.Middleware(func(http.ResponseWriter, *http.Request) { called = true })
	h(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Error("nil meter must pass through")
	}
}

type readerFromRecorder struct {
	*httptest.ResponseRecorder
	readFromCalls int
}

func (r *readerFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.readFromCalls++
	return io.Copy(r.ResponseRecorder, src)
}

// http.ServeContent / io.Copy reach for ReadFrom when the underlying writer
// offers it (sendfile): the bytes must still be metered, exactly once, and the
// fast path must actually be taken rather than degraded to Write.
func TestCountingWriter_ReadFromFastPathIsMeteredOnce(t *testing.T) {
	m := &Meter{}
	rec := &readerFromRecorder{ResponseRecorder: httptest.NewRecorder()}
	h := m.Middleware(func(w http.ResponseWriter, _ *http.Request) {
		n, err := w.(io.ReaderFrom).ReadFrom(strings.NewReader(strings.Repeat("x", 1000)))
		if err != nil || n != 1000 {
			t.Fatalf("ReadFrom = %d, %v", n, err)
		}
	})
	h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if m.Total() != 1000 {
		t.Fatalf("metered %d bytes, want 1000", m.Total())
	}
	if rec.readFromCalls != 1 {
		t.Fatalf("fast path taken %d times, want 1", rec.readFromCalls)
	}
	if rec.Body.Len() != 1000 {
		t.Fatalf("body = %d bytes", rec.Body.Len())
	}
}

// http.ResponseController reaches the real writer through Unwrap (deadlines,
// hijack): the meter must not hide it.
func TestCountingWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	var got http.ResponseWriter
	(&Meter{}).Middleware(func(w http.ResponseWriter, _ *http.Request) {
		got = w.(interface{ Unwrap() http.ResponseWriter }).Unwrap()
	})(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got != rec {
		t.Fatal("Unwrap must hand back the wrapped writer")
	}
}
