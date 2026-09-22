package library

import (
	"testing"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// Mechanical contracts of the pHash helpers: window bounds, degenerate
// inputs, the Hamming tolerance and its exact-match fallback.

func TestFrameCutoff(t *testing.T) {
	m := &domainlibrary.TrickplayManifest{Interval: 15, TileWidth: 10, TileHeight: 10, ImageCount: 3}
	// 60 min at one frame per 15 s = 240 frames; a quarter of them.
	if got := frameCutoff(m, 3_600_000, 0.25); got != 60 {
		t.Errorf("25%% of a 60 min episode = %d frames, want 60", got)
	}
	// Out-of-range percentages fall back to the default quarter.
	for _, pct := range []float64{0, -1, 1.5} {
		if got := frameCutoff(m, 3_600_000, pct); got != 60 {
			t.Errorf("cutoff %v: got %d, want the default 60", pct, got)
		}
	}
	if got := frameCutoff(m, 3_600_000, 1); got != 240 {
		t.Errorf("100%%: got %d, want 240", got)
	}
	// Without a usable interval or duration the whole sprite set is searched.
	if got := frameCutoff(&domainlibrary.TrickplayManifest{TileWidth: 10, TileHeight: 10, ImageCount: 3}, 3_600_000, 0.25); got != 300 {
		t.Errorf("no interval: got %d, want every tile (300)", got)
	}
	if got := frameCutoff(m, 0, 0.25); got != 300 {
		t.Errorf("no duration: got %d, want every tile (300)", got)
	}
}

func TestHammingDist(t *testing.T) {
	cases := []struct {
		a, b uint64
		want int
	}{
		{0, 0, 0},
		{0, 1, 1},
		{0xFF, 0, 8},
		{0xFFFFFFFFFFFFFFFF, 0, 64},
		{0b1010, 0b0101, 4},
	}
	for _, c := range cases {
		if got := hammingDist(c.a, c.b); got != c.want {
			t.Errorf("hammingDist(%x, %x) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestFrequencyMapHamming(t *testing.T) {
	if got := FrequencyMapHamming(nil, 0.5, 8); got != nil {
		t.Errorf("no episodes: got %v, want nil", got)
	}
	// Candidates come from the FIRST episode; a hash within maxHamming bits of
	// one in each other episode counts as present there.
	const base uint64 = 0x0F0F0F0F0F0F0F0F
	eps := [][]uint64{
		{base, 0x1111},
		{base ^ 0b111},  // 3 bits away
		{base ^ 0xFF00}, // 8 bits away
		{0x2222},        // unrelated
	}
	got := FrequencyMapHamming(eps, 0.75, 8)
	if !got[base] {
		t.Errorf("base is within 8 bits of episodes 2 and 3 (3/4 >= 0.75), want it kept: %v", got)
	}
	if got[0x1111] {
		t.Errorf("0x1111 appears in the first episode only, want it dropped: %v", got)
	}
	if got[0x2222] {
		t.Errorf("a hash absent from the first episode is never a candidate: %v", got)
	}
	// A tighter tolerance loses the 8-bit neighbour: 2/4 < 0.75.
	if got := FrequencyMapHamming(eps, 0.75, 4); got[base] {
		t.Errorf("with maxHamming 4 only 2 of 4 episodes carry base, want it dropped: %v", got)
	}
	// maxHamming <= 0 is the exact-match map.
	exact := FrequencyMapHamming([][]uint64{{base}, {base}, {base ^ 1}}, 0.6, 0)
	if !exact[base] || len(exact) != 1 {
		t.Errorf("exact fallback: got %v, want {base}", exact)
	}
}

func TestMatchesIntroHashes(t *testing.T) {
	intro := map[uint64]bool{0xF0: true}
	if !matchesIntroHashes(0xF0, intro, 0) {
		t.Error("exact hash must match with tolerance 0")
	}
	if matchesIntroHashes(0xF1, intro, 0) {
		t.Error("tolerance 0 is exact-only")
	}
	if !matchesIntroHashes(0xF1, intro, 1) {
		t.Error("one bit away must match with tolerance 1")
	}
	if matchesIntroHashes(0xF3, intro, 1) {
		t.Error("two bits away must not match with tolerance 1")
	}
	if matchesIntroHashes(0xF0, nil, 8) {
		t.Error("an empty intro set matches nothing")
	}
}
