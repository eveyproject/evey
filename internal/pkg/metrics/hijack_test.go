package metrics

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// hijackableRecorder prouve que statusRecorder relaie Hijack : sans ce relais,
// l'upgrade websocket de /api/v1/devices/ws panique en prod
// (`*metrics.statusRecorder is not http.Hijacker`) et l'enregistrement des
// devices (Play On) est mort pour tout client derrière InstrumentHandler.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked int
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked++
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, bufio.NewReadWriter(bufio.NewReader(c1), bufio.NewWriter(c1)), nil
}

func TestStatusRecorder_ImplementsHijacker(t *testing.T) {
	t.Parallel()
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	if _, ok := w.(http.Hijacker); !ok {
		t.Fatal("statusRecorder must implement http.Hijacker: the websocket upgrade type-asserts it and panics otherwise")
	}
}

func TestStatusRecorder_Hijack_ForwardsToHijacker(t *testing.T) {
	t.Parallel()
	inner := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}
	conn, rw, err := rec.Hijack()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn == nil || rw == nil {
		t.Fatal("want a live conn and buffered readwriter from the inner hijacker")
	}
	_ = conn.Close()
	if inner.hijacked != 1 {
		t.Fatalf("want 1 hijack forwarded, got %d", inner.hijacked)
	}
}

func TestStatusRecorder_Hijack_NoHijacker_ReturnsError(t *testing.T) {
	t.Parallel()
	rec := &statusRecorder{ResponseWriter: &nonFlushRW{header: make(http.Header)}, status: http.StatusOK}
	_, _, err := rec.Hijack()
	if err == nil {
		t.Fatal("want an error when the underlying writer cannot hijack, got nil")
	}
	if errors.Is(err, http.ErrHijacked) {
		t.Fatalf("want ErrNotSupported semantics, got ErrHijacked: %v", err)
	}
}
