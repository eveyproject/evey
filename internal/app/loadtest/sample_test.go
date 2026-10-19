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

// Whatever the library looks like, the sample holds the same rules: n titles
// (or the whole library when it is smaller), none twice, each kind within one
// title of its exact share, each bitrate decile of a kind within one title of
// its exact share of that kind's seats, and the same seed replays it.
func TestLibrarySample_AnyLibrary(t *testing.T) {
	gen := func(films, episodes int, filmRate, epRate func(i int) int64) []MediaCandidate {
		var lib []MediaCandidate
		for i := range films {
			lib = append(lib, MediaCandidate{ID: fmt.Sprintf("m%06d", i), Kind: "movie", BitrateBps: filmRate(i)})
		}
		for i := range episodes {
			lib = append(lib, MediaCandidate{ID: fmt.Sprintf("e%06d", i), Kind: "episode", BitrateBps: epRate(i)})
		}
		return lib
	}
	spread := func(i int) int64 { return int64(1_000_000 + (i*7919)%60_000_000) }
	same := func(int) int64 { return 8_000_000 }
	unknown := func(int) int64 { return 0 }
	halfUnknown := func(i int) int64 {
		if i%2 == 0 {
			return 0
		}
		return spread(i)
	}
	for name, lib := range map[string][]MediaCandidate{
		"films only":                    gen(900, 0, spread, spread),
		"episodes only":                 gen(0, 40_000, spread, spread),
		"one film among many episodes":  gen(1, 9_000, spread, spread),
		"a few films, few episodes":     gen(7, 12, spread, spread),
		"every bitrate unknown":         gen(400, 4_000, unknown, unknown),
		"half the bitrates unknown":     gen(400, 4_000, halfUnknown, halfUnknown),
		"one bitrate for every title":   gen(400, 4_000, same, same),
		"exactly the sample size":       gen(100, 200, spread, spread),
		"one title more than requested": gen(100, 201, spread, spread),
	} {
		t.Run(name, func(t *testing.T) {
			const n = 300
			s := LibrarySample(lib, n, 99)
			if want := min(n, len(lib)); len(s) != want {
				t.Fatalf("%d titles, want %d", len(s), want)
			}
			seen := map[string]bool{}
			for _, c := range s {
				if seen[c.ID] {
					t.Fatalf("%s drawn twice", c.ID)
				}
				seen[c.ID] = true
			}
			if !slices.EqualFunc(s, LibrarySample(lib, n, 99), func(x, y MediaCandidate) bool { return x.ID == y.ID }) {
				t.Fatal("the same seed drew two samples")
			}
			if len(lib) <= n {
				return
			}
			libKind, sampleKind := map[string][]MediaCandidate{}, map[string][]MediaCandidate{}
			for _, c := range lib {
				libKind[c.Kind] = append(libKind[c.Kind], c)
			}
			for _, c := range s {
				sampleKind[c.Kind] = append(sampleKind[c.Kind], c)
			}
			for kind, titles := range libKind {
				exact := float64(n) * float64(len(titles)) / float64(len(lib))
				seats := len(sampleKind[kind])
				if float64(seats) < exact-1 || float64(seats) > exact+1 {
					t.Errorf("%s: %d titles, want %.1f (its share of the library)", kind, seats, exact)
				}
				bands, _ := bitrateQuantiles(titles, librarySampleBands)
				bandOf := map[string]string{}
				perBand := map[string]int{}
				for i, c := range titles {
					bandOf[c.ID] = bands[i]
					perBand[bands[i]]++
				}
				drawn := map[string]int{}
				for _, c := range sampleKind[kind] {
					drawn[bandOf[c.ID]]++
				}
				for b, count := range perBand {
					exact := float64(seats) * float64(count) / float64(len(titles))
					if float64(drawn[b]) < exact-1 || float64(drawn[b]) > exact+1 {
						t.Errorf("%s band %s: %d titles, want %.1f", kind, b, drawn[b], exact)
					}
				}
			}
		})
	}
}
