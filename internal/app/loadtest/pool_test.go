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

// A library spread over several disks must be handed out ROUND-ROBIN: viewer 1
// on disk A, viewer 2 on disk B. Serving them in database order piles the whole
// audience onto whichever drive comes first - the run then measures that single
// drive and reports it as the machine's ceiling, while the rest of the array
// sits idle.
func TestPool_SpreadsViewersAcrossDisks(t *testing.T) {
	// Two files on /tmp (one disk) is all a unit test can stage; the ordering
	// logic itself is what matters, so drive it directly.
	a := []MediaCandidate{{ID: "a1"}, {ID: "a2"}, {ID: "a3"}}
	b := []MediaCandidate{{ID: "b1"}, {ID: "b2"}}
	got := interleaveByDisk([][]MediaCandidate{a, b})

	want := []string{"a1", "b1", "a2", "b2", "a3"}
	if len(got) != len(want) {
		t.Fatalf("got %d candidates, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d = %s, want %s (full: %v)", i, got[i].ID, id, ids(got))
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
