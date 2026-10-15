package loadtest

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"sync"
)

// runner_ramp.go holds audience sizing: the ramp state and the spawn / resize /
// shed / drop of viewers as the target audience changes.

// rampState is the ramp's own bookkeeping across every spawn attempt: which
// tier it's at, how many viewers replayed a title at another position, how
// many candidates were skipped as ineligible, and each slot's assigned viewer
// type.
type rampState struct {
	tier              int
	skippedCandidates int
	// roundBySlot is which replay of its title each viewer is on (0 = the first
	// viewer on that title). Counting the replays of the LIVE audience - rather
	// than every replay the run ever spawned - is what makes the figure mean
	// something: the climb rebuilds its audience between tiers, and a running
	// total would keep counting viewers that no longer exist.
	roundBySlot map[int]int
	// replayedByTier snapshots, for each tier the run held, how many of THOSE
	// viewers were replaying a title.
	replayedByTier map[int]int

	// audienceIntact is true while the live audience is exactly ranks 1..tier -
	// which is what makes the scenario's mix (e.g. 80% transcode) actually hold.
	// A viewer the server REFUSED punches a hole in that sequence: the audience
	// is then a subset, missing precisely the ranks the server couldn't serve,
	// and the survivors skew towards the cheap path. Judging the next tier on
	// that would quietly turn a heavy run into a direct-play run and report a
	// capacity nobody asked about, so the audience is rebuilt once before the
	// next judged tier - and only then, which is what keeps the thundering-herd
	// restart out of the common case.
	audienceIntact bool
	// consecutiveSkips counts ineligible candidates since the last viewer that
	// actually started streaming. Once it reaches the number of distinct
	// titles, every title has been tried and none is mkvgo-eligible: keep
	// retrying and the run would burn the whole pool one HTTP request at a
	// time. Any successful viewer resets it.
	consecutiveSkips int
	viewerTypeBySlot map[int]viewerType
	// pathBySlot is the path the server chose for each viewer that started, and
	// pathsByType tallies them per viewer type over the whole run: what the
	// server decided for each kind of client, whatever tier it happened at.
	pathBySlot  map[int]string
	pathsByType map[viewerType]map[string]int
	// mediaBySlot is which TITLE each live viewer holds. The shrink-time reap
	// kills sessions media-wide (RemoveSession), so it must know which media
	// still have a live viewer on them and leave those alone.
	mediaBySlot map[int]string
	// filePathBySlot is the file each live viewer reads, so a tier can say
	// which branch of a pooled mount ITS audience sat on; branchesByTier
	// snapshots that at each tier's start, like replayedByTier.
	filePathBySlot map[int]string
	branchesByTier map[int][]BranchShare
	// stratumBySlot is each live viewer's stratum, given back to the pool when
	// the viewer leaves; strataByTier snapshots the live split at each tier.
	stratumBySlot map[int]string
	strataByTier  map[int][]StratumShare
	pool          *candidatePool

	// rebuilding is true while resizeTo is repopulating a REBUILT audience;
	// herdBySlot marks the viewers spawned that way. Their first fetches all
	// land together - a start no real audience produces - so their TTFF must
	// never feed the startup verdict, nor the report's startup percentiles.
	// A viewer that organically joins an established audience is unmarked:
	// its first fetch is exactly what the startup verdict measures.
	rebuilding bool
	herdBySlot map[int]bool

	// cancelBySlot lets the ramp take viewers AWAY, not just add them - which
	// is what makes the bisection possible at all. Each viewer owns a context
	// derived from the run's, so shedding one leaves the others untouched.
	cancelBySlot map[int]context.CancelFunc
	// nextSlot is a monotonic id: a slot number is never reused, even after
	// its viewer has been shed, so a late event from a dead viewer can never be
	// mistaken for a live one's.
	nextSlot int
}

