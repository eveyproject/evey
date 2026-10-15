package loadtest

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// climb.go - how the run finds the machine's ceiling, and how sure it is.
//
// Two phases:
//
//   FAST - jump by whole batches on a short hold until something gives out.
//          It only finds the neighbourhood of the ceiling: a five-second hold
//          inside a join burst decides nothing about the answer, it only says
//          where the grid below starts and how wide its steps are.
//   GRID - fixed audience sizes, a tenth of the FAST break apart, climbed in
//          ascending order (shedding viewers leaves the disks a backlog that
//          condemns the smaller audience for the larger one's sins, measured
//          2026-10-15: the same 101 broke right after 141 and held later).
//          Each size is held for several full windows and judged on the share
//          of windows that passed. The answer is the largest size that passes
//          most of its windows, with the range around it.
//
// Why repeated windows and not a bisection: one window's verdict flips on a
// handful of slow segments (the same 131 saw its busiest disk at 41 ms then
// 12 ms a window later), and a bisection that branches on each flip lands
// somewhere else every run (78, 126 and 121 viewers for three runs of one
// server). A pass rate over several windows is a measurement; a single break
// is an anecdote.

type climbPhase int

const (
	phaseFast climbPhase = iota
	phaseGrid
)

func (p climbPhase) String() string {
	if p == phaseGrid {
		return "measuring"
	}
	return "ramping"
}

const (
	// fastBatch is how many viewers join at once while the server still has
	// room. Ten at a time turns a 200-viewer box from a 200-step crawl into a
	// 20-step sprint.
	fastBatch = 10
	// maxFastBatch caps how big a batch can grow. While the machine is still
	// idle the batch DOUBLES, so the climb reaches the interesting region in a
	// handful of tiers - the grid afterwards pays the imprecision back.
	maxFastBatch = 320
	// batchGrowthCeiling / batchCautionFloor: below the first load (0..1, the
	// most-loaded resource) the batch doubles, above the second it drops back
	// to fastBatch.
	batchGrowthCeiling = 0.5
	batchCautionFloor  = 0.75
	// fastHold / fastWarmup: how long a FAST tier is observed and how much of
	// that is thrown away. Short on purpose: this phase only asks "did
	// anything break", never "how many does it hold".
	fastHold   = 5 * time.Second
	fastWarmup = 1500 * time.Millisecond

	// measureHold / measureWarmup: one GRID window for a run that starts
	// encoders, and the warm-up discarded when an audience size is first
	// reached. The window must cover an ffmpeg cold start (up to ~25 s).
	measureHold   = 30 * time.Second
	measureWarmup = 12 * time.Second

	// directPlayMeasureHold / directPlayMeasureWarmup: the same for a run that
	// spawns no encoder - a viewer is in steady state seconds after joining.
	directPlayMeasureHold   = 15 * time.Second
	directPlayMeasureWarmup = 4 * time.Second
)

// Grid parameters, read off the 2026-10-15 runs on the production box.
//
// Windows at one audience size flipped between held and broken on roughly one
// window in three near the ceiling and one in ten a step below it. With five
// windows, a size whose true failure rate is one in ten passes four or more
// 92 % of the time, and one whose failure rate is one in two does so only
// 19 % of the time: five windows separate "holds" from "coin toss", and the
// 80 % bar tolerates one join transient without condemning the size.
const (
	// gridWindows is how many full windows each audience size is judged on.
	gridWindows = 5
	// gridPassShare is the share of windows a size must pass to be held: the
	// answer is the largest size at or above it.
	gridPassShare = 0.8
	// gridHalfShare bounds the range: the largest size passing at least half
	// its windows is the top of "between A and B", and the first size below
	// it ends the climb.
	gridHalfShare = 0.5
	// gridStepDivisor sets the grid's step as a share of the FAST break: a
	// tenth of the ceiling, the precision the variance allows.
	gridStepDivisor = 10
)

// gridTally is one audience size's record in the grid.
type gridTally struct {
	viewers int
	windows int
	passed  int
	// of is how many windows the size is judged on (gridWindows unless a
	// test shrinks it).
	of int
	// reasons counts what broke the failed windows.
	reasons map[StopReason]int
	details map[StopReason]string
}

func (g *gridTally) failed() int { return g.windows - g.passed }

// class is what the tally already says for sure, deciding before all windows
// are in when the rest cannot change it: "held" (enough passes for
// gridPassShare), "lost" (too many failures for gridHalfShare), "between", or
// "" while undecided.
func (g *gridTally) class() string {
	total := g.of
	if total <= 0 {
		total = gridWindows
	}
	needHeld := int(math.Ceil(gridPassShare * float64(total)))
	needHalf := int(math.Ceil(gridHalfShare * float64(total)))
	maxFailHeld := total - needHeld
	maxFailHalf := total - needHalf
	switch {
	case g.passed >= needHeld:
		return "held"
	case g.failed() > maxFailHalf:
		return "lost"
	case g.failed() > maxFailHeld && g.passed >= needHalf:
		return "between"
	case g.windows >= total:
		if g.passed >= needHalf {
			return "between"
		}
		return "lost"
	}
	return ""
}

