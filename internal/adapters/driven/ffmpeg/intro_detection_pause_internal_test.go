package ffmpeg

import (
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// With no busy checker, runPausable behaves exactly like cmd.Run().
func TestRunPausable_NilBusy_RunsNormally(t *testing.T) {
	d := &IntroDetector{}
	require.NoError(t, d.runPausable(exec.Command("true")))
}

// While busy() is true the command is SIGSTOP-suspended (its own short timer does
// NOT elapse); once busy clears it is SIGCONT-resumed and completes. This is the
// "yield to real work, keep progress" contract.
func TestRunPausable_SuspendsWhileBusyThenResumes(t *testing.T) {
	old := detectionPausePoll
	detectionPausePoll = 10 * time.Millisecond
	defer func() { detectionPausePoll = old }()

	var busy atomic.Bool
	busy.Store(true)
	d := (&IntroDetector{}).WithBusyChecker(busy.Load)

	cmd := exec.Command("sleep", "0.2")
	done := make(chan error, 1)
	go func() { done <- d.runPausable(cmd) }()

	// A non-suspended 0.2 s sleep would finish well within 400 ms; suspended, it
	// must NOT have finished yet.
	select {
	case <-done:
		t.Fatal("command finished while it should have been SIGSTOP-suspended")
	case <-time.After(400 * time.Millisecond):
	}

	busy.Store(false) // release → SIGCONT → it runs out its sleep and exits
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("command did not resume after the busy flag cleared")
	}
}
