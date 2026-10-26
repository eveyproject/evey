package httpapi

import (
	"net/http"
	"time"
)

// segmentWriteIdleTimeout bounds how long one write of a segment may block on
// a client that stopped reading. The server has no WriteTimeout (long streams
// need none), so without it a stalled client pins the handler and its file.
const segmentWriteIdleTimeout = 30 * time.Second

// serveSegmentFile is http.ServeFile with a write deadline pushed forward on
// every write: a slow but reading client is never cut, a stalled one is.
func serveSegmentFile(w http.ResponseWriter, r *http.Request, path string) {
	rc := http.NewResponseController(w)
	// Cleared afterwards: a deadline left on a keep-alive connection would cut
	// whatever the next request on it streams.
	defer func() { _ = rc.SetWriteDeadline(time.Time{}) }()
	http.ServeFile(&slidingDeadlineWriter{ResponseWriter: w, rc: rc}, r, path)
}

type slidingDeadlineWriter struct {
	http.ResponseWriter
	rc *http.ResponseController
}

func (s *slidingDeadlineWriter) Write(b []byte) (int, error) {
	// ErrNotSupported (a writer with no deadline control) just serves unbounded, as before.
	_ = s.rc.SetWriteDeadline(time.Now().Add(segmentWriteIdleTimeout))
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the connection underneath.
func (s *slidingDeadlineWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
