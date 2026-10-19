package loadtest

import (
	"fmt"
	"slices"
	"testing"
)

// sampleLibrary is a library of ten episodes per film: films at 2 to 80 Mb/s,
// episodes at 1 to 10 Mb/s.
func sampleLibrary() []MediaCandidate {
	var lib []MediaCandidate
	for i := range 200 {
		lib = append(lib, MediaCandidate{ID: fmt.Sprintf("m%04d", i), Kind: "movie", BitrateBps: int64(2_000_000 + i*390_000)})
	}
	for i := range 2000 {
		lib = append(lib, MediaCandidate{ID: fmt.Sprintf("e%04d", i), Kind: "episode", BitrateBps: int64(1_000_000 + i*4_500)})
	}
	return lib
}

func kindsAndHeavy(sample []MediaCandidate) (movies, heavy int) {
	for _, c := range sample {
		if c.Kind == "movie" {
			movies++
		}
		if c.BitrateBps >= 40_000_000 {
			heavy++
		}
	}
	return movies, heavy
}

// A sample holds each kind and the heavy tail in their share of the library,
// whatever the seed: a tenth of films, and the films above 40 Mb/s (102 of
// 2200 titles, 13.8 of 300) within a seat of a decile (27 films over ten
// deciles). A slice of half films and half episodes drew 150 films, 76 of them
// above 40 Mb/s. The seed changes the titles, not the mix.
func TestLibrarySample_KeepsTheLibrarysShares(t *testing.T) {
	lib := sampleLibrary()
	var firsts []string
	for _, seed := range []uint64{1, 2, 1792362923756274997, 1792402278540730711} {
		s := LibrarySample(lib, 300, seed)
		if len(s) != 300 {
			t.Fatalf("seed %d: %d titles, want 300", seed, len(s))
		}
		movies, heavy := kindsAndHeavy(s)
		if movies < 26 || movies > 28 {
			t.Errorf("seed %d: %d films of 300, want 27 (the library holds 200 of 2200)", seed, movies)
		}
		if heavy < 12 || heavy > 16 {
			t.Errorf("seed %d: %d titles above 40 Mb/s, want 14 (102 of 2200 in the library)", seed, heavy)
		}
		seen := map[string]bool{}
		for _, c := range s {
			if seen[c.ID] {
				t.Fatalf("seed %d: %s drawn twice", seed, c.ID)
			}
			seen[c.ID] = true
		}
		firsts = append(firsts, s[0].ID+s[150].ID)
	}
	if firsts[0] == firsts[1] {
		t.Error("seeds 1 and 2 drew the same titles")
	}
}

// The same seed over the same library replays the same titles.
func TestLibrarySample_ReplaysItsSeed(t *testing.T) {
	a, b := LibrarySample(sampleLibrary(), 300, 42), LibrarySample(sampleLibrary(), 300, 42)
	if !slices.EqualFunc(a, b, func(x, y MediaCandidate) bool { return x.ID == y.ID }) {
		t.Fatal("seed 42 drew two different samples of one library")
	}
}

// A library no larger than the sample is returned whole.
func TestLibrarySample_SmallLibrary(t *testing.T) {
	lib := sampleLibrary()[:50]
	if got := LibrarySample(lib, 300, 7); len(got) != 50 {
		t.Fatalf("got %d titles, want the 50 of the library", len(got))
	}
	if got := LibrarySample(lib, 0, 7); len(got) != 0 {
		t.Fatalf("got %d titles for an empty sample", len(got))
	}
}
