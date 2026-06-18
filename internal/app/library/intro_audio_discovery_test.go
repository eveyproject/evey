package library

import (
	"math/rand"
	"testing"
)

// makeNoise returns n deterministic pseudo-random fingerprint frames.
func makeNoise(r *rand.Rand, n int) []uint32 {
	out := make([]uint32, n)
	for i := range out {
		out[i] = r.Uint32()
	}
	return out
}

// withNoisyCopy returns a copy of intro with up to maxFlips random bit flips per
// frame — simulates the same theme re-encoded at lower quality (SD).
func withNoisyCopy(r *rand.Rand, intro []uint32, maxFlips int) []uint32 {
	out := make([]uint32, len(intro))
	for i, f := range intro {
		flips := r.Intn(maxFlips + 1)
		for range flips {
			f ^= 1 << uint(r.Intn(32))
		}
		out[i] = f
	}
	return out
}

// episodeWithIntro builds noise of total length with a (noisy) copy of intro
// spliced in at frame `at`.
func episodeWithIntro(r *rand.Rand, total, at int, intro []uint32, maxFlips int) []uint32 {
	ep := makeNoise(r, total)
	copy(ep[at:], withNoisyCopy(r, intro, maxFlips))
	return ep
}

func TestLongestSharedRun_FindsBlockAtDifferentOffsets(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	intro := makeNoise(r, 120) // ~16s
	a := episodeWithIntro(r, 800, 100, intro, 4)
	b := episodeWithIntro(r, 800, 450, intro, 4) // different position

	s, e, found := longestSharedRun(a, b, 12, 80)
	if !found {
		t.Fatal("expected to find the shared run")
	}
	// run should land on a's intro region [100,220), allowing small edge slack
	if s < 90 || s > 110 || (e-s) < 100 {
		t.Fatalf("run [%d,%d) doesn't match intro at ~100 len ~120", s, e)
	}
}

func TestDiscover_VariablePositionRecurrence(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	intro := makeNoise(r, 130)
	// THE failing real-world case: intro floats across episodes (cold-open).
	target := episodeWithIntro(r, 900, 150, intro, 5)
	sibs := [][]uint32{
		episodeWithIntro(r, 900, 320, intro, 5),
		episodeWithIntro(r, 900, 470, intro, 5),
		episodeWithIntro(r, 900, 600, intro, 5),
	}
	startMs, endMs, matches, found := discoverRecurringIntro(target, sibs, defaultDiscoverParams())
	if !found {
		t.Fatalf("variable-position intro not discovered (matches=%d)", matches)
	}
	if matches < 3 {
		t.Fatalf("expected >=3 confirming episodes, got %d", matches)
	}
	// target intro at frame 150 -> ~19.8s
	gotStart := float64(startMs) / 1000.0
	if gotStart < 16 || gotStart > 24 {
		t.Fatalf("intro start %.1fs not near expected ~19.8s", gotStart)
	}
	if endMs <= startMs {
		t.Fatal("end before start")
	}
}

func TestDiscover_NoSharedIntro_ReturnsNothing(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	target := makeNoise(r, 900)
	sibs := [][]uint32{makeNoise(r, 900), makeNoise(r, 900), makeNoise(r, 900)}
	if _, _, _, found := discoverRecurringIntro(target, sibs, defaultDiscoverParams()); found {
		t.Fatal("must not discover an intro in pure noise")
	}
}

func TestDiscover_CoincidentalShortMatch_Rejected(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	shared := makeNoise(r, 30) // 30 frames < MinRunFrames(80) -> too short to be an intro
	target := episodeWithIntro(r, 900, 150, shared, 3)
	sibs := [][]uint32{
		episodeWithIntro(r, 900, 300, shared, 3),
		episodeWithIntro(r, 900, 500, shared, 3),
		episodeWithIntro(r, 900, 650, shared, 3),
	}
	if _, _, _, found := discoverRecurringIntro(target, sibs, defaultDiscoverParams()); found {
		t.Fatal("a short coincidental shared block must not be accepted as an intro")
	}
}

func TestDiscover_AlignedPosition_StillWorks(t *testing.T) {
	// Regression guard for the easy case the current detector already handles.
	r := rand.New(rand.NewSource(5))
	intro := makeNoise(r, 120)
	at := 200
	target := episodeWithIntro(r, 800, at, intro, 4)
	sibs := [][]uint32{
		episodeWithIntro(r, 800, at, intro, 4),
		episodeWithIntro(r, 800, at, intro, 4),
		episodeWithIntro(r, 800, at, intro, 4),
	}
	_, _, matches, found := discoverRecurringIntro(target, sibs, defaultDiscoverParams())
	if !found || matches < 4 {
		t.Fatalf("aligned intro should be found in all (found=%v matches=%d)", found, matches)
	}
}

func TestDiscover_BelowMinMatches_Rejected(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	intro := makeNoise(r, 130)
	target := episodeWithIntro(r, 900, 150, intro, 5)
	// Only ONE sibling carries the intro -> matches=2 < MinMatches(3).
	sibs := [][]uint32{
		episodeWithIntro(r, 900, 300, intro, 5),
		makeNoise(r, 900),
		makeNoise(r, 900),
	}
	if _, _, m, found := discoverRecurringIntro(target, sibs, defaultDiscoverParams()); found {
		t.Fatalf("must reject with too few confirming siblings (matches=%d)", m)
	}
}
