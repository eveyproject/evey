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

// THE 21-VIEWER REGRESSION, second act. Straight from a dev run: the climb held
// 21 on a FAST hold, then 31, 26, 23 and 22 all broke under full windows, so the
// bisection converged and the run stood behind 21 - a tier it had only ever
// observed for a few seconds, in the middle of a ten-viewer join burst. The
// report's headline CPU for "21 viewers" was 61% (the spawn burst), against 1-9%
// for every bigger tier, and the per-viewer column an operator sizes a machine
// with was garbage.
//
// The answer the run stands behind must ALWAYS have been measured under a full
// window. If it only ever held a FAST hold, the climb owes it one real
// measurement before calling it the ceiling.
func TestClimb_TheAnswerIsNeverLeftOnFastEvidence(t *testing.T) {
	c := &climb{target: 1, batch: fastBatch, spawnsEncoders: true}
	measuredAnswer := false
	var reason StopReason
	done := false
	for range 200 {
		if c.phase != phaseFast && c.target == 21 {
			measuredAnswer = true
		}
		// The machine holds exactly 21: every bigger tier breaks, in ANY phase.
		if c.target > 21 {
			c.tierBroke(StopCPUSaturated, "cpu")
		}
		// A working box (load between the growth ceiling and the caution floor),
		// so the FAST stride stays at 10 and the climb walks 1, 11, 21, 31 - the
		// production shape.
		outcome, r, _ := c.next(0, 0.6)
		if outcome == climbDone {
			reason, done = r, true
			break
		}
	}
	if !done {
		t.Fatalf("climb did not converge")
	}
	if c.lo != 21 {
		t.Fatalf("sustained = %d, want 21", c.lo)
	}
	if !measuredAnswer {
		t.Fatalf("the verdict tier (21) was never held under a full measurement window: the report's resource figures for the answer come from a %v join-burst sample", fastHold)
	}
	if reason != StopCPUSaturated {
		t.Fatalf("reason = %s, want %s (the verdict must survive the final measurement)", reason, StopCPUSaturated)
	}
}

// And the counterpart: if the answer FAILS its own final measurement, it was
// never the answer. The climb must resume the search below it rather than
// report a tier it just watched break.
func TestClimb_AnAnswerThatFailsItsOwnMeasurementIsNotTheAnswer(t *testing.T) {
	c := &climb{target: 1, batch: fastBatch, spawnsEncoders: true}
	for range 200 {
		// The machine really holds 20. A FAST hold is too short to notice 21
		// struggling (it only breaks past 30 there), but any full window sees
		// the truth.
		if (c.phase == phaseFast && c.target > 30) || (c.phase != phaseFast && c.target > 20) {
			c.tierBroke(StopCPUSaturated, "cpu")
		}
		outcome, _, _ := c.next(0, 0.6)
		if outcome == climbDone {
			if c.lo > 20 {
				t.Fatalf("sustained = %d, want <= 20: the run stood behind a tier that broke under its own final measurement", c.lo)
			}
			if c.lo < 19 {
				t.Fatalf("sustained = %d, want ~20: the search below the disproven answer gave up too much", c.lo)
			}
			return
		}
	}
	t.Fatalf("climb did not converge")
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

// driveNoisy is drive with a machine whose full-window measurements can lie:
// falseBreak decides, for the n-th time a given target is judged in a given
// phase, whether to report a break the machine does not actually have. The real
// ceiling is `ceiling`; every break above it reproduces.
func driveNoisy(t *testing.T, ceiling, maxViewers int, falseBreak func(phase climbPhase, target, n int) bool) (targets []int, sustained int, reason StopReason) {
	t.Helper()
	c := &climb{target: 1, batch: fastBatch, spawnsEncoders: true}
	seen := map[[2]int]int{}

	for range 300 {
		targets = append(targets, c.target)
		key := [2]int{int(c.phase), c.target}
		n := seen[key]
		seen[key]++
		if c.target > ceiling || falseBreak(c.phase, c.target, n) {
			c.tierBroke(StopSLOBreach, "p95")
		}
		load := float64(c.target) / float64(max(ceiling, 1))
		outcome, r, _ := c.next(maxViewers, load)
		if outcome == climbDone {
			return targets, c.lo, r
		}
	}
	t.Fatalf("climb did not converge; targets=%v", targets)
	return nil, 0, ""
}

// The 2026-10-04 run: the machine held 63, but the settle window at 61 read
// 5879 ms against a 5500 ms budget once. The answer was thrown away and the
// bisection restarted from zero, landing on 29. A break that has not
// reproduced in a second window is noise: the answer stands.
func TestClimb_ASingleNoisyWindowDoesNotThrowTheAnswerAway(t *testing.T) {
	targets, sustained, reason := driveNoisy(t, 62, 80, func(p climbPhase, target, n int) bool {
		return p == phaseSettle && n == 0 // the first settle window lies, the second tells the truth
	})
	if reason != StopSLOBreach {
		t.Fatalf("reason = %s, want %s", reason, StopSLOBreach)
	}
	closeEnough(t, sustained, 62, targets)
	for i, tg := range targets {
		if tg > 0 && tg < sustained-sustained*bisectTolerancePercent/100-1 && i > len(targets)/2 {
			t.Fatalf("the search dropped to %d after the settle window: the noisy break was believed (targets=%v)", tg, targets)
		}
	}
}

// A noisy first window in the bisection must not push hi down either: the tier
// is measured again, holds, and the search keeps climbing towards the truth.
func TestClimb_ABisectionBreakThatDoesNotReproduceIsNotAVerdict(t *testing.T) {
	fired := false
	targets, sustained, _ := driveNoisy(t, 100, 0, func(p climbPhase, target, n int) bool {
		// The first bisection tier the machine actually holds lies once.
		if p == phaseBisect && target <= 100 && n == 0 && !fired {
			fired = true
			return true
		}
		return false
	})
	if !fired {
		t.Fatalf("the scenario never reached a bisection tier below the ceiling (targets=%v): the test proves nothing", targets)
	}
	closeEnough(t, sustained, 100, targets)
}

// The retry must not be a loophole: a tier above the real ceiling breaks in
// both windows and still condemns.
func TestClimb_ABreakThatReproducesStillCondemnsTheTier(t *testing.T) {
	targets, sustained, reason := driveNoisy(t, 47, 0, func(climbPhase, int, int) bool { return false })
	closeEnough(t, sustained, 47, targets)
	if reason != StopSLOBreach {
		t.Fatalf("reason = %s, want %s", reason, StopSLOBreach)
	}
}

// Each full-window tier that breaks is measured exactly twice before being
// believed: never a third time (that would be an unbounded retry loop).
func TestClimb_AFullWindowBreakIsRetriedExactlyOnce(t *testing.T) {
	c := &climb{phase: phaseBisect, lo: 10, hi: 80, target: 45, batch: fastBatch}
	c.tierBroke(StopSLOBreach, "p95")
	if outcome, _, _ := c.next(0, 0.5); outcome != climbContinue || c.target != 45 || !c.retried || c.hi != 80 {
		t.Fatalf("first break: outcome=%v target=%d retried=%v hi=%d, want the same tier measured again with hi untouched", outcome, c.target, c.retried, c.hi)
	}
	c.tierBroke(StopSLOBreach, "p95")
	if _, _, _ = c.next(0, 0.5); c.hi != 45 || c.retried {
		t.Fatalf("second break: hi=%d retried=%v, want hi=45 (a reproduced break counts) and the retry flag cleared", c.hi, c.retried)
	}
}
