package loadtest

import "testing"

// drive runs the climb against a modelled machine that holds up to `ceiling`
// viewers, and returns the sequence of targets it tried plus the final verdict.
// phantomAt, when non-zero, makes the machine cry wolf ONCE at that target
// during a FAST hold - a cold-start spike misread as saturation.
func drive(t *testing.T, ceiling, maxViewers, phantomAt int) (targets []int, sustained int, reason StopReason) {
	t.Helper()
	c := &climb{target: 1, batch: fastBatch, spawnsEncoders: true}
	phantomFired := false

	for range 200 { // a generous bound; a real climb converges in a few dozen
		targets = append(targets, c.target)

		// Judge the tier the way the run loop would.
		broke := c.target > ceiling
		if !broke && c.phase == phaseFast && phantomAt > 0 && c.target == phantomAt && !phantomFired {
			broke = true
			phantomFired = true
		}
		if broke {
			c.tierBroke(StopCPUSaturated, "cpu")
		}

		// Model the load the way the real machine reports it: proportional to
		// how close this tier is to the ceiling.
		load := float64(c.target) / float64(max(ceiling, 1))
		outcome, r, _ := c.next(maxViewers, load)
		if outcome == climbDone {
			return targets, c.lo, r
		}
	}
	t.Fatalf("climb did not converge; targets=%v", targets)
	return nil, 0, ""
}

// closeEnough: the climb stops once the answer is within a few percent, because
// chasing the exact viewer costs a full measurement window per step and nobody
// acts differently on 58 than on 61.
func closeEnough(t *testing.T, got, want int, targets []int) {
	t.Helper()
	tolerance := max(1, want*bisectTolerancePercent/100)
	if got > want || want-got > tolerance {
		t.Fatalf("sustained = %d, want %d (+0/-%d) (targets=%v)", got, want, tolerance, targets)
	}
}

// The whole point: find the ceiling in a handful of steps, not one per viewer.
func TestClimb_FindsTheCeilingFastThenBisects(t *testing.T) {
	targets, sustained, reason := drive(t, 47, 0, 0)

	closeEnough(t, sustained, 47, targets)
	if reason != StopCPUSaturated {
		t.Fatalf("reason = %s, want %s", reason, StopCPUSaturated)
	}
	// A one-at-a-time ramp would need 48 tiers. Batches of 10 + bisection get
	// there in far fewer.
	if len(targets) > 15 {
		t.Fatalf("took %d tiers to find a ceiling of 47: %v", len(targets), targets)
	}
}

// A box that holds far more than a naive default must be allowed to say so:
// with no ceiling configured, the climb keeps going until the machine breaks.
func TestClimb_UnboundedByDefault(t *testing.T) {
	targets, sustained, reason := drive(t, 312, 0, 0)
	closeEnough(t, sustained, 312, targets)
	if reason != StopCPUSaturated {
		t.Fatalf("reason = %s, want a capacity verdict", reason)
	}
}

// An operator ceiling that is never reached is NOT a capacity result, and the
// verdict must not pretend otherwise.
func TestClimb_OperatorCeilingIsNotACapacityVerdict(t *testing.T) {
	_, sustained, reason := drive(t, 1000, 30, 0)
	if sustained != 30 {
		t.Fatalf("sustained = %d, want 30 (the operator's ceiling)", sustained)
	}
	if reason != StopMaxViewers {
		t.Fatalf("reason = %s, want %s", reason, StopMaxViewers)
	}
}

// The FAST phase judges on a few seconds of samples, so it can misread an
// ffmpeg cold start as saturation. A phantom break must NOT cap the run: the
// CONFIRM phase re-tests under a full window, finds the tier holds, and the
// climb resumes.
func TestClimb_PhantomBreakDoesNotCapTheRun(t *testing.T) {
	// The machine really holds 80, but cries wolf once at 11.
	targets, sustained, reason := drive(t, 80, 0, 11)
	if sustained < 70 {
		t.Fatalf("sustained = %d, want ~80: a phantom break at 11 capped the whole run (targets=%v)", sustained, targets)
	}
	if reason != StopCPUSaturated {
		t.Fatalf("reason = %s, want %s", reason, StopCPUSaturated)
	}
}

// A server that can't even hold one viewer says so, rather than looping.
func TestClimb_ZeroCapacity(t *testing.T) {
	_, sustained, _ := drive(t, 0, 0, 0)
	if sustained != 0 {
		t.Fatalf("sustained = %d, want 0", sustained)
	}
}

// Every candidate the bisection settles on must be re-tested under a full
// measurement window before it becomes the answer - a FAST hold is never the
// last word.
// A ceiling that was only ever established on a SHORT hold is too cheap to be
// the final answer: the FAST phase judges on a few seconds of samples and can
// mistake a cold start for saturation. That one must be re-tested under a full
// window before the run stands behind it.
func TestClimb_AShortHoldVerdictIsNeverTheFinalAnswer(t *testing.T) {
	// A box already working hard (so the FAST batch stays at its base stride and
	// overshoots the ceiling by only one batch) and a ceiling of 30. The FAST
	// phase breaks at 31; every tier the bisection then tries HOLDS, so hi is
	// never re-broken under a full window - it stays a short-hold verdict, and
	// must be confirmed before the run stands behind it.
	c := &climb{target: 1, batch: fastBatch, spawnsEncoders: true}
	seenConfirm := false
	for range 200 {
		if c.target > 30 {
			c.tierBroke(StopCPUSaturated, "cpu")
		}
		if c.phase == phaseConfirm {
			seenConfirm = true
		}
		outcome, _, _ := c.next(0, 0.8) // a busy box: the batch never doubles
		if outcome == climbDone {
			break
		}
	}
	if !seenConfirm {
		t.Fatalf("a ceiling established only on a short hold became the final answer with no confirmation")
	}
	if c.lo < 29 || c.lo > 30 {
		t.Fatalf("sustained = %d, want ~30", c.lo)
	}
}

