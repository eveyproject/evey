package library

import (
	"math/rand"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
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
// frame - simulates the same theme re-encoded at lower quality (SD).
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

	s, e, found := longestSharedRun(a, b, 12, 80, 0)
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
	// Target intro sits past the 25 s 0:00-bumper guard (frame 250 -> ~33s).
	target := episodeWithIntro(r, 900, 250, intro, 5)
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
	// target intro at frame 250 -> ~33.1s
	gotStart := float64(startMs) / 1000.0
	if gotStart < 29 || gotStart > 37 {
		t.Fatalf("intro start %.1fs not near expected ~33.1s", gotStart)
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
	at := 240 // ~29.7s at the real 8.08 frames/s, clear of the 25s anti-bumper floor
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

func TestDiscover_RejectsRecurring0SecBumper(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	bumper := makeNoise(r, 160) // ~21s recurring network/promo card pinned at 0:00
	intro := makeNoise(r, 130)  // the real intro, floats later in each episode
	build := func(introAt int) []uint32 {
		ep := makeNoise(r, 1000)
		copy(ep[0:], withNoisyCopy(r, bumper, 5))      // bumper aligned at 0:00 everywhere
		copy(ep[introAt:], withNoisyCopy(r, intro, 5)) // real intro after a variable teaser
		return ep
	}
	target := build(300) // ~39.7s
	sibs := [][]uint32{build(350), build(500), build(620)}

	startMs, _, _, found := discoverRecurringIntro(target, sibs, defaultDiscoverParams())
	if !found {
		t.Fatal("real intro after the bumper should still be discovered")
	}
	if got := float64(startMs) / 1000.0; got < 35 {
		t.Fatalf("discovery latched the 0:00 bumper (start %.1fs), expected the real intro ~39.7s", got)
	}

	// Characterise the bug the guard fixes: with the guard disabled the LONGER
	// 0:00 bumper wins and is mislabelled as the intro.
	pNoGuard := defaultDiscoverParams()
	pNoGuard.MinStartFrames = 0
	if s, _, _, f := discoverRecurringIntro(target, sibs, pNoGuard); !f || float64(s)/1000.0 > 5 {
		t.Fatalf("without the guard the 0:00 bumper should win (found=%v start=%.1fs)", f, float64(s)/1000.0)
	}
}

func TestSelectNearestSiblings_PicksNeighbors(t *testing.T) {
	ep := func(n int) *domainlibrary.MediaItem {
		v := n
		return &domainlibrary.MediaItem{IndexNumber: &v}
	}
	target := ep(5)
	cands := []*domainlibrary.MediaItem{ep(1), ep(2), ep(3), ep(4), ep(6), ep(7), ep(8)}

	got := selectNearestSiblings(target, cands, 4)
	if len(got) != 4 {
		t.Fatalf("expected 4 neighbours, got %d", len(got))
	}
	want := map[int]bool{3: true, 4: true, 6: true, 7: true} // ±2 of episode 5
	for _, m := range got {
		if m.IndexNumber == nil || !want[*m.IndexNumber] {
			t.Fatalf("selected episode %v is not within ±2 of the target", m.IndexNumber)
		}
	}
}

func TestSelectNearestSiblings_NilIndexFallsBackToOrder(t *testing.T) {
	ep := func(n int) *domainlibrary.MediaItem {
		v := n
		return &domainlibrary.MediaItem{IndexNumber: &v}
	}
	target := &domainlibrary.MediaItem{} // no IndexNumber
	cands := []*domainlibrary.MediaItem{ep(8), ep(2), ep(5), ep(1), ep(9)}
	got := selectNearestSiblings(target, cands, 3)
	if len(got) != 3 || got[0] != cands[0] || got[2] != cands[2] {
		t.Fatal("with no target index, the first N candidates should be returned in order")
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
