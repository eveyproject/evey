package loadtest

import (
	"fmt"
	"math"
	"time"
)

// climb.go - how the run finds the machine's ceiling FAST.
//
// Adding one viewer at a time and waiting for a full steady-state window at
// every step is honest but useless: on a box that holds 200 direct-play viewers
// it would take hours to say so. What an operator wants is the highest number
// the server actually holds, found as quickly as the measurement allows.
//
// So the climb runs in three phases:
//
//   FAST    - jump by whole batches (10 viewers) on a short hold. The goal is
//             not to measure, it is to find the neighbourhood of the ceiling:
//             keep doubling down until something gives out.
//   BISECT  - once a batch breaks and CONFIRM agrees, the answer lies between
//             the last batch that held (lo) and the one that broke (hi). Halve the interval, hold
//             each candidate long enough to judge it properly, and repeat.
//             log2(step) rounds, not step rounds.
//   CONFIRM - the FAST phase judges on a short hold, so it can cry wolf (an
//             ffmpeg cold start looks like a CPU spike). Before searching below
//             a FAST break, re-test it in steady state. If hi actually holds, the
//             break was a phantom: adopt it and go back to FAST. Without this,
//             one bad reading at the first batch would cap the whole run.
//   SETTLE  - the mirror image of CONFIRM, for the ANSWER instead of the break.
//             When the verdict lands on a lo that only ever held a FAST hold,
//             everything the report would say about it (CPU, memory, per-viewer
//             cost) comes from a few seconds inside a ten-viewer join burst. One
//             last full window at lo makes the headline figures real - and if lo
//             fails its own measurement, it was never the answer: the search
//             resumes below it.
//
// Every measured candidate (BISECT, CONFIRM, SETTLE) is judged sequentially
// on windows of the measurement hold, not on one: held at the fourth passing
// window, broken at the second failing one, at most five. One window's verdict
// flips on a handful of slow segments (the same 131 viewers saw their busiest
// disk at 41 ms then 12 ms a window later, 2026-10-15), and a bisection that
// branches on each flip lands somewhere else every run. With five windows a
// size failing one window in ten is held 92 % of the time, one failing every
// other window only 19 %.
//
// The bisection needs to REMOVE viewers, not just add them, which is why every
// viewer owns a cancellable context of its own (see rampState.cancelBySlot).

type climbPhase int

const (
	phaseFast climbPhase = iota
	phaseBisect
	phaseConfirm
	phaseSettle
)

func (p climbPhase) String() string {
	switch p {
	case phaseFast:
		return "ramping"
	case phaseBisect:
		return "narrowing"
	case phaseConfirm:
		return "confirming"
	case phaseSettle:
		return "measuring"
	default:
		return "ramping"
	}
}

const (
	// fastBatch is how many viewers join at once while the server still has
	// room. Ten at a time turns a 200-viewer box from a 200-step crawl into a
	// 20-step sprint.
	fastBatch = 10
	// maxFastBatch caps how big a batch can grow. A direct-play viewer can cost
	// the server almost nothing, so a box may hold hundreds; adding 10 at a time
	// would take a hundred tiers to find that out. While the machine is still
	// idle the batch DOUBLES, so the climb reaches the interesting region in a
	// handful of tiers - and the bisection afterwards pays the imprecision back.
	maxFastBatch = 320
	// batchGrowthCeiling is the load (0..1, the most-loaded resource) below
	// which the server is "wide open" and the batch may double. Above
	// batchCautionFloor it is clearly working, and the batch drops back to
	// fastBatch so the bisection interval stays narrow near the ceiling.
	batchGrowthCeiling = 0.5
	batchCautionFloor  = 0.75
	// fastHold is how long a FAST tier is observed. Short on purpose: this
	// phase only asks "did anything break", never "what does it cost".
	fastHold = 5 * time.Second
	// fastWarmup discards the first moments of a FAST tier (the joining
	// viewers' cold start), leaving a few real samples behind.
	fastWarmup = 1500 * time.Millisecond

	// measureHold / measureWarmup are how long a BISECT/CONFIRM candidate is held,
	// and how much of that is thrown away as start-up transient. This is where the
	// real judgement happens, so the window must cover an ffmpeg COLD START (up to
	// ~25s worst case, see transcode_vod_wire.go) plus a genuine steady-state
	// stretch.
	measureHold   = 30 * time.Second
	measureWarmup = 12 * time.Second

	// directPlayMeasureHold / directPlayMeasureWarmup are the same windows for a
	// run that spawns NO ENCODER at all.
	//
	// Direct play has no ffmpeg to cold-start: a viewer bursts a few segments to
	// fill its buffer and is then in steady state, which takes seconds, not
	// half a minute. Waiting out an encoder's warm-up on a run that never starts
	// one is pure dead time - and with a bisection holding several tiers, it is
	// most of the run's wall clock. Half the window, same evidence.
	directPlayMeasureHold   = 15 * time.Second
	directPlayMeasureWarmup = 4 * time.Second
)