// shedTo drops viewers until only `target` remain, youngest first. The viewers
// that stay are the oldest ones, which are also the ones furthest past their
// own cold start - so the audience that remains is the most settled available.
func (r *Runner) shedTo(ramp *rampState, target int) {
	for slot := ramp.nextSlot; slot >= 1 && ramp.tier > target; slot-- {
		cancel, live := ramp.cancelBySlot[slot]
		if !live {
			continue
		}
		cancel()
		ramp.releaseStratum(slot)
		delete(ramp.cancelBySlot, slot)
		delete(ramp.herdBySlot, slot)
		delete(ramp.mediaBySlot, slot)
		delete(ramp.filePathBySlot, slot)
		ramp.tier--
		r.liveViewers.Add(-1)
		if ramp.viewerTypeBySlot[slot] == viewerTypeTranscode {
			r.liveTranscodeViewers.Add(-1)
		}
	}
}

// shedOldest drops the n OLDEST live viewers: the grid's churn. The oldest
// leave the way an audience does (they finish), and the newcomers who replace
// them stay long enough for their first fetch - the startup being measured -
// to land. Shedding the youngest instead cut each window's newcomers off
// before their first segment arrived, and startup was never measured.
func (r *Runner) shedOldest(ramp *rampState, n int) {
	for slot := 1; slot <= ramp.nextSlot && n > 0; slot++ {
		if _, live := ramp.cancelBySlot[slot]; !live {
			continue
		}
		r.dropSlot(ramp, slot)
		n--
	}
}

// dropSlot removes one live viewer without marking the audience as holed: a
// planned departure, not a refusal.
func (r *Runner) dropSlot(ramp *rampState, slot int) {
	cancel, live := ramp.cancelBySlot[slot]
	if !live {
		return
	}
	cancel()
	ramp.releaseStratum(slot)
	delete(ramp.cancelBySlot, slot)
	delete(ramp.herdBySlot, slot)
	delete(ramp.mediaBySlot, slot)
	delete(ramp.filePathBySlot, slot)
	ramp.tier--
	r.liveViewers.Add(-1)
	if ramp.viewerTypeBySlot[slot] == viewerTypeTranscode {
		r.liveTranscodeViewers.Add(-1)
	}
}

// releaseStratum gives a leaving viewer's stratum place back to the pool.
func (ramp *rampState) releaseStratum(slot int) {
	if ramp.pool != nil {
		ramp.pool.release(ramp.viewerTypeBySlot[slot], ramp.stratumBySlot[slot])
	}
	delete(ramp.stratumBySlot, slot)
}

// StratumShare is how many viewers of an audience sat in one stratum
// ("/mnt/disk_1|q3"), for the report.
type StratumShare struct {
	Stratum string
	Viewers int
}

