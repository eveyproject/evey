package httpapi

// stream_access_log.go - one log line per direct-play file request.
//
// The HLS routes log what they serve; the direct-play route logged nothing, so
// a phone reading the raw file was invisible server-side: on 2026-09-27 two
// phones played an E-AC-3 film without sound and the only server trace was a
// StartStream followed by progress reports. The line says which client read
// which byte range, with what outcome.

import (
	"io"
	"log/slog"
	"net/http"
	"time"
)

// streamAccessRecorder notes the status and the bytes of a response while
// keeping the optional interfaces ServeFile relies on (io.ReaderFrom for the
// sendfile path, http.Flusher, Unwrap for http.ResponseController).
type streamAccessRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (s *streamAccessRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *streamAccessRecorder) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(p)
	s.bytes += int64(n)
	return n, err
}

func (s *streamAccessRecorder) ReadFrom(src io.Reader) (int64, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	if rf, ok := s.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(src)
		s.bytes += n
		return n, err
	}
	return io.Copy(struct{ io.Writer }{s}, src) // s.Write counts
}

func (s *streamAccessRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *streamAccessRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// serveFileLogged serves path like http.ServeFile and logs the request once it
// ends: a direct-play request lasts as long as the client keeps reading.
func serveFileLogged(w http.ResponseWriter, r *http.Request, mediaID, path string) {
	rec := &streamAccessRecorder{ResponseWriter: w}
	start := time.Now()
	http.ServeFile(rec, r, path)
	var userID string
	if claims := HTTPClaimsFromContext(r.Context()); claims != nil {
		userID = claims.UserID
	} else if st, ok := StreamTokenFromContext(r.Context()); ok && st != nil {
		userID = string(st.UserID)
	}
	slog.InfoContext(r.Context(), "stream: direct play served",
		slog.String("component", "stream"),
		slog.String("media_id", mediaID),
		slog.String("user_id", userID),
		slog.String("method", r.Method),
		slog.String("range", r.Header.Get("Range")),
		slog.Int("status", rec.status),
		slog.Int64("bytes", rec.bytes),
		slog.Int64("ms", time.Since(start).Milliseconds()),
		slog.String("user_agent", clampString(r.UserAgent(), playerEventMaxDetail)),
	)
}
