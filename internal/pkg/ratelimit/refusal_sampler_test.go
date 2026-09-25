package ratelimit

import (
	"testing"
	"time"
)

// A looping client wrote 580 884 refusal lines in two hours (prod,
// 2026-09-07). One line per (client, path) per window, the rest counted.
func TestRefusalSampler_OneLinePerWindowWithTheCount(t *testing.T) {
	s := &sampler{last: make(map[string]*sampleEntry)}
	t0 := time.Date(2026, 9, 7, 21, 32, 0, 0, time.UTC)

	suppressed, ok := s.admit("ip\x00/repairs", t0)
	if !ok || suppressed != 0 {
		t.Fatalf("first refusal: ok=%v suppressed=%d, want logged with 0", ok, suppressed)
	}
	for i := 1; i <= 999; i++ {
		if _, ok := s.admit("ip\x00/repairs", t0.Add(time.Duration(i)*50*time.Millisecond)); ok {
			t.Fatalf("refusal %d inside the window must not be logged", i)
		}
	}
	// Another client, or another path, is its own key.
	if _, ok := s.admit("ip2\x00/repairs", t0.Add(time.Second)); !ok {
		t.Fatal("a different client is logged on its first refusal")
	}
	if _, ok := s.admit("ip\x00/media", t0.Add(time.Second)); !ok {
		t.Fatal("a different path is logged on its first refusal")
	}
	// The window rolls: one line, carrying what it swallowed.
	suppressed, ok = s.admit("ip\x00/repairs", t0.Add(refusalSampleWindow))
	if !ok || suppressed != 999 {
		t.Fatalf("after the window: ok=%v suppressed=%d, want logged with 999", ok, suppressed)
	}
}

func TestRefusalSampler_TableIsBounded(t *testing.T) {
	s := &sampler{last: make(map[string]*sampleEntry)}
	now := time.Now()
	for i := 0; i < refusalSamplerMaxKeys+10; i++ {
		s.admit(string(rune('a'+i%26))+string(rune(i)), now)
	}
	if len(s.last) > refusalSamplerMaxKeys {
		t.Fatalf("table holds %d keys, cap is %d", len(s.last), refusalSamplerMaxKeys)
	}
}