// bisectTolerancePercent is how close the bisection has to get before it calls
// it a day. Chasing the exact viewer costs a full measurement window per step
// (log2 of the interval), and nobody acts differently on "58" than on "61": a
// few percent of the answer is noise anyway, since the next run on the same box
// will land a viewer or two either side. Stopping at 5% turns a six-step
// bisection into two or three, which is minutes off every run.
const bisectTolerancePercent = 5

// climb is the ramp's search state: everything known about where the ceiling
// is.
type climb struct {
	phase climbPhase
	// lo is the highest viewer count PROVEN to hold. hi is the lowest count
	// seen to break (0 = nothing has broken yet).
	lo, hi int
	// target is the viewer count currently being held.
	target int

	// broke records what gave out during the CURRENT tier, if anything. It is
	// cleared whenever a new tier starts.
	broke  StopReason
	detail string

	// hiReason is why hi broke - the verdict the report ultimately carries.
	hiReason StopReason
	hiDetail string

	// batch is how many viewers the next FAST tier adds. It grows while the
	// machine is idle and collapses back to fastBatch as it starts to work.
	batch int

	// spawnsEncoders is false for a scenario that never starts an ffmpeg (pure
	// direct play). Such a run needs a far shorter measurement window, because
	// there is no cold start to wait out.
	spawnsEncoders bool

	// hiJudgedProperly records that hi broke under a FULL measurement window
	// rather than a FAST hold. When it did, the confirmation pass has nothing
	// left to check and is skipped - it would cost a whole extra window to
	// re-learn what the bisection just proved.
	hiJudgedProperly bool

	// loMeasured records that the current lo HELD under a full measurement
	// window at some point. The answer the run stands behind must never rest on
	// a FAST hold alone - a join burst is not a regime - so a verdict landing on
	// an unmeasured lo first routes through phaseSettle.
	loMeasured bool
	// measuredLo is the highest count that held under a full measurement
	// window: what a failed settle falls back to.
	measuredLo int

	// cur is the measured candidate being judged, steps every one judged so
	// far in order (the report's table); sameTier is true when next() kept the
	// target for another window of the same candidate: the run loop then opens
	// a window without a resize. windows overrides stepWindows (tests).
	cur      *stepTally
	steps    []*stepTally
	sameTier bool
	windows  int

	// pendingReason/pendingDetail hold the verdict found while the answer still
	// needed its own measurement; delivered once the settle pass holds.
	pendingReason StopReason
	pendingDetail string

	// The hold/warm-up windows for each phase. Real runs use the constants
	// above; tests shrink them so a whole climb takes milliseconds instead of
	// minutes. Zero means "use the constant".
	fastHold, fastWarmup       time.Duration
	measureHold, measureWarmup time.Duration
}

