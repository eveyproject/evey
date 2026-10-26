package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// deadlineRecorder is a ResponseRecorder that records every write deadline
// http.ResponseController sets on it.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	mu        sync.Mutex
	deadlines []time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.mu.Lock()
	d.deadlines = append(d.deadlines, t)
	d.mu.Unlock()
	return nil
}

// A segment write must be bounded by a deadline that slides with each write, so
// a client that stops reading releases the handler instead of pinning it, and
// the deadline is cleared afterwards so the next request on the connection is
// not cut by it.
func TestServeSegmentFile_SlidingWriteDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream_0_000.m4s")
	body := bytes.Repeat([]byte{0x42}, 256<<10)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodGet, "/seg", nil)

	before := time.Now()
	serveSegmentFile(rec, req, path)

	if !bytes.Equal(rec.Body.Bytes(), body) {
		t.Fatalf("served %d bytes, want %d", rec.Body.Len(), len(body))
	}
	if len(rec.deadlines) < 3 {
		t.Fatalf("got %d deadline updates, want one per write plus the final clear", len(rec.deadlines))
	}
	for _, d := range rec.deadlines[:len(rec.deadlines)-1] {
		if d.IsZero() || d.Before(before) || d.After(time.Now().Add(segmentWriteIdleTimeout)) {
			t.Fatalf("write deadline %v outside [now, now+%v]", d, segmentWriteIdleTimeout)
		}
	}
	if last := rec.deadlines[len(rec.deadlines)-1]; !last.IsZero() {
		t.Fatalf("deadline left at %v after the segment, want cleared", last)
	}
}
