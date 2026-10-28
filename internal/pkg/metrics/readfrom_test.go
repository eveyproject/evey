package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// readerFromRecorder stands for net/http's response writer, whose ReadFrom
// is the sendfile path.
type readerFromRecorder struct {
	*httptest.ResponseRecorder
	readFromCalls int
}

func (r *readerFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.readFromCalls++
	return io.Copy(r.ResponseRecorder, src)
}

// A file served behind InstrumentHandler must still reach the writer's
// ReadFrom (sendfile): io.Copy only finds it on the wrapper it is handed.
func TestStatusRecorder_ForwardsReadFrom(t *testing.T) {
	inner := &readerFromRecorder{ResponseRecorder: httptest.NewRecorder()}
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: inner, status: http.StatusOK}
	if _, ok := w.(io.ReaderFrom); !ok {
		t.Fatal("statusRecorder must implement io.ReaderFrom")
	}
	// ServeContent copies through an io.LimitedReader, which has no WriteTo.
	n, err := io.Copy(w, &io.LimitedReader{R: strings.NewReader("payload"), N: 7})
	if err != nil || n != 7 {
		t.Fatalf("copy: %d %v", n, err)
	}
	if inner.readFromCalls != 1 {
		t.Fatalf("ReadFrom reached the inner writer %d times, want 1", inner.readFromCalls)
	}
	if got := inner.Body.String(); got != "payload" {
		t.Fatalf("body %q", got)
	}

	plain := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: plain, status: http.StatusOK}
	if _, err := rec.ReadFrom(strings.NewReader("x")); err != nil || plain.Body.String() != "x" {
		t.Fatalf("fallback copy: %q %v", plain.Body.String(), err)
	}
}
