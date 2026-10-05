package loadtest

import (
	"math/rand/v2"
	"sort"
	"time"
)

// startPositions are the fractions of a title's usable length at which
// successive viewers of that SAME title are placed. The order is a
// bit-reversal sequence (0, 1/2, 1/4, 3/4, 1/8, ...) rather than a linear
// sweep: each new round of viewers lands as far as possible from every
// position already taken, so two viewers of one title never read neighbouring
// bytes.
//
// Distinct positions are what make a title reusable at all. Evey's mkvgo path
// is stateless - it packages each segment from the source on request, with no
// segment cache (see transcode_ondemand.go) - so two viewers reading DIFFERENT
// offsets of one file do two genuine disk reads and two genuine remuxes. Two
// viewers reading the SAME offset would be served the second time from the OS
// page cache, which measures nothing.
var startPositions = []float64{0, 0.5, 0.25, 0.75, 0.125, 0.625, 0.375, 0.875}

// assignment is what one simulated viewer is given: a title, and where in that
// title to start playing.
type assignment struct {
	media     MediaCandidate
	startFrac float64
	// round is 0 for the first viewer placed on this title, 1 for the second,
	// and so on. A transcode-typed viewer with round > 0 must fork the server's
	// decision hash (audioOffsetMs), or it would land in the session an earlier
	// viewer already owns instead of creating work of its own.
	round int
}

// minUsableRunway is how much of a title must lie ahead of a replayed viewer
// for the replay to be worth anything. Below this, the viewer would reach the
// end of the file and wrap around to bytes an earlier viewer already pulled
// through the page cache - measuring the cache, not the disk. Such a title is
// only ever handed out ONCE, at the start.
const minUsableRunway = 0.05

// candidatePool hands out (title, start position) assignments. Its capacity is
// titles x usable positions, not titles: a library smaller than MaxViewers is
// no longer a reason to stop the ramp, which is the whole point - the load test
// must report what the SERVER ran out of, never what the library ran out of.
type candidatePool struct {
	candidates  []MediaCandidate
	assignments []assignment
	taken       int
	// dead titles are never handed out again. Mkvgo eligibility is a property
	// of the TITLE (how the server decides to serve it), not of the start
	// position: a title skipped at one offset will be skipped at every other,
	// and retrying it - above all when the climb rebuilds its audience - burns
	// candidates and inflates the skip count for nothing.
	dead map[string]bool
	// served counts what was actually handed out, per source class - the run's
	// own record of what its verdict was measured on.
	served map[SourceMixEntry]int
	// branches records where each handed-out title lives on a pooled mount. Nil
	// (a no-op) unless the library sits on one (see pooldiag.go).
	branches *branchTally
}

// newCandidatePool lays out every assignment the run may hand out, breadth
// first: every title once at the start, THEN every (long enough) title again
// half-way in, and so on. Titles are spread before they are stacked, so a run
// only ever reuses a file once it has exhausted the library. The order of the
// titles is the caller's: the run shuffles them (see shuffleCandidates), the
// way an audience picks films, rather than arranging them by disk.
func newCandidatePool(candidates []MediaCandidate, runDuration time.Duration) *candidatePool {
	p := &candidatePool{candidates: candidates}
	for round := range startPositions {
		for _, m := range candidates {
			if round > 0 && usableFraction(m.DurationSec, runDuration) <= minUsableRunway {
				// Too short to replay: it would wrap around and re-read bytes
				// the page cache is already holding.
				continue
			}
			p.assignments = append(p.assignments, assignment{
				media:     m,
				startFrac: startFracFor(m, round, runDuration),
				round:     round,
			})
		}
	}
	return p
}

// capacity is how many viewers this pool can place in total.
func (p *candidatePool) capacity() int { return len(p.assignments) }

// take hands out the next assignment, skipping titles marked ineligible.
func (p *candidatePool) take() (assignment, bool) {
	for p.taken < len(p.assignments) {
		asg := p.assignments[p.taken]
		p.taken++
		if p.dead[asg.media.ID] {
			continue
		}
		p.recordServed(asg.media)
		p.branches.record(asg.media.Path)
		return asg, true
	}
	return assignment{}, false
}

// markIneligible retires a title from the pool for the rest of the run: the
// server has shown it will not serve it the way this scenario measures.
func (p *candidatePool) markIneligible(mediaID string) {
	if p.dead == nil {
		p.dead = map[string]bool{}
	}
	p.dead[mediaID] = true
}

// startFracFor places round N of a title at startPositions[N], scaled down to
// the part of the title a viewer can still play to the end of the run without
// looping back to the start. A viewer that wraps around would re-read bytes an
// earlier viewer already pulled through the page cache, so the runway matters:
// a 2h film during a 15min run is usable up to ~87%, a 25min episode only up
// to ~40%.
func startFracFor(m MediaCandidate, round int, runDuration time.Duration) float64 {
	if round <= 0 {
		return 0
	}
	return startPositions[round%len(startPositions)] * usableFraction(m.DurationSec, runDuration)
}

// usableFraction is how far into a title a viewer may start and still have
// enough runway left to play for the whole remaining run. Zero when the title
// is shorter than the run (such a title can only be played from the start, and
// will loop - the parent's candidate query is what keeps those out of the
// pool).
func usableFraction(durationSec int, runDuration time.Duration) float64 {
	runSec := runDuration.Seconds()
	if durationSec <= 0 || runSec <= 0 {
		return 0
	}
	usable := 1 - runSec/float64(durationSec)
	if usable < 0 {
		return 0
	}
	return usable
}

// recordServed tallies one handout against its source class.
func (p *candidatePool) recordServed(m MediaCandidate) {
	if p.served == nil {
		p.served = map[SourceMixEntry]int{}
	}
	p.served[SourceMixEntry{Codec: m.VideoCodec, Height: m.VideoHeight}]++
}

// candidateIDs lists every media ID the pool can hand out - the reap surface:
// a leftover server session from this run can only belong to one of these.
func (p *candidatePool) candidateIDs() []string {
	ids := make([]string, 0, len(p.candidates))
	for _, m := range p.candidates {
		ids = append(ids, m.ID)
	}
	return ids
}

// sourceMix reports what the run served, heaviest class first so the reader
// sees what dominated the verdict before the long tail.
func (p *candidatePool) sourceMix() []SourceMixEntry {
	out := make([]SourceMixEntry, 0, len(p.served))
	for k, n := range p.served {
		k.Count = n
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Codec != out[j].Codec {
			return out[i].Codec < out[j].Codec
		}
		return out[i].Height < out[j].Height
	})
	return out
}

// shuffleCandidates puts the titles in a random order, in place, and returns
// them. A real audience picks films at random, so each disk of a library ends
// up carrying its share of the titles - no need to know what the storage is
// made of. The seed is logged with the run so a draw can be replayed.
func shuffleCandidates(candidates []MediaCandidate, seed uint64) []MediaCandidate {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	return candidates
}
