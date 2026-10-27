package httpapi

import (
	"io"
	"net/http"
	"time"
)

// segmentWriteIdleTimeout bounds how long one write of a segment may block on
// a client that stopped reading. The server has no WriteTimeout (long streams
// need none), so without it a stalled client pins the handler and its file.
const segmentWriteIdleTimeout = 30 * time.Second

// segmentWriteChunk is how much one ReadFrom hop may send before the deadline
// is pushed again: 256 KiB per 30 s only cuts a client under ~9 KB/s.
const segmentWriteChunk = 256 << 10

// serveSegmentFile is http.ServeFile with a write deadline pushed forward on
// every write: a slow but reading client is never cut, a stalled one is.
func serveSegmentFile(w http.ResponseWriter, r *http.Request, path string) {
	rc := http.NewResponseController(w)
	// Cleared afterwards: a deadline left on a keep-alive connection would cut
	// whatever the next request on it streams.
	defer func() { _ = rc.SetWriteDeadline(time.Time{}) }() //nolint:errcheck // no deadline control means none was set
	http.ServeFile(&slidingDeadlineWriter{ResponseWriter: w, rc: rc}, r, path)
}

type slidingDeadlineWriter struct {
	http.ResponseWriter
	rc *http.ResponseController
}

func (s *slidingDeadlineWriter) Write(b []byte) (int, error) {
	// ErrNotSupported (a writer with no deadline control) just serves unbounded, as before.
	_ = s.rc.SetWriteDeadline(time.Now().Add(segmentWriteIdleTimeout)) //nolint:errcheck // see above
	return s.ResponseWriter.Write(b)
}

// ReadFrom keeps the underlying writer's sendfile path, in chunks so the
// deadline still slides. The chunk wraps the file directly (one LimitedReader
// deep): sendfile only recognises that shape.
func (s *slidingDeadlineWriter) ReadFrom(src io.Reader) (int64, error) {
	rf, ok := s.ResponseWriter.(io.ReaderFrom)
	if !ok {
		return io.Copy(struct{ io.Writer }{s}, src)
	}
	base, remain := src, int64(-1)
	if lr, ok := src.(*io.LimitedReader); ok {
		base, remain = lr.R, lr.N
	}
	var total int64
	for remain != 0 {
		step := int64(segmentWriteChunk)
		if remain > 0 {
			step = min(step, remain)
		}
		_ = s.rc.SetWriteDeadline(time.Now().Add(segmentWriteIdleTimeout)) //nolint:errcheck // ErrNotSupported serves unbounded
		n, err := rf.ReadFrom(&io.LimitedReader{R: base, N: step})
		total += n
		if remain > 0 {
			remain -= n
		}
		if err != nil || n < step {
			if lr, ok := src.(*io.LimitedReader); ok {
				lr.N = remain
			}
			return total, err
		}
	}
	if lr, ok := src.(*io.LimitedReader); ok {
		lr.N = 0
	}
	return total, nil
}

// Unwrap lets http.ResponseController reach the connection underneath.
func (s *slidingDeadlineWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
