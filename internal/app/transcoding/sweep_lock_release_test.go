package transcoding

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/pkg/bgloop"
)

// A recover only unwinds defers. bgloop.Tick (2026-08-01) caught panics in the
// background sweeps, but the critical sections unlocked with a plain call at
// the end of the block - so a panic left p.mu held forever, and p.mu is the
// same lock every EnsureSegmentRange takes. Software-path playback froze,
// /healthz still returned 200, and nothing restarted. Before the net, the same
// panic killed the process and Docker restarted it in seconds.
//
// These pin the only thing that matters: after a panicking tick, the lock must
// be reacquirable.

// panicSessions returns a session table that makes any sweep panic: the nil
// entry dereferences on the first s.mu.Lock().
func panicSessions() map[string][]*swSession {
	return map[string][]*swSession{"boom": {nil}}
}

func TestSweepIdle_ReleasesProducerLockOnPanic(t *testing.T) {
	p := &SoftwareProducer{
		sessions: panicSessions(),
		opts:     SoftwareProducerOptions{InactivityTimeout: time.Minute},
	}

	bgloop.Tick("test.sweepIdle", p.sweepIdle)

	if !p.mu.TryLock() {
		t.Fatal("p.mu stayed locked after a panicking tick: every segment request " +
			"would block forever, with no crash and no restart")
	}
	p.mu.Unlock()
}

func TestSweepKillPaused_ReleasesProducerLockOnPanic(t *testing.T) {
	p := &SoftwareProducer{
		sessions: panicSessions(),
		opts:     SoftwareProducerOptions{KillPausedAfter: time.Minute},
	}

	bgloop.Tick("test.sweepKillPaused", p.sweepKillPaused)

	if !p.mu.TryLock() {
		t.Fatal("p.mu stayed locked after a panicking tick")
	}
	p.mu.Unlock()
}

// No test for sweepPause: it releases p.mu before the loop that can panic, so a
// "lock is reacquirable" assertion there would pass while proving nothing. Its
// per-session critical section (s.mu around the SIGSTOP) was moved to defer for
// the same reason.

func TestOrchestratorSweepIdle_ReleasesLockOnPanic(t *testing.T) {
	o := &SegmentOrchestrator{
		sessions: map[string][]*orchestratorSession{"boom": {nil}},
	}

	bgloop.Tick("test.orchestrator.sweepIdle", o.sweepIdle)

	if !o.mu.TryLock() {
		t.Fatal("o.mu stayed locked after a panicking tick: hardware sessions could " +
			"no longer start or stop")
	}
	o.mu.Unlock()
}