// liveStrata is the live audience's split by stratum, largest first.
func (ramp *rampState) liveStrata() []StratumShare {
	counts := map[string]int{}
	for slot := range ramp.cancelBySlot {
		counts[ramp.stratumBySlot[slot]]++
	}
	out := make([]StratumShare, 0, len(counts))
	for k, n := range counts {
		out = append(out, StratumShare{Stratum: k, Viewers: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stratum < out[j].Stratum })
	return out
}

// spawnNext tries to fill the next ramp slot (ramp.tier+1) with the pool's
// next (title, start position) assignment. Reusing a title is normal and
// expected once the library is exhausted - the viewer is simply placed further
// into the file - so the only way this fails is a pool with nothing left at
// all, which is a library that cannot even fill titles x positions viewers.
func (r *Runner) spawnNext(viewerCtx context.Context, cfg Config, pool *candidatePool, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (bool, string) {
	// The viewer's TYPE follows its rank in the live audience (tier+1), not its
	// slot id: the scenario's mix (e.g. 50/50) must hold for the viewers that
	// are actually watching, even after the bisection has shed some.
	vt := scenarioViewerType(cfg.Scenario, ramp.tier+1)

	asg, ok := pool.takeFor(vt)
	if !ok {
		return false, fmt.Sprintf("media pool exhausted (%d titles x %d start positions)", len(pool.candidates), len(startPositions))
	}
	// A transcode-typed viewer replaying a title must fork the server's
	// decision hash, or it would attach to the session an earlier viewer
	// already drives instead of costing the server a new encode. The mkvgo
	// path is stateless, so a distinct start position is enough there.
	forked := asg.round > 0 && vt == viewerTypeTranscode

	ramp.nextSlot++
	slot := ramp.nextSlot
	ramp.tier++
	r.liveViewers.Add(1)
	if vt == viewerTypeTranscode {
		r.liveTranscodeViewers.Add(1)
	}
	ramp.viewerTypeBySlot[slot] = vt
	ramp.roundBySlot[slot] = asg.round
	ramp.mediaBySlot[slot] = asg.media.ID
	if ramp.filePathBySlot != nil {
		ramp.filePathBySlot[slot] = asg.media.Path
	}
	if ramp.stratumBySlot != nil {
		ramp.stratumBySlot[slot] = asg.stratum
	}
	if ramp.rebuilding {
		ramp.herdBySlot[slot] = true
	}

	// Its own context, so the bisection can shed THIS viewer without touching
	// any other.
	vCtx, cancel := context.WithCancel(viewerCtx)
	ramp.cancelBySlot[slot] = cancel

	wg.Add(1)
	// Tagged as the load generator's own work, so the CPU profile can bill this
	// goroutine (and the audio fetch it spawns) to the harness rather than to the
	// server it is pretending to watch.
	go withHarnessLabel(vCtx, func(ctx context.Context) {
		r.runViewer(ctx, cfg, slot, vt, asg, forked, events, wg)
	})
	return true, ""
}

// resizeTo brings the live audience to `target`, spawning or shedding as
// needed. It reports false only when the pool cannot supply another viewer.
func (r *Runner) resizeTo(target int, viewerCtx context.Context, cfg Config, pool *candidatePool, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (bool, string) {
	if target < ramp.tier {
		r.shedTo(ramp, target)
		// The shed viewers' encoder sessions live on server-side and their
		// CPU would be billed to the smaller tier about to be judged. Reap
		// them - but only the media no SURVIVOR holds, because the reap
		// kills media-wide.
		r.reapIdleSessions(pool, ramp)
		return true, ""
	}
	for ramp.tier < target {
		if ok, detail := r.spawnNext(viewerCtx, cfg, pool, events, wg, ramp); !ok {
			return false, detail
		}
	}
	return true, ""
}

// spawnInitialViewers launches the run's very first wave: exactly one
// viewer for every mode except flash, which launches min(MaxViewers,
// available candidates) at once (no ramp at all). It reports whether the
// run has anything to observe; on failure the caller finishes the run
// immediately with StopLibraryLimit.
func (r *Runner) spawnInitialViewers(isFlash bool, viewerCtx context.Context, cfg Config, pool *candidatePool, events chan viewerEvent, wg *sync.WaitGroup, ramp *rampState) (bool, string) {
	// flashTarget is how many viewers the slam launches at once. Unlike the
	// climb, flash cannot discover its own size - it IS the size - so an
	// unbounded MaxViewers (the default) needs a concrete number here rather
	// than "every assignment in the pool", which would be thousands.
	flashTarget := 1
	if isFlash {
		want := cfg.MaxViewers
		if want <= 0 {
			want = defaultFlashViewers
		}
		flashTarget = min(want, pool.capacity())
	}
	for range flashTarget {
		if ok, detail := r.spawnNext(viewerCtx, cfg, pool, events, wg, ramp); !ok {
			if isFlash && ramp.tier > 0 {
				// Some viewers already launched; the flash slam is smaller
				// than intended but still valid - fall through and let it
				// run its observation window rather than discarding it.
				break
			}
			if detail == "" {
				detail = "no media candidates available"
			}
			return false, detail
		}
	}
	return true, ""
}

// recordPath notes the path the server chose for a viewer, once: a seek's
// first fetch is a TTFF too, and must not count the viewer twice.
func (ramp *rampState) recordPath(ev viewerEvent) {
	if ev.servedBy == "" {
		return
	}
	if _, seen := ramp.pathBySlot[ev.slot]; seen {
		return
	}
	ramp.pathBySlot[ev.slot] = ev.servedBy
	if ramp.pathsByType[ev.vt] == nil {
		ramp.pathsByType[ev.vt] = map[string]int{}
	}
	ramp.pathsByType[ev.vt][ev.servedBy]++
}

// pathsReport is pathsByType keyed by type name, for the report.
func (ramp *rampState) pathsReport() map[string]map[string]int {
	out := make(map[string]map[string]int, len(ramp.pathsByType))
	for vt, paths := range ramp.pathsByType {
		out[string(vt)] = maps.Clone(paths)
	}
	return out
}

// replayedInAudience counts how many of the viewers currently watching were
// placed on a title someone else already had (at a different position).
func (ramp *rampState) replayedInAudience() int {
	n := 0
	for slot := range ramp.cancelBySlot {
		if ramp.roundBySlot[slot] > 0 {
			n++
		}
	}
	return n
}

// liveBranchShares is where the viewers watching RIGHT NOW sit on a pooled
// mount, most viewers first: the placement of this tier's audience, not of
// every viewer the run ever spawned. nil when the mount is not a pool.
func (ramp *rampState) liveBranchShares(tally *branchTally) []BranchShare {
	if tally == nil {
		return nil
	}
	counts := map[string]int{}
	for slot := range ramp.cancelBySlot {
		if br := tally.branchFor(ramp.filePathBySlot[slot]); br != "" {
			counts[br]++
		}
	}
	return sharesFromCounts(counts)
}

// dropViewer removes one viewer from the live audience: it is no longer
// watching anything (the server refused it, or it failed outright), so leaving
// it in the count would inflate every tier above.
func (r *Runner) dropViewer(ramp *rampState, slot int) {
	cancel, live := ramp.cancelBySlot[slot]
	if !live {
		return
	}
	cancel()
	ramp.releaseStratum(slot)
	delete(ramp.cancelBySlot, slot)
	delete(ramp.herdBySlot, slot)
	delete(ramp.mediaBySlot, slot)
	delete(ramp.filePathBySlot, slot)
	ramp.tier--
	r.liveViewers.Add(-1)
	if ramp.viewerTypeBySlot[slot] == viewerTypeTranscode {
		r.liveTranscodeViewers.Add(-1)
	}
	// The audience is now a subset with a hole where this viewer was, so the
	// scenario's mix no longer holds. The next judged tier gets a clean rebuild.
	ramp.audienceIntact = false
}

// reapLeftoverSessions asks the server to kill every streaming session the
// run's candidates may still hold (see WithSessionReaper). Called only when
// no simulated viewer is watching: right after the audience was shed to zero
// for a rebuild, and once the run is over. No-op without a wired reaper.
func (r *Runner) reapLeftoverSessions(pool *candidatePool) {
	if r.reapSessions != nil {
		r.reapSessions(pool.candidateIDs())
	}
}

// purgeRunActivity clears the playback activity the run raised, its own
// "loadtest" signal included: called once no simulated viewer is left.
func (r *Runner) purgeRunActivity(pool *candidatePool) {
	if r.forgetPlayback != nil {
		r.forgetPlayback(append(pool.candidateIDs(), loadTestActivityID))
	}
}

// liveMedia is the set of media IDs the live audience currently holds.
func (ramp *rampState) liveMedia() map[string]bool {
	live := make(map[string]bool, len(ramp.mediaBySlot))
	for _, id := range ramp.mediaBySlot {
		live[id] = true
	}
	return live
}

// reapIdleSessions asks the server to kill the sessions of every candidate NO
// live viewer holds - the shrink-time counterpart of reapLeftoverSessions.
// A shed viewer's encoder session outlives it by minutes and would spend CPU
// (and admission budget) over the smaller tier being judged; a title a
// survivor still watches is left alone, because the reap kills media-wide.
// No-op without a wired reaper, and when every candidate is being watched.
// releaseViewerMedia forgets which title a viewer that is no longer watching
// held, then reaps the sessions of every title nobody live is on. It is the
// reap for modes that keep the slot in the audience count - flash judges
// served-of-launched, so dropViewer (which shrinks the tier) is not an option
// there - and it kills media-wide, so a title a survivor shares is spared.
func (r *Runner) releaseViewerMedia(pool *candidatePool, ramp *rampState, slot int) {
	delete(ramp.mediaBySlot, slot)
	r.reapIdleSessions(pool, ramp)
}

func (r *Runner) reapIdleSessions(pool *candidatePool, ramp *rampState) {
	if r.reapSessions == nil {
		return
	}
	live := ramp.liveMedia()
	ids := make([]string, 0, len(pool.candidates))
	for _, m := range pool.candidates {
		if !live[m.ID] {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	r.reapSessions(ids)
}
