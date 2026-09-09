package library

import (
	"math/rand/v2"
	"testing"
)

// The pHash pass is the CPU-heaviest pure-Go work in the product, and it runs on
// the box the user already said is weak. FrequencyMapHamming is
// O(candidates x episodes x frames) with no early exit across candidates, so its
// cost grows with the square of a season's frame count - the shape that turns a
// background backfill into a box that stops serving.
//
// Sizes are the real ones: trickplay aHash at 1 frame/15s, so a 22-minute
// episode is ~88 frames and a 55-minute one ~220, over seasons of 8 to 24.
// Defaults from intro_detection_config.go: FreqMapThreshold 0.30, MaxHamming 8.

const (
	benchThreshold  = 0.30
	benchMaxHamming = 8
)

// makeSeasonHashes builds episodes that share an intro (the same hashes, jittered
// by a few bits like real frame drift does) and diverge everywhere else.
func makeSeasonHashes(episodes, frames, introFrames int) [][]uint64 {
	r := rand.New(rand.NewPCG(1, 2))
	intro := make([]uint64, introFrames)
	for i := range intro {
		intro[i] = r.Uint64()
	}

	out := make([][]uint64, episodes)
	for e := range out {
		ep := make([]uint64, frames)
		for i := range ep {
			if i < introFrames {
				// Flip up to 3 bits: within MaxHamming, as a real re-encode is.
				ep[i] = intro[i] ^ (uint64(r.UintN(8)) << r.UintN(60))
				continue
			}
			ep[i] = r.Uint64()
		}
		out[e] = ep
	}
	return out
}

func BenchmarkFrequencyMapHamming(b *testing.B) {
	cases := []struct {
		name                        string
		episodes, frames, introFram int
	}{
		{"season8x88", 8, 88, 6},
		{"season12x220", 12, 220, 8},
		{"season24x220", 24, 220, 8},
	}
	for _, c := range cases {
		hashes := makeSeasonHashes(c.episodes, c.frames, c.introFram)
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				FrequencyMapHamming(hashes, benchThreshold, benchMaxHamming)
			}
		})
	}
}

// FrequencyMap is the maxHamming<=0 path: exact match, one map pass. Benchmarked
// beside the Hamming one because the gap between them is the price of tolerating
// frame drift, and that price is what a future optimisation has to beat.
func BenchmarkFrequencyMapExact(b *testing.B) {
	hashes := makeSeasonHashes(24, 220, 8)
	b.ReportAllocs()
	for b.Loop() {
		FrequencyMap(hashes, benchThreshold)
	}
}
