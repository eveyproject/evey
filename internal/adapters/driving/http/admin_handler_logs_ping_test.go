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

// The log search's numeric query params: blank or junk fall back to the
// default, negatives too, and a runaway value is clamped to the ceiling.
func TestParseLogSearchInt(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int
	}{
		{"", 50}, {"abc", 50}, {"-1", 50}, {"0", 0}, {"25", 25}, {"500", 500}, {"501", 500},
	} {
		if got := parseLogSearchInt(tc.raw, 50, 500); got != tc.want {
			t.Errorf("parseLogSearchInt(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}
