package loadtest

import (
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

// The run draws its titles the way an audience does: at random. The shuffle
// keeps every title exactly once, moves them (a pool handed out in library
// order loads whatever the listing happens to start with), and is reproducible
// from its seed so a run can be replayed.
func TestShuffleCandidates_IsAPermutationReproducibleFromItsSeed(t *testing.T) {
	in := manyCandidates(50)
	a := shuffleCandidates(append([]MediaCandidate(nil), in...), 42)
	b := shuffleCandidates(append([]MediaCandidate(nil), in...), 42)
	c := shuffleCandidates(append([]MediaCandidate(nil), in...), 43)

	seen := map[string]int{}
	for _, m := range a {
		seen[m.ID]++
	}
	if len(seen) != len(in) {
		t.Fatalf("shuffle kept %d distinct titles of %d", len(seen), len(in))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("title %s appears %d times after the shuffle", id, n)
		}
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
		t.Fatalf("the same seed gave two orders: %v vs %v", ids(a)[:6], ids(b)[:6])
	}
	if same(a, c) {
		t.Fatal("two seeds gave the same order")
	}
	if same(a, in) {
		t.Fatal("the shuffle left 50 titles in library order")
	}
}

func ids(cs []MediaCandidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
