package ffmpeg

import (
	"testing"
	"time"
)

// Done() must not fire while the pump still has a segment to publish, even
// once the process is over: the orchestrators fail every waiter on it.
func TestFinishAfterPump_WaitsForBothTheProcessAndThePump(t *testing.T) {
	done := make(chan struct{})
	release := make(chan struct{})
	finished := finishAfterPump(done, func() { <-release })

	close(done)
	select {
	case <-finished:
		t.Fatal("finished with the pump still publishing")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("not finished once the process and the pump are both over")
	}
}

// A pump that returns early (its stop path) does not finish the session ahead
// of its process.
func TestFinishAfterPump_WaitsForTheProcess(t *testing.T) {
	done := make(chan struct{})
	finished := finishAfterPump(done, func() {})
	select {
	case <-finished:
		t.Fatal("finished before the process")
	case <-time.After(50 * time.Millisecond):
	}
	close(done)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("not finished after the process")
	}
}