// mainReason is the signal that broke most of this size's failed windows.
func (g *gridTally) mainReason() (StopReason, string) {
	var best StopReason
	n := -1
	for r, c := range g.reasons {
		if c > n || (c == n && r < best) {
			best, n = r, c
		}
	}
	return best, g.details[best]
}

// gridChurn is how many viewers of an audience are replaced at each new window
// of the same size: the oldest leave and newcomers join. A real audience
// is never frozen, and without newcomers the windows after the first would
// never measure how long a new viewer waits to start - the one figure a
// person holding the remote notices first.
//
// At least startupMinSamples newcomers per window, so every window carries
// enough first fetches to judge startup on; a tenth of the audience past
// thirty viewers. Never the whole audience minus nothing: at most all of it.
func gridChurn(tier int) int { return max(1, min(tier, max(startupMinSamples, tier/10))) }

// GridTier is one audience size of the grid, for the report.
type GridTier struct {
	Viewers, Windows, Passed int
	Held                     bool
	Reason                   StopReason
	// Resources is what the machine did over all the size's windows (the
	// server's CPU share).
	Resources ResourceStats
}

// climb is the ramp's search state.
type climb struct {
	phase climbPhase
	// lo is the answer so far: the largest size proven to hold (FAST: survived
	// a short hold; GRID: passed gridPassShare of its windows). hi is the
	// lowest size seen to fail (0 = none).
	lo, hi int
	// target is the viewer count currently being held.
	target int

	// broke records what gave out during the CURRENT window. Cleared whenever
	// a window starts.
	broke  StopReason
	detail string

	// hiReason is why hi failed - the verdict the report carries.
	hiReason StopReason
	hiDetail string

	batch          int
	spawnsEncoders bool

	// The grid: its step, every size measured so far, and the direction it is
	// walking (+1 up, -1 down below a first size that did not hold).
	step      int
	tallies   map[int]*gridTally
	direction int
	// sameTier is true when next() kept the target for another window of the
	// same size: the run loop then opens a window without a resize.
	sameTier bool
	// windows overrides gridWindows (tests); zero means gridWindows.
	windows int

	fastHold, fastWarmup       time.Duration
	measureHold, measureWarmup time.Duration
}

// growBatch adapts the FAST stride to how hard the server is actually working.
func (c *climb) growBatch(load float64) {
	switch {
	case load >= batchCautionFloor:
		// Near the ceiling the stride drops to the grid's own precision - a
		// tenth of the audience - not to ten viewers: the grid re-measures
		// the neighbourhood anyway, and a crawl of tens on a big box spent
		// minutes on holds that decide nothing.
		c.batch = max(fastBatch, c.target/gridStepDivisor)
	case load < batchGrowthCeiling:
		c.batch = min(max(c.batch, fastBatch)*2, maxFastBatch)
	}
}

// tierBroke records that something gave out in the current window. The first
// signal wins.
func (c *climb) tierBroke(reason StopReason, detail string) {
	if c.broke == "" {
		c.broke, c.detail = reason, detail
	}
}

// holdFor is how long the current phase observes a window.
func (c *climb) holdFor() time.Duration {
	if c.phase == phaseFast {
		return orDefault(c.fastHold, fastHold)
	}
	if !c.spawnsEncoders {
		return orDefault(c.measureHold, directPlayMeasureHold)
	}
	return orDefault(c.measureHold, measureHold)
}

// warmupFor is how much of a newly reached size is discarded as transient.
func (c *climb) warmupFor() time.Duration {
	if c.phase == phaseFast {
		return orDefault(c.fastWarmup, fastWarmup)
	}
	if !c.spawnsEncoders {
		return orDefault(c.measureWarmup, directPlayMeasureWarmup)
	}
	return orDefault(c.measureWarmup, measureWarmup)
}

func orDefault(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}

// climbOutcome is what the run loop must do after a window's hold expires.
type climbOutcome int

const (
	// climbContinue: a target was chosen (the same one when sameTier); keep going.
	climbContinue climbOutcome = iota
	// climbDone: the answer is known. lo is it.
	climbDone
)

// climbBy bumps the target by the current batch, respecting the operator's
// ceiling.
func (c *climb) climbBy(maxViewers int) {
	next := c.target + max(c.batch, fastBatch)
	if maxViewers > 0 {
		next = min(next, maxViewers)
	}
	c.target = next
}

