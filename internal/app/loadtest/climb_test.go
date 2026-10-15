package loadtest

import "testing"

// drive runs the climb against a modelled machine whose windows hold up to
// `ceiling` viewers. failAt, when set, decides a window's fate instead: it
// gets the target and that target's window number (0-based) and says whether
// the window broke. It returns the climb, the targets tried, the answer and
// the reason.
func drive(t *testing.T, ceiling, maxViewers int, failAt func(target, window int) bool) (c *climb, targets []int, sustained int, reason StopReason) {
	t.Helper()
	c = &climb{target: 1, batch: fastBatch, spawnsEncoders: true}
	windowOf := map[int]int{}
	for range 400 {
		targets = append(targets, c.target)
		broke := c.target > ceiling
		if failAt != nil {
			w := 0
			if c.phase == phaseGrid {
				w = windowOf[c.target]
				windowOf[c.target]++
			}
			broke = failAt(c.target, w)
		}
		if broke {
			c.tierBroke(StopSLOBreach, "slow")
		}
		load := float64(c.target) / float64(max(ceiling, 1))
		outcome, r, _ := c.next(maxViewers, load)
		if outcome == climbDone {
			return c, targets, c.lo, r
		}
	}
	t.Fatalf("climb did not finish; targets=%v", targets)
	return nil, nil, 0, ""
}

func gridTallyOf(c *climb, viewers int) *gridTally { return c.tallies[viewers] }

// The FAST hunt only finds the neighbourhood; the answer is the largest grid
// size within a step of the true ceiling, and the reason is what broke above.
func TestClimb_HuntsFastThenAnswersFromTheGrid(t *testing.T) {
	c, targets, sustained, reason := drive(t, 47, 0, nil)
	if reason != StopSLOBreach {
		t.Fatalf("reason = %s, want slo_breach", reason)
	}
	if sustained > 47 || sustained < 47-c.step {
		t.Fatalf("sustained = %d, want within one grid step (%d) under 47 (targets=%v)", sustained, c.step, targets)
	}
	if c.hi <= sustained {
		t.Fatalf("hi = %d, want the first size above the answer that lost", c.hi)
	}
	if g := gridTallyOf(c, sustained); g == nil || g.passed < 4 {
		t.Fatalf("the answer %d rests on %+v, want at least four passing windows", sustained, g)
	}
}

// A size is held when it passes four windows of five: one join transient does
// not condemn it, two do.
func TestClimb_ASizeIsJudgedOnItsShareOfWindows(t *testing.T) {
	// 40 and below always hold; up to 50 fails its first window only; up to
	// 60 fails two; above, everything fails.
	fail := func(target, w int) bool {
		switch {
		case target <= 40:
			return false
		case target <= 50:
			return w == 0
		case target <= 60:
			return w < 2
		}
		return true
	}
	c, targets, sustained, _ := drive(t, 0, 0, fail)
	if sustained < 41 || sustained > 50 {
		t.Fatalf("sustained = %d, want the largest size failing at most one window in five (targets=%v)", sustained, targets)
	}
	low, high := c.gridRange()
	if low > sustained || high < sustained {
		t.Fatalf("range [%d, %d] must hold the answer %d", low, high, sustained)
	}
	if low >= sustained {
		t.Fatalf("low = %d: the answer %d failed one window, the bottom of the range is the last size that failed none", low, sustained)
	}
	if high <= sustained {
		t.Fatalf("high = %d: a size passing three windows of five sits inside the range, above the answer %d (targets=%v)", high, sustained, targets)
	}
}

// A size is decided as soon as the remaining windows cannot change it: four
// passes is held, three failures is lost.
func TestClimb_StopsMeasuringASizeOnceItIsDecided(t *testing.T) {
	c, _, _, _ := drive(t, 30, 0, nil)
	for v, g := range c.tallies {
		if g.windows > gridWindows {
			t.Fatalf("size %d measured %d windows, at most %d", v, g.windows, gridWindows)
		}
		if g.class() == "held" && g.windows != 4 {
			t.Fatalf("size %d held after %d windows, want 4 (decided at four passes)", v, g.windows)
		}
		if g.class() == "lost" && g.windows != 3 {
			t.Fatalf("size %d lost after %d windows, want 3 (decided at three failures)", v, g.windows)
		}
	}
}

