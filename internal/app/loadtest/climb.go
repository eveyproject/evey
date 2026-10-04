package loadtest

import (
	"fmt"
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
//   BISECT  - once a batch breaks, the answer lies between the last batch that
//             held (lo) and the one that broke (hi). Halve the interval, hold
//             each candidate long enough to judge it properly, and repeat.
//             log2(step) rounds, not step rounds.
//   CONFIRM - the FAST phase judges on a short hold, so it can cry wolf (an
//             ffmpeg cold start looks like a CPU spike). Before accepting hi as
//             the ceiling, re-test it in steady state. If hi actually holds, the
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

	// retried records that the current tier already broke once under a full
	// measurement window and is being measured a second time. A single window
	// is a sample, not a verdict: its p95 rests on a few hundred segments, and
	// a tier sitting near its budget flips on noise - or on the backlog of the
	// audience that was just shed, still queued on the disk. Only a break that
	// REPRODUCES condemns a tier (see next).
	retried bool

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
	// Take the tier's verdict, then clear it so the next tier starts clean.
	brokeReason, brokeDetail := c.broke, c.detail
	held := brokeReason == ""
	c.broke, c.detail = "", ""

	// A break under a full window must reproduce before it counts. The tier is
	// measured once more as it stands - same audience, a fresh window - so the
	// backlog that a shed audience leaves on the disk has drained and one noisy
	// sample cannot send the search down. FAST holds are exempt: they are cheap
	// by design and already get a confirmation pass.
	if !held && !c.retried && (c.phase == phaseBisect || c.phase == phaseSettle) {
		c.retried = true
		return climbContinue, "", ""
	}
	c.retried = false

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
		c.hi, c.hiReason, c.hiDetail = c.target, brokeReason, brokeDetail
		c.hiJudgedProperly = false // a FAST hold is too short to be the last word
		if c.converged() {
			// Nothing to bisect (a 1-wide gap, or the very first viewer broke),
			// but a FAST verdict rests on a few seconds of samples - too cheap
			// to accept. Confirm it properly.
			c.phase = phaseConfirm
			c.target = c.hi
			return climbContinue, "", ""
		}
		c.phase = phaseBisect
		c.target = (c.lo + c.hi) / 2
		return climbContinue, "", ""

	case phaseBisect:
		if held {
			c.lo = c.target
			c.loMeasured = true // it held under a full measurement window
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
			// Confirmed under a full measurement window: hi really is one viewer
			// too many, so lo is the ceiling.
			return c.finish(brokeReason, brokeDetail)
		}
		// hi HELD. The earlier break was a phantom - a cold-start spike read as
		// saturation on a short hold. Adopt hi and resume climbing: capping the
		// machine on a reading that doesn't reproduce would be the worst of both
		// worlds.
		c.lo = c.hi
		c.loMeasured = true // the confirmation pass IS a full window
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
			return climbDone, c.pendingReason, c.pendingDetail
		}
		// The answer BROKE under the very window that was meant to back it: it
		// was never the answer. Resume the search below it - everything under
		// this tier is unproven again, so the bisection restarts from zero. This
		// is rare (the tier held a FAST hold moments ago) and it terminates:
		// each settle failure strictly lowers hi.
		c.hi, c.hiReason, c.hiDetail = c.target, brokeReason, brokeDetail
		c.hiJudgedProperly = true
		c.lo, c.loMeasured = 0, false
		if c.converged() {
			return c.finish(brokeReason, brokeDetail)
		}
		c.phase = phaseBisect
		c.target = (c.lo + c.hi) / 2
		return climbContinue, "", ""
	}
	return climbDone, StopError, "unreachable climb phase"
}
