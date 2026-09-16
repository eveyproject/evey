package library

import (
	"testing"
	"time"
)

// A library whose watcher is proven alive still gets an hourly backstop walk:
// the first pass is always due, the next one only once the interval elapsed,
// and each pass stamps the library independently of the others.
func TestDueForWatchedScan(t *testing.T) {
	s := &CommandService{}
	if s.sinceWatchedScan("lib-a") != 0 {
		t.Fatal("never walked = 0, not a huge duration")
	}
	if !s.dueForWatchedScan("lib-a") {
		t.Fatal("the first pass is always due")
	}
	if s.dueForWatchedScan("lib-a") {
		t.Fatal("a pass just ran: not due again")
	}
	if since := s.sinceWatchedScan("lib-a"); since <= 0 || since > time.Minute {
		t.Fatalf("since = %v, want a fresh stamp", since)
	}
	if !s.dueForWatchedScan("lib-b") {
		t.Fatal("another library keeps its own clock")
	}

	// Age lib-a past the interval: due again, and the pass re-stamps it.
	s.watchedScanMu.Lock()
	s.lastWatchedScan["lib-a"] = time.Now().Add(-watchedScanInterval - time.Second)
	s.watchedScanMu.Unlock()
	if !s.dueForWatchedScan("lib-a") {
		t.Fatal("interval elapsed: due")
	}
	if s.dueForWatchedScan("lib-a") {
		t.Fatal("the pass must re-stamp the library")
	}
}
