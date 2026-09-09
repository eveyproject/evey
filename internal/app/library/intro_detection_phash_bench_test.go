package library

import (
	"math/rand/v2"
	"testing"
)

// FrequencyMapHamming is O(candidates x episodes x frames), so cost grows with
// the SQUARE of a season's frame count. Sizes are real: trickplay aHash at
// 1 frame/15s, i.e. ~88 frames for 22 min and ~220 for 55 min.

const (
	benchThreshold  = 0.30
	benchMaxHamming = 8
)

// Episodes share an intro, jittered a few bits like real frame drift, and
// diverge after it.
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

// The maxHamming<=0 path, for what tolerating frame drift costs.
func BenchmarkFrequencyMapExact(b *testing.B) {
	hashes := makeSeasonHashes(24, 220, 8)
	b.ReportAllocs()
	for b.Loop() {
		FrequencyMap(hashes, benchThreshold)
	}
}
