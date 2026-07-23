package main

import (
	"sync/atomic"
	"testing"
)

// stopBackgroundWorkers must invoke EVERY registered stopper, and one panicking
// stopper must not skip the rest: shutdown is best-effort cleanup, never a place
// a single buggy worker can wedge the drain of the others (which each hold a real
// DB write that must finish before the pool closes).
func TestStopBackgroundWorkers_RunsAllEvenIfOnePanics(t *testing.T) {
	var a, b, c atomic.Bool
	d := &appDeps{
		bgStoppers: []func(){
			func() { a.Store(true) },
			func() { panic("boom in a stopper") },
			func() { b.Store(true) },
			func() { c.Store(true) },
		},
	}
	d.stopBackgroundWorkers()
	if !a.Load() || !b.Load() || !c.Load() {
		t.Fatalf("not all stoppers ran: a=%v b=%v c=%v", a.Load(), b.Load(), c.Load())
	}
}
