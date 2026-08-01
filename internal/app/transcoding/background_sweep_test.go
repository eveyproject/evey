package transcoding

import (
	"sync/atomic"
	"testing"
)

// The point of the helper is that a bad tick costs a tick, not the server. A
// panic in one sweep used to unwind a goroutine with no caller and take the
// whole process down - every viewer's stream, because one session was bad.
func TestSweep_PanicIsContainedAndTheLoopGoesOn(t *testing.T) {
	var ran atomic.Int32

	// A panicking tick must not propagate...
	sweep("test.boom", func() {
		ran.Add(1)
		panic("bad session")
	})
	// ...and the next tick must still run: had the panic escaped, this
	// goroutine would be gone and the test binary with it.
	sweep("test.ok", func() { ran.Add(1) })

	if got := ran.Load(); got != 2 {
		t.Errorf("ticks run = %d, want 2 (the panicking one and the one after it)", got)
	}
}

// A nil dereference is the realistic shape (a session map entry that went away
// mid-sweep), not an explicit panic() call.
func TestSweep_ContainsANilDereference(t *testing.T) {
	type session struct{ id string }
	var missing *session

	sweep("test.nil", func() { _ = missing.id })

	// Reaching here at all is the assertion.
}
