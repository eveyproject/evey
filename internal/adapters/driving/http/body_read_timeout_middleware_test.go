package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// slowBody yields its bytes with a pause partway through, simulating a client
// that dribbles its POST body — the shape BodyReadTimeoutMiddleware defends
// against. It implements io.Reader over an httptest request body.
type slowBody struct {
	chunks [][]byte
	pause  time.Duration
	i      int
}

func (s *slowBody) Read(p []byte) (int, error) {
	if s.i > 0 {
		time.Sleep(s.pause)
	}
	if s.i >= len(s.chunks) {
		return 0, io.EOF
	}
	n := copy(p, s.chunks[s.i])
	s.i++
	return n, nil
}

// serverReadErr spins up a real server (deadlines need a real net.Conn, not
// httptest.NewRecorder) wrapping the handler in bodyReadTimeoutMiddleware with a
// short deadline, POSTs a body that pauses past that deadline, and reports the
// error the handler saw while reading the body.
func serverReadErr(t *testing.T, wrap func(http.Handler) http.Handler, pause time.Duration) error {
	t.Helper()
	readErr := make(chan error, 1)
	h := wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readErr <- err
		w.WriteHeader(http.StatusOK)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := &slowBody{chunks: [][]byte{[]byte("first"), []byte("second")}, pause: pause}
	req, err := http.NewRequest(http.MethodPost, srv.URL, body)
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = -1 // chunked, so the server reads incrementally
	resp, err := srv.Client().Do(req)
	if err == nil {
		resp.Body.Close()
	}
	select {
	case e := <-readErr:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("handler never returned")
		return nil
	}
}

// A slow POST body must be cut off by the read deadline.
func TestBodyReadTimeout_SlowPostIsCutOff(t *testing.T) {
	err := serverReadErr(t, func(next http.Handler) http.Handler {
		return bodyReadTimeoutMiddleware(next, 100*time.Millisecond)
	}, 400*time.Millisecond)
	if err == nil {
		t.Fatal("expected a read error (deadline exceeded), got nil")
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) && !isTimeout(err) {
		t.Fatalf("expected a deadline/timeout error, got %v", err)
	}
}

// ClearBodyReadDeadline (as upload handlers call) lets a slow body read fully.
func TestBodyReadTimeout_ClearedBodyReadsFully(t *testing.T) {
	err := serverReadErr(t, func(next http.Handler) http.Handler {
		return bodyReadTimeoutMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ClearBodyReadDeadline(w)
			next.ServeHTTP(w, r)
		}), 100*time.Millisecond)
	}, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("cleared deadline must allow a slow body, got %v", err)
	}
}

func isTimeout(err error) bool {
	var te interface{ Timeout() bool }
	return errors.As(err, &te) && te.Timeout()
}
