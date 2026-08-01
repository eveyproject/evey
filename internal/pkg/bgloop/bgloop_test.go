package bgloop

import (
	"sync/atomic"
	"testing"
)

// The whole point: a bad iteration costs an iteration, not the process. Before
// this, a panic in a loop nobody waits on took the server down for every user.
func TestTick_ContainsThePanicAndTheLoopGoesOn(t *testing.T) {
	var ran atomic.Int32

	Tick("test.boom", func() {
		ran.Add(1)
		panic("bad entry")
	})
	Tick("test.ok", func() { ran.Add(1) })

	if got := ran.Load(); got != 2 {
		t.Errorf("iterations run = %d, want 2 (the panicking one and the one after)", got)
	}
}

// The realistic shape is a nil dereference on state that changed under the
// sweep, not an explicit panic() call.
func TestTick_ContainsANilDereference(t *testing.T) {
	type entry struct{ id string }
	var gone *entry

	Tick("test.nil", func() { _ = gone.id })
	// Reaching this line is the assertion.
}

// A normal iteration must be entirely transparent - no swallowed work, no
// altered control flow.
func TestTick_RunsTheFunctionExactlyOnce(t *testing.T) {
	var n atomic.Int32
	Tick("test.plain", func() { n.Add(1) })
	if got := n.Load(); got != 1 {
		t.Errorf("ran %d times, want exactly 1", got)
	}
}