// growBatch adapts the FAST stride to how hard the server is actually working.
// load is the most-loaded resource as a fraction of its ceiling (0..1).
//
// Wide open -> double the stride: an idle box that holds hundreds of direct-play
// viewers must not be discovered ten at a time. Working hard -> back to the base
// stride, so the interval the bisection later has to search stays small.
func (c *climb) growBatch(load float64) {
	switch {
	case load >= batchCautionFloor:
		c.batch = fastBatch
	case load < batchGrowthCeiling:
		c.batch = min(max(c.batch, fastBatch)*2, maxFastBatch)
	}
}

// tierBroke records that something gave out at the current tier. The first
// signal wins: a server that both saturates its CPU and misses its latency
// budget is CPU-bound, and the extra signal adds nothing.
func (c *climb) tierBroke(reason StopReason, detail string) {
	if c.broke == "" {
		c.broke, c.detail = reason, detail
	}
}

// holdFor is how long the current phase observes a tier before judging it:
// short while hunting for the neighbourhood of the ceiling, long while deciding
// whether a candidate really holds. That asymmetry is what makes the climb both
// fast and trustworthy.
func (c *climb) holdFor() time.Duration {
	if c.phase == phaseFast {
		return orDefault(c.fastHold, fastHold)
	}
	if !c.spawnsEncoders {
		return orDefault(c.measureHold, directPlayMeasureHold)
	}
	return orDefault(c.measureHold, measureHold)
}

// warmupFor is how much of that hold is discarded as start-up transient.
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

// climbOutcome is what the run loop must do after a tier's hold expires.
type climbOutcome int

