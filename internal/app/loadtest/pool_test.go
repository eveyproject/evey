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

// A library-shaped set of titles: 100 bitrates from 1 to 100 Mb/s.
func mixedCandidates() []MediaCandidate {
	out := make([]MediaCandidate, 0, 100)
	for i := range 100 {
		out = append(out, MediaCandidate{ID: fmt.Sprintf("t%d", i), BitrateBps: int64(i+1) * 1_000_000, DurationSec: 3600})
	}
	return out
}

// The draw keeps every title exactly once and is reproducible from its seed,
// so a run can be replayed; another seed draws another order.
func TestRepresentativeDraw_IsAPermutationReproducibleFromItsSeed(t *testing.T) {
	a, _, _ := representativeDraw(mixedCandidates(), 42)
	b, _, _ := representativeDraw(mixedCandidates(), 42)
	c, _, _ := representativeDraw(mixedCandidates(), 43)
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

// Whatever number of viewers a run reaches, they pull the library's own mix of
// bitrates: every prefix of the draw holds each quarter of the bitrates within
// one title of its share. A plain shuffle could hand the first fifty viewers
// mostly heavy files.
func TestRepresentativeDraw_EveryPrefixKeepsTheLibrarysBitrates(t *testing.T) {
	in := mixedCandidates()
	out, mix, floors := representativeDraw(in, 7)
	if len(floors) != 4 || floors[0] != 1_000_000 || floors[1] != 26_000_000 || floors[2] != 51_000_000 || floors[3] != 76_000_000 {
		t.Fatalf("band floors = %v, want the quartiles 1/26/51/76 Mb/s", floors)
	}
	for _, q := range []string{"q1", "q2", "q3", "q4"} {
		if mix[q] != 25 {
			t.Fatalf("mix = %v, want 25 titles per band", mix)
		}
	}
	quarter := func(bps int64) int { return int((bps/1_000_000 - 1) / 25) }
	taken := [4]int{}
	for n, m := range out {
		taken[quarter(m.BitrateBps)]++
		for q := range 4 {
			if d := float64(taken[q]) - 0.25*float64(n+1); d > 1 || d < -1 {
				t.Fatalf("after %d viewers, band %d has %d titles, %.1f expected", n+1, q+1, taken[q], 0.25*float64(n+1))
			}
		}
	}
}

// A title of unknown bitrate is a band of its own, never counted as a heavy or
// a light one; with no known bitrate at all there are no floors.
func TestBitrateBands_UnknownIsItsOwnBand(t *testing.T) {
	band, floors := bitrateBands([]MediaCandidate{{ID: "a", BitrateBps: 8_000_000}, {ID: "b"}})
	if band[0] == "unknown" || band[1] != "unknown" || len(floors) != 4 {
		t.Fatalf("bands = %v floors = %v, want a known band for a and unknown for b", band, floors)
	}
	if _, floors := bitrateBands([]MediaCandidate{{ID: "c"}}); len(floors) != 0 {
		t.Fatalf("floors = %v, want none when no bitrate is known", floors)
	}
}

// Every prefix of the draw keeps each disk to its share of each bitrate band:
// the heavy titles of a two-disk pool go to both disks from the first viewers
// on, instead of piling on whichever disk the shuffle favoured.
func TestRepresentativeDrawOnDisks_KeepsEachDiskToItsShare(t *testing.T) {
	in := mixedCandidates() // bitrates 1..100 Mb/s: q4 = the 25 heaviest
	disks := make([]string, len(in))
	diskOf := map[string]string{}
	for i, c := range in {
		disks[i] = "/mnt/disk_1"
		if i%2 == 1 {
			disks[i] = "/mnt/disk_2"
		}
		diskOf[c.ID] = disks[i]
	}
	for _, seed := range []uint64{1, 2, 3, 42} {
		out, mix, _, _ := representativeDrawOnDisks(in, disks, seed)
		if len(out) != len(in) || mix["q4"] != 25 {
			t.Fatalf("seed %d: %d titles, q4=%d; want %d and 25", seed, len(out), mix["q4"], len(in))
		}
		for _, n := range []int{8, 20, 40} {
			heavy := map[string]int{}
			for _, c := range out[:n] {
				if c.BitrateBps >= 76_000_000 {
					heavy[diskOf[c.ID]]++
				}
			}
			if d := heavy["/mnt/disk_1"] - heavy["/mnt/disk_2"]; d > 1 || d < -1 {
				t.Errorf("seed %d, first %d viewers: heavy titles per disk %v", seed, n, heavy)
			}
		}
	}
	// No disk known: the plain bitrate draw, unchanged.
	a, _, _, _ := representativeDrawOnDisks(in, nil, 9)
	b, _, _ := representativeDraw(in, 9)
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("without disks the draw changed at %d: %s vs %s", i, a[i].ID, b[i].ID)
		}
	}
}
