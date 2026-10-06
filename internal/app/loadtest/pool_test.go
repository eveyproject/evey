package loadtest

import (
	"fmt"
	"testing"
	"time"
)

// A title the server refuses to serve the scenario's way is refused at EVERY
// start position: once marked, the pool must never offer it again - not on the
// next spawn, and above all not when the climb rebuilds its whole audience.
func TestPool_AnIneligibleTitleIsNeverOfferedAgain(t *testing.T) {
	pool := newCandidatePool(manyCandidates(3), time.Minute)
	pool.markIneligible("m1")
	for range pool.capacity() {
		asg, ok := pool.take()
		if !ok {
			break
		}
		if asg.media.ID == "m1" {
			t.Fatalf("the pool handed out m1 (round %d) after it was marked ineligible", asg.round)
		}
	}
}

// A library-shaped set of titles: 10 SD, 30 720p, 50 1080p, 10 4K.
func mixedCandidates() []MediaCandidate {
	var out []MediaCandidate
	add := func(n, height int, tag string) {
		for i := range n {
			out = append(out, MediaCandidate{ID: fmt.Sprintf("%s-%d", tag, i), VideoHeight: height, DurationSec: 3600})
		}
	}
	add(10, 480, "sd")
	add(30, 720, "hd")
	add(50, 1080, "fhd")
	add(10, 2160, "uhd")
	return out
}

// The draw keeps every title exactly once and is reproducible from its seed,
// so a run can be replayed; another seed draws another order.
func TestRepresentativeDraw_IsAPermutationReproducibleFromItsSeed(t *testing.T) {
	a, _ := representativeDraw(mixedCandidates(), 42)
	b, _ := representativeDraw(mixedCandidates(), 42)
	c, _ := representativeDraw(mixedCandidates(), 43)
	seen := map[string]int{}
	for _, m := range a {
		seen[m.ID]++
	}
	if len(seen) != 100 || len(a) != 100 {
		t.Fatalf("draw kept %d distinct titles in %d slots, want 100 in 100", len(seen), len(a))
	}
	same := func(x, y []MediaCandidate) bool {
		for i := range x {
			if x[i].ID != y[i].ID {
				return false
			}
		}
		return true
	}
	if !same(a, b) {
		t.Fatal("the same seed must replay the same draw")
	}
	if same(a, c) {
		t.Fatal("another seed must draw another order")
	}
}

// Whatever number of viewers a run reaches, they watch the library's own mix:
// every prefix of the draw holds each quality within one title of its share.
// A plain shuffle could hand the first fifty viewers mostly 4K.
func TestRepresentativeDraw_EveryPrefixKeepsTheLibrarysMix(t *testing.T) {
	out, mix := representativeDraw(mixedCandidates(), 7)
	if mix["sd"] != 10 || mix["720p"] != 30 || mix["1080p"] != 50 || mix["2160p"] != 10 {
		t.Fatalf("mix = %v, want sd 10, 720p 30, 1080p 50, 2160p 10", mix)
	}
	share := map[string]float64{"sd": 0.1, "720p": 0.3, "1080p": 0.5, "2160p": 0.1}
	taken := map[string]int{}
	for n, m := range out {
		taken[qualityClass(m.VideoHeight)]++
		for k, p := range share {
			if d := float64(taken[k]) - p*float64(n+1); d > 1 || d < -1 {
				t.Fatalf("after %d viewers, %s has %d titles, %.1f expected", n+1, k, taken[k], p*float64(n+1))
			}
		}
	}
}

// A quality the library does not have is never drawn, and a title whose
// streams were never probed is a quality of its own, not counted as 4K.
func TestRepresentativeDraw_OnlyTheQualitiesTheLibraryHas(t *testing.T) {
	in := []MediaCandidate{{ID: "a", VideoHeight: 1080}, {ID: "b", VideoHeight: 1080}, {ID: "c"}}
	_, mix := representativeDraw(in, 1)
	if len(mix) != 2 || mix["1080p"] != 2 || mix["unknown"] != 1 {
		t.Fatalf("mix = %v, want only 1080p 2 and unknown 1", mix)
	}
}

// Heights of wide films sit below the nominal ones and still land in their
// class.
func TestQualityClass_WideFilms(t *testing.T) {
	for h, want := range map[int]string{1606: "2160p", 2160: "2160p", 800: "1080p", 1080: "1080p", 536: "720p", 720: "720p", 404: "sd", 0: "unknown"} {
		if got := qualityClass(h); got != want {
			t.Fatalf("height %d = %s, want %s", h, got, want)
		}
	}
}

func ids(cs []MediaCandidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
