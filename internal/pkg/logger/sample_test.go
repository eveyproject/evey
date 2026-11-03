package logger

import (
	"testing"
	"time"
)

// One line per key and per window goes through; the next window's line
// carries the count of the ones held back, and keys do not share a budget.
func TestSampler_OneLinePerKeyPerWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	s := NewSamplerForTest(time.Minute, func() time.Time { return now })

	if ok, held := s.Allow("a"); !ok || held != 0 {
		t.Fatalf("first line: allowed=%v held=%d, want allowed with nothing held", ok, held)
	}
	for range 3 {
		if ok, _ := s.Allow("a"); ok {
			t.Fatal("a line inside the window must be held back")
		}
	}
	if ok, held := s.Allow("b"); !ok || held != 0 {
		t.Fatalf("another key: allowed=%v held=%d, want its own first line", ok, held)
	}

	now = now.Add(time.Minute)
	if ok, held := s.Allow("a"); !ok || held != 3 {
		t.Fatalf("next window: allowed=%v held=%d, want allowed with 3 held", ok, held)
	}
	if ok, held := s.Allow("a"); ok || held != 0 {
		t.Fatalf("right after: allowed=%v held=%d, want held back again", ok, held)
	}
}
