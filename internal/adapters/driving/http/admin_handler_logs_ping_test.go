package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestSSEWritePing_WritesCommentFrame(t *testing.T) {
	w := httptest.NewRecorder()
	if err := sseWritePing(w); err != nil {
		t.Fatalf("sseWritePing returned error: %v", err)
	}
	if got := w.Body.String(); got != ": ping\n\n" {
		t.Errorf("expected \": ping\\n\\n\", got %q", got)
	}
}

func TestSSEWriteLine_WritesDataFrame(t *testing.T) {
	w := httptest.NewRecorder()
	if err := sseWriteLine(w, "hello"); err != nil {
		t.Fatalf("sseWriteLine returned error: %v", err)
	}
	if got := w.Body.String(); got != "data: hello\n\n" {
		t.Errorf("expected \"data: hello\\n\\n\", got %q", got)
	}
}