// The hold and warm-up must be short while hunting and long while judging:
// that asymmetry is the entire reason the climb is both fast AND trustworthy.
func TestClimb_HuntsOnAShortHoldAndJudgesOnALongOne(t *testing.T) {
	c := &climb{phase: phaseFast, spawnsEncoders: true}
	if c.holdFor() != fastHold || c.warmupFor() != fastWarmup {
		t.Fatalf("FAST must hunt on a short hold, got hold=%v warmup=%v", c.holdFor(), c.warmupFor())
	}
	for _, p := range []climbPhase{phaseBisect, phaseConfirm} {
		c.phase = p
		if c.holdFor() != measureHold || c.warmupFor() != measureWarmup {
			t.Fatalf("%v must judge on a full window, got hold=%v warmup=%v", p, c.holdFor(), c.warmupFor())
		}
	}
	if fastHold >= measureHold {
		t.Fatalf("the hunting hold (%v) must be much shorter than the judging one (%v)", fastHold, measureHold)
	}
}

// A box that holds a thousand direct-play viewers must be discovered in a
// handful of tiers, not a hundred: while the machine is idle the batch doubles.
func TestClimb_BigIdleBoxIsFoundInFewTiers(t *testing.T) {
	targets, sustained, reason := drive(t, 1000, 0, 0)
	closeEnough(t, sustained, 1000, targets)
	if reason != StopCPUSaturated {
		t.Fatalf("reason = %s, want a capacity verdict", reason)
	}
	// A +10-at-a-time ramp would need ~100 tiers. Doubling while idle, plus a
	// bisection that stops within a few percent, must get there in a small
	// fraction of that - every tier past the FAST phase costs a full measurement
	// window, so tier count IS wall-clock.
	if len(targets) > 22 {
		t.Fatalf("took %d tiers to find a ceiling of 1000: %v", len(targets), targets)
	}
}

// A pure direct-play run starts no ffmpeg, so it must not sit through an
// encoder's cold-start warm-up at every judged tier. Across a bisection that is
// most of the run's wall clock, spent waiting for something that never happens.
func TestClimb_DirectPlayDoesNotWaitForAnEncoderThatNeverStarts(t *testing.T) {
	transcoding := &climb{phase: phaseBisect, spawnsEncoders: true}
	directPlay := &climb{phase: phaseBisect, spawnsEncoders: false}

	if directPlay.holdFor() >= transcoding.holdFor() {
		t.Fatalf("direct play holds a tier for %v, transcoding for %v: with no encoder to cold-start, it must be markedly shorter",
			directPlay.holdFor(), transcoding.holdFor())
	}
	if directPlay.warmupFor() >= transcoding.warmupFor() {
		t.Fatalf("direct play warm-up %v is not shorter than transcoding's %v", directPlay.warmupFor(), transcoding.warmupFor())
	}
	// And the scenarios must be classified correctly, or the saving lands on the
	// wrong runs.
	if scenarioSpawnsEncoders(ScenarioMkvgo) {
		t.Fatalf("the mkvgo scenario never starts an encoder")
	}
	for _, s := range []string{ScenarioTranscode, ScenarioHeavy, ScenarioBalanced, ScenarioLight, ScenarioNatural} {
		if !scenarioSpawnsEncoders(s) {
			t.Fatalf("scenario %q can put a viewer on the transcode path; it must wait out the cold start", s)
		}
	}
}

// Once the bisection has broken a tier under a FULL measurement window, that IS
// the proof. Spending another whole window "confirming" it re-learns what was
// just measured, and a measurement window is the single most expensive thing the
// run does.
func TestClimb_DoesNotReconfirmWhatTheBisectionAlreadyProved(t *testing.T) {
	// A box with plenty of headroom right up to its cliff: the load stays low
	// (so the FAST batch keeps doubling and overshoots well past the ceiling),
	// and the bisection then breaks a tier of its own under a full window.
	c := &climb{target: 1, batch: fastBatch, spawnsEncoders: true}
	confirms := 0
	brokeDuringBisect := false
	for range 200 {
		if c.phase == phaseConfirm {
			confirms++
		}
		if c.target > 40 {
			c.tierBroke(StopCPUSaturated, "cpu")
			if c.phase == phaseBisect {
				brokeDuringBisect = true
			}
		}
		outcome, _, _ := c.next(0, 0.1) // idle box: never throttles the batch
		if outcome == climbDone {
			break
		}
	}
	if !brokeDuringBisect {
		t.Fatalf("test setup: the bisection never broke a tier, so there is nothing to re-confirm")
	}
	// The bisection broke a tier under a FULL window. That IS the proof; spending
	// another whole window to re-learn it is the most expensive thing the run
	// could do.
	if confirms > 0 {
		t.Fatalf("ran %d confirmation tiers after the bisection had already broken a tier under a full window", confirms)
	}
	if c.lo < 38 || c.lo > 40 {
		t.Fatalf("sustained = %d, want ~40", c.lo)
	}
}