// When the first grid size already loses (FAST survived a size the server
// cannot hold under full windows), the grid walks down until a size holds.
func TestClimb_WalksDownWhenTheFirstGridSizeLoses(t *testing.T) {
	c := &climb{target: 1, batch: fastBatch}
	// FAST: the short holds survive up to the fourth tier, the fifth breaks.
	for _, held := range []bool{true, true, true, true, false} {
		if !held {
			c.tierBroke(StopSLOBreach, "slow")
		}
		c.next(0, 0.1)
		if c.phase == phaseGrid {
			break
		}
	}
	if c.phase != phaseGrid {
		t.Fatalf("still in FAST at %d", c.target)
	}
	first := c.target
	ceiling := first / 2
	for range 300 {
		if c.target > ceiling {
			c.tierBroke(StopSLOBreach, "slow")
		}
		if out, _, _ := c.next(0, 0.5); out == climbDone {
			break
		}
	}
	if c.lo == 0 || c.lo > ceiling || c.lo < ceiling-c.step {
		t.Fatalf("answer = %d, want within a step under %d, found walking down from %d", c.lo, ceiling, first)
	}
}

// The FAST hunt decides nothing about the answer: a phantom break on a short
// hold only places the grid, which then climbs past it.
func TestClimb_AFastPhantomDoesNotCapTheAnswer(t *testing.T) {
	phantomUsed := false
	fail := func(target, w int) bool {
		if target == 21 && !phantomUsed {
			phantomUsed = true
			return true
		}
		return target > 47
	}
	_, targets, sustained, _ := drive(t, 0, 0, fail)
	if sustained < 40 {
		t.Fatalf("sustained = %d: a single short-hold phantom at 21 capped the run (targets=%v)", sustained, targets)
	}
}

// The operator's ceiling is not a capacity verdict, and it is measured on full
// windows before it is reported.
func TestClimb_OperatorCeilingIsMeasuredThenReported(t *testing.T) {
	c, _, sustained, reason := drive(t, 1000, 25, nil)
	if reason != StopMaxViewers || sustained != 25 {
		t.Fatalf("got %d (%s), want 25 (max_viewers)", sustained, reason)
	}
	if g := gridTallyOf(c, 25); g == nil || g.passed < 4 {
		t.Fatalf("the ceiling was reported on %+v, want full windows", g)
	}
}

// A server that cannot hold even one viewer reports zero.
func TestClimb_ZeroCapacity(t *testing.T) {
	_, _, sustained, reason := drive(t, 0, 0, nil)
	if sustained != 0 {
		t.Fatalf("sustained = %d, want 0", sustained)
	}
	if reason != StopSLOBreach {
		t.Fatalf("reason = %s, want what broke the single viewer", reason)
	}
}

// FAST hunts on a short hold, the grid judges on a long one, and a run that
// starts no encoder does not wait for one.
func TestClimb_HoldsFollowThePhase(t *testing.T) {
	c := &climb{phase: phaseFast, spawnsEncoders: true}
	if c.holdFor() != fastHold || c.warmupFor() != fastWarmup {
		t.Fatalf("FAST hold %v warm-up %v", c.holdFor(), c.warmupFor())
	}
	c.phase = phaseGrid
	if c.holdFor() != measureHold || c.warmupFor() != measureWarmup {
		t.Fatalf("grid hold %v warm-up %v", c.holdFor(), c.warmupFor())
	}
	c.spawnsEncoders = false
	if c.holdFor() != directPlayMeasureHold || c.warmupFor() != directPlayMeasureWarmup {
		t.Fatalf("direct-play grid hold %v warm-up %v", c.holdFor(), c.warmupFor())
	}
}

// A big idle box is still found in a handful of FAST tiers.
func TestClimb_BigIdleBoxIsFoundInFewFastTiers(t *testing.T) {
	c, targets, sustained, _ := drive(t, 600, 0, nil)
	fast := 0
	for _, tg := range targets {
		if c.tallies[tg] == nil {
			fast++
		}
	}
	if fast > 10 {
		t.Fatalf("%d FAST tiers to find 600: %v", fast, targets)
	}
	if sustained < 600-c.step {
		t.Fatalf("sustained = %d, want within a step of 600", sustained)
	}
}

// The grid report lists every measured size with its windows, smallest first.
func TestClimb_GridReport(t *testing.T) {
	c, _, _, _ := drive(t, 47, 0, nil)
	rep := c.gridReport()
	if len(rep) < 2 {
		t.Fatalf("grid report = %+v", rep)
	}
	for i, g := range rep {
		if i > 0 && rep[i-1].Viewers >= g.Viewers {
			t.Fatalf("not sorted: %+v", rep)
		}
		if g.Held != (g.Passed >= 4) {
			t.Fatalf("held flag wrong: %+v", g)
		}
		if !g.Held && g.Reason == "" {
			t.Fatalf("a size that did not hold names its reason: %+v", g)
		}
	}
}