const (
	// climbContinue: a new target was chosen; resize and keep going.
	climbContinue climbOutcome = iota
	// climbDone: the ceiling is known. lo is the answer.
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

// converged reports that the bisection has narrowed the answer enough to stop.
// The interval need not close to a single viewer: within a few percent, the
// remaining steps cost a full measurement window each and buy nothing an
// operator would act on.
func (c *climb) converged() bool {
	tolerance := max(1, c.lo*bisectTolerancePercent/100)
	return c.hi-c.lo <= tolerance
}

// finish delivers the run's verdict - unless the answer it is about to stand
// behind was only ever observed on a FAST hold. Everything the report says
// about the answer (its CPU, its memory, the per-viewer cost) comes from that
// tier's samples, and a FAST hold's samples sit inside a join burst. One last
// full window at lo settles it; the verdict waits in pendingReason meanwhile.
func (c *climb) finish(reason StopReason, detail string) (climbOutcome, StopReason, string) {
	if c.lo <= 0 || c.loMeasured {
		return climbDone, reason, detail
	}
	c.pendingReason, c.pendingDetail = reason, detail
	c.phase = phaseSettle
	c.target = c.lo
	return climbContinue, "", ""
}

// next judges the tier that just finished and decides where to go. maxViewers
// is the operator's ceiling (0 = unbounded: climb until something gives out).
// load is how hard the server was working during the tier - the most-loaded
// resource as a fraction of its ceiling - which sets how big a stride the next
// FAST tier dares take.
func (c *climb) next(maxViewers int, load float64) (climbOutcome, StopReason, string) {
	// Take the window's verdict, then clear it so the next window starts clean.
	brokeReason, brokeDetail := c.broke, c.detail
	held := brokeReason == ""
	c.broke, c.detail = "", ""
	c.sameTier = false

	// A measured candidate is decided on its windows, sequentially: another
	// window of the same audience until four have passed or two have failed.
	// FAST holds are exempt: cheap by design, and the confirmation and settle
	// passes judge whatever they decided.
	if c.phase != phaseFast {
		t := c.judge(held, brokeReason, brokeDetail)
		switch t.class() {
		case "":
			c.sameTier = true
			return climbContinue, "", ""
		case "held":
			held = true
		default:
			held = false
			brokeReason, brokeDetail = t.mainReason()
		}
		c.cur = nil
	}

	switch c.phase {
	case phaseFast:
		if held {
			c.lo = c.target
			c.loMeasured = false // a FAST hold proves survival, not cost
			if maxViewers > 0 && c.target >= maxViewers {
				// Climbed as high as the operator allowed without the machine
				// ever complaining. That is NOT a capacity verdict, and the
				// report says so.
				return c.finish(StopMaxViewers, fmt.Sprintf("reached the configured ceiling of %d viewers with room to spare", c.target))
			}
			c.growBatch(load)
			c.climbBy(maxViewers)
			return climbContinue, "", ""
		}
		// Something gave out. The ceiling is somewhere in (lo, target].
		// A FAST verdict rests on a few seconds inside a join burst - too cheap
		// to search below. Confirm it at the same size first: a phantom then
		// costs one measured candidate instead of a bisection under it (prod
		// 2026-10-19: a FAST break at 61 sent six full candidates, 41 to 61,
		// through six of the run's ten minutes, and every one of them held).
		c.hi, c.hiReason, c.hiDetail = c.target, brokeReason, brokeDetail
		c.hiJudgedProperly = false
		c.phase = phaseConfirm
		return climbContinue, "", ""

	case phaseBisect:
		if held {
			c.lo = c.target
			c.loMeasured = true // it held under a full measurement window
			c.measuredLo = c.lo
		} else {
			c.hi, c.hiReason, c.hiDetail = c.target, brokeReason, brokeDetail
			c.hiJudgedProperly = true // it broke under a full measurement window
		}
		if !c.converged() {
			c.target = (c.lo + c.hi) / 2
			return climbContinue, "", ""
		}
		// Converged: lo holds, hi does not, and they are close enough that the
		// difference is noise.
		//
		// If hi broke under a full measurement window, the bisection has already
		// proved it - confirming would spend another whole window re-learning it.
		// Only a hi inherited from a FAST hold (a few seconds of samples) is too
		// cheap to trust and needs the confirmation pass.
		if c.hiJudgedProperly {
			return c.finish(c.hiReason, c.hiDetail)
		}
		c.phase = phaseConfirm
		c.target = c.hi
		return climbContinue, "", ""

	case phaseConfirm:
		if !held {
			// Confirmed under a full measurement window: the ceiling lies in
			// (lo, hi], and hi needs no other look. Close enough to lo, lo is
			// the answer; otherwise bisect what is left.
			c.hiReason, c.hiDetail = brokeReason, brokeDetail
			c.hiJudgedProperly = true
			if c.converged() {
				return c.finish(brokeReason, brokeDetail)
			}
			c.phase = phaseBisect
			c.target = (c.lo + c.hi) / 2
			return climbContinue, "", ""
		}
		// hi HELD. The earlier break was a phantom - a cold-start spike read as
		// saturation on a short hold. Adopt hi and resume climbing: capping the
		// machine on a reading that doesn't reproduce would be the worst of both
		// worlds.
		c.lo = c.hi
		c.loMeasured = true // the confirmation pass IS a full window
		c.measuredLo = c.lo
		if maxViewers > 0 && c.lo >= maxViewers {
			return c.finish(StopMaxViewers, fmt.Sprintf("reached the configured ceiling of %d viewers with room to spare", c.lo))
		}
		c.phase = phaseFast
		c.hi, c.hiReason, c.hiDetail = 0, "", ""
		c.target = c.lo
		// Resume cautiously: this region already looked like trouble once.
		c.batch = fastBatch
		c.climbBy(maxViewers)
		return climbContinue, "", ""

	case phaseSettle:
		if held {
			// The answer survived its own measurement: the report's figures for
			// it now come from a real window, and the verdict found earlier
			// stands.
			c.loMeasured = true
			c.measuredLo = c.lo
			return climbDone, c.pendingReason, c.pendingDetail
		}
		// The answer BROKE under the very window that was meant to back it: it
		// was never the answer. Resume the search below it, from the highest
		// count a full window proved: what only FAST holds showed above that is
		// unproven again, what a full window proved is not (prod 2026-10-19:
		// a restart from zero threw away a measured 91 and ran out the clock).
		// It terminates: each settle failure strictly lowers hi, and measuredLo
		// sits below it.
		c.hi, c.hiReason, c.hiDetail = c.target, brokeReason, brokeDetail
		c.hiJudgedProperly = true
		c.lo, c.loMeasured = c.measuredLo, c.measuredLo > 0
		if c.converged() {
			return c.finish(brokeReason, brokeDetail)
		}
		c.phase = phaseBisect
		c.target = (c.lo + c.hi) / 2
		return climbContinue, "", ""
	}
	return climbDone, StopError, "unreachable climb phase"
}

// stepWindows is the most windows a measured candidate is judged on, and
// stepPassShare the share of them it must pass to be held: four of five.
const (
	stepWindows   = 5
	stepPassShare = 0.8
)

// stepTally is one measured candidate's windows.
type stepTally struct {
	viewers, windows, passed int
	// of is how many windows the candidate is judged on (stepWindows unless a
	// test shrinks it).
	of      int
	reasons map[StopReason]int
	details map[StopReason]string
}

func (t *stepTally) failed() int { return t.windows - t.passed }

// class is "held" once enough windows passed, "lost" once too many failed
// for that to remain possible, "" while undecided.
func (t *stepTally) class() string {
	total := t.of
	if total <= 0 {
		total = stepWindows
	}
	need := int(math.Ceil(stepPassShare * float64(total)))
	switch {
	case t.passed >= need:
		return "held"
	case t.failed() > total-need:
		return "lost"
	}
	return ""
}

// mainReason is the signal that broke most of the candidate's failed windows.
func (t *stepTally) mainReason() (StopReason, string) {
	var best StopReason
	n := -1
	for r, c := range t.reasons {
		if c > n || (c == n && r < best) {
			best, n = r, c
		}
	}
	return best, t.details[best]
}

// judge adds the finished window to the candidate being judged, opening its
// tally on the candidate's first window.
func (c *climb) judge(held bool, reason StopReason, detail string) *stepTally {
	t := c.cur
	if t == nil || t.viewers != c.target {
		t = &stepTally{viewers: c.target, of: c.windows, reasons: map[StopReason]int{}, details: map[StopReason]string{}}
		c.cur = t
		c.steps = append(c.steps, t)
	}
	t.windows++
	if held {
		t.passed++
	} else {
		t.reasons[reason]++
		if _, ok := t.details[reason]; !ok {
			t.details[reason] = detail
		}
	}
	return t
}

// stepChurn is how many viewers of an audience are replaced at each new
// window of the same candidate: the oldest leave and newcomers join. A real
// audience is never frozen, and without newcomers the windows after the first
// would never measure how long a new viewer waits to start - the one figure a
// person holding the remote notices first. At least startupMinSamples
// newcomers per window, a tenth of the audience past thirty viewers.
func stepChurn(tier int) int { return max(1, min(tier, max(startupMinSamples, tier/10))) }

// StepTier is one measured candidate, for the report: its windows, how many
// passed, whether it held and what broke it.
type StepTier struct {
	Viewers, Windows, Passed int
	Held                     bool
	Reason                   StopReason
	// Resources is what the machine did over all the candidate's windows (the
	// server's CPU share).
	Resources ResourceStats
}

// stepReport lists every measured candidate in the order judged.
func (c *climb) stepReport() []StepTier {
	out := make([]StepTier, 0, len(c.steps))
	for _, t := range c.steps {
		if t.windows == 0 {
			continue
		}
		reason, _ := t.mainReason()
		held := t.class() == "held"
		if held {
			reason = ""
		}
		out = append(out, StepTier{Viewers: t.viewers, Windows: t.windows, Passed: t.passed, Held: held, Reason: reason})
	}
	return out
}

// answerRange is the range around the answer: the last size held and the
// first size broken above it (the answer alone when nothing broke).
func (c *climb) answerRange() (low, high int) {
	low, high = c.lo, c.lo
	if c.hi > c.lo {
		high = c.hi
	}
	return low, high
}
