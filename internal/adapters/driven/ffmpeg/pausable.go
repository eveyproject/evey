package ffmpeg

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// detectionPausePoll is how often a paused-while-busy process re-checks whether
// it should suspend/resume. A var so tests can shorten it.
var detectionPausePoll = time.Second

// pauseProcessWhileBusy SIGSTOPs proc while busy() reports real work
// (playback/transcode or a recalibration) and SIGCONTs it when that clears,
// until the returned stop() is called - which also guarantees the process is left
// running, never stranded stopped. This is the same throttle the transcode
// encoder uses; it frees CPU, memory bandwidth and I/O for what matters while
// keeping the background decode's progress (no kill/re-run). busy==nil or a nil
// process is a no-op. Call stop() exactly once (after the process has been
// waited for, or is about to be).
func pauseProcessWhileBusy(proc *os.Process, busy func() bool) (stop func()) {
	if busy == nil || proc == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(detectionPausePoll)
		defer ticker.Stop()
		paused := false
		for {
			select {
			case <-done:
				if paused {
					//nolint:errcheck // best-effort SIGCONT; a dead process just errors
					_ = proc.Signal(syscall.SIGCONT)
				}
				return
			case <-ticker.C:
				switch b := busy(); {
				case b && !paused:
					if proc.Signal(syscall.SIGSTOP) == nil {
						paused = true
					}
				case !b && paused:
					if proc.Signal(syscall.SIGCONT) == nil {
						paused = false
					}
				}
			}
		}
	}()
	return func() { close(done) }
}

// runPausableCmd runs cmd to completion, suspending it via SIGSTOP while busy()
// is true (see pauseProcessWhileBusy). With no busy checker it is exactly
// cmd.Run(). The caller sets Stdout/Stderr beforehand, as with Run.
func runPausableCmd(cmd *exec.Cmd, busy func() bool) error {
	if busy == nil {
		return cmd.Run()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	stop := pauseProcessWhileBusy(cmd.Process, busy)
	err := cmd.Wait()
	stop()
	return err
}