// startGrid leaves FAST: the grid starts at the last size that survived a
// short hold (or at breakAt when nothing did) and steps a tenth of the size
// that broke.
func (c *climb) startGrid(breakAt int) {
	c.phase = phaseGrid
	c.step = max(1, int(math.Round(float64(breakAt)/gridStepDivisor)))
	c.tallies = map[int]*gridTally{}
	c.direction = +1
	start := c.lo
	if start <= 0 {
		start = max(1, breakAt-c.step)
	}
	c.lo, c.hi, c.hiReason, c.hiDetail = 0, 0, "", ""
	c.target = start
	c.sameTier = false
}

// next judges the window that just finished and decides where to go.
func (c *climb) next(maxViewers int, load float64) (climbOutcome, StopReason, string) {
	brokeReason, brokeDetail := c.broke, c.detail
	held := brokeReason == ""
	c.broke, c.detail = "", ""
	c.sameTier = false

	if c.phase == phaseFast {
		if held {
			c.lo = c.target
			if maxViewers > 0 && c.target >= maxViewers {
				// Climbed as far as the operator allowed: measure that size
				// properly before calling it.
				c.startGrid(c.target)
				c.target = maxViewers
				return climbContinue, "", ""
			}
			c.growBatch(load)
			c.climbBy(maxViewers)
			return climbContinue, "", ""
		}
		c.startGrid(c.target)
		return climbContinue, "", ""
	}

	// GRID: one more window of the current size.
	t := c.tallies[c.target]
	if t == nil {
		t = &gridTally{viewers: c.target, of: c.windows, reasons: map[StopReason]int{}, details: map[StopReason]string{}}
		c.tallies[c.target] = t
	}
	t.windows++
	if held {
		t.passed++
	} else {
		t.reasons[brokeReason]++
		if _, ok := t.details[brokeReason]; !ok {
			t.details[brokeReason] = brokeDetail
		}
	}
	cls := t.class()
	if cls == "" {
		c.sameTier = true
		return climbContinue, "", ""
	}
	if cls == "held" {
		c.lo = max(c.lo, c.target)
	} else if c.hi == 0 || c.target < c.hi {
		c.hi = c.target
		c.hiReason, c.hiDetail = t.mainReason()
	}

	switch {
	case c.direction > 0 && cls == "lost":
		// The climb ends at the first size that loses most of its windows.
		// Below the first size measured, walk down until one holds.
		if c.lo == 0 && c.target > 1 {
			c.direction = -1
			c.target = c.nextDown()
			return climbContinue, "", ""
		}
		return c.done()
	case c.direction > 0:
		if maxViewers > 0 && c.target >= maxViewers {
			return c.done()
		}
		nxt := c.target + c.step
		if maxViewers > 0 {
			nxt = min(nxt, maxViewers)
		}
		c.target = nxt
		return climbContinue, "", ""
	default: // walking down
		if cls == "held" || c.target <= 1 {
			return c.done()
		}
		c.target = c.nextDown()
		return climbContinue, "", ""
	}
}

// nextDown is the next size below the current one on the way down, never
// below one viewer.
func (c *climb) nextDown() int { return max(1, c.target-c.step) }

// done ends the climb: lo is the answer. The reason is what broke the first
// size above it, or the operator's ceiling when nothing did.
func (c *climb) done() (climbOutcome, StopReason, string) {
	// The cause is read on the first size ABOVE the answer that did not hold:
	// a size below it that lost a window or two along the way is noise the
	// answer already outlived.
	c.hi, c.hiReason, c.hiDetail = 0, "", ""
	for v, t := range c.tallies {
		if v > c.lo && t.class() != "held" && (c.hi == 0 || v < c.hi) {
			c.hi = v
			c.hiReason, c.hiDetail = t.mainReason()
		}
	}
	if c.hi == 0 {
		return climbDone, StopMaxViewers, fmt.Sprintf("reached %d viewers with room to spare", c.lo)
	}
	return climbDone, c.hiReason, c.hiDetail
}

// gridRange is the answer's range: low is the largest size that passed EVERY
// window (at most the answer), high the largest size that passed at least
// half (at least the answer). Both are the answer when the grid has nothing
// to say either way.
func (c *climb) gridRange() (low, high int) {
	low, high = c.lo, c.lo
	best := 0
	for _, t := range c.tallies {
		if t.windows > 0 && t.failed() == 0 && t.viewers <= c.lo && t.viewers > best {
			best = t.viewers
		}
		if t.passed*2 >= t.windows && t.windows > 0 && t.viewers > high {
			high = t.viewers
		}
	}
	if best > 0 {
		low = best
	}
	return low, high
}

// gridReport lists every size the grid measured, smallest first.
func (c *climb) gridReport() []GridTier {
	out := make([]GridTier, 0, len(c.tallies))
	for _, t := range c.tallies {
		reason, _ := t.mainReason()
		held := t.class() == "held"
		if held {
			reason = ""
		}
		out = append(out, GridTier{Viewers: t.viewers, Windows: t.windows, Passed: t.passed, Held: held, Reason: reason})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Viewers < out[j].Viewers })
	return out
}
