package ffmpeg

import (
	"os"
	"path/filepath"
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

// A media segment with no fragment (what a session seeked past the end of its
// track writes) is not one; one with a moof is.
func TestFMP4HasFragment(t *testing.T) {
	dir := t.TempDir()
	box := func(typ string, body int) []byte {
		b := make([]byte, 8+body)
		b[3] = byte(8 + body)
		copy(b[4:8], typ)
		return b
	}
	empty := filepath.Join(dir, "empty.m4s")
	if err := os.WriteFile(empty, box("styp", 16), 0o644); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(dir, "full.m4s")
	if err := os.WriteFile(full, append(append(box("styp", 16), box("moof", 8)...), box("mdat", 8)...), 0o644); err != nil {
		t.Fatal(err)
	}
	if fmp4HasFragment(empty) {
		t.Error("an styp alone is no fragment")
	}
	if !fmp4HasFragment(full) {
		t.Error("a moof is a fragment")
	}
}
