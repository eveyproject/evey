package loadtest

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"strings"
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
	// stratum is the class the title was drawn in ("<disk>|q3" for a direct
	// viewer, a cost class for an encoded one). A replay or a replacement stays
	// in it.
	stratum string
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
	// candidates is every title any lane can hand out: the reap surface.
	candidates []MediaCandidate
	// lanes gives each viewer type its own draw, so each type watches the
	// library's own mix whatever the scenario's pattern: a 50/50 pattern over
	// one shared draw cycling four bitrate bands handed the transcodes bands 2
	// and 4 only. def serves the types without a lane of their own.
	lanes map[viewerType]*lane
	def   *lane
	// served counts what was actually handed out, per source class - the run's
	// own record of what its verdict was measured on.
	served map[SourceMixEntry]int
	// branches records where each handed-out title lives on a pooled mount. Nil
	// (a no-op) unless the library sits on one (see pooldiag.go).
	branches *branchTally
}

// lane hands out one viewer type's assignments, stratum by stratum.
//
// Every stratum (disk x bitrate quarter for a direct viewer) keeps its share
// of the eligible titles at EVERY audience size, not only over the whole run:
// each new viewer goes to the stratum furthest behind its share in the LIVE
// audience, and a viewer that leaves (shed by the climb, dropped by the
// server) frees its stratum's place. A single ordered list did not: the climb
// sheds the youngest viewers and refills from wherever the list had got to,
// so after a few resizes the retained audience was whatever the list order
// left, and a title retired mid-run was replaced by the next one in the list,
// from another disk (2026-10-15: 49/44/7 % by disk, 75 to 83 titles retired
// on the way).
type lane struct {
	strata []string
	queues map[string][]assignment
	next   map[string]int
	share  map[string]float64
	live   map[string]int
	// dead titles are never handed out again by this lane: eligibility is a
	// property of the TITLE, not of the start position.
	dead map[string]bool
}

// newLane lays out every assignment the run may hand out, stratum by stratum
// and breadth first within each: every title of the stratum once at the start,
// THEN every long enough title again further in, and so on - a replay stays in
// its title's stratum. classes[i] is candidates[i]'s stratum; nil puts every
// title in one. A stratum's share is its count of titles.
func newLane(candidates []MediaCandidate, classes []string, runDuration time.Duration) *lane {
	l := &lane{queues: map[string][]assignment{}, next: map[string]int{}, share: map[string]float64{}, live: map[string]int{}}
	byStratum := map[string][]MediaCandidate{}
	for i, m := range candidates {
		k := ""
		if i < len(classes) {
			k = classes[i]
		}
		if _, ok := byStratum[k]; !ok {
			l.strata = append(l.strata, k)
		}
		byStratum[k] = append(byStratum[k], m)
	}
	sort.Strings(l.strata)
	for _, k := range l.strata {
		titles := byStratum[k]
		l.share[k] = float64(len(titles)) / float64(max(len(candidates), 1))
		for round := range startPositions {
			for _, m := range titles {
				if round > 0 && usableFraction(m.DurationSec, runDuration) <= minUsableRunway {
					// Too short to replay: it would wrap around and re-read bytes
					// the page cache is already holding.
					continue
				}
				l.queues[k] = append(l.queues[k], assignment{media: m, startFrac: startFracFor(m, round, runDuration), round: round, stratum: k})
			}
		}
	}
	return l
}

// size is how many assignments the lane holds in total.
func (l *lane) size() int {
	n := 0
	for _, q := range l.queues {
		n += len(q)
	}
	return n
}

// pick chooses the stratum the next viewer goes to: the one furthest behind
// its share of the live audience, among those with an assignment left. Ties go
// to the first in name order, so a seed replays its picks.
func (l *lane) pick() (string, bool) {
	total := 0
	for _, n := range l.live {
		total += n
	}
	best, bestDeficit, found := "", 0.0, false
	for _, k := range l.strata {
		if !l.hasNext(k) {
			continue
		}
		deficit := l.share[k]*float64(total+1) - float64(l.live[k])
		if !found || deficit > bestDeficit {
			best, bestDeficit, found = k, deficit, true
		}
	}
	return best, found
}

// hasNext reports whether stratum k still has an assignment that is not dead,
// skipping past dead ones.
func (l *lane) hasNext(k string) bool {
	q := l.queues[k]
	for l.next[k] < len(q) {
		if !l.dead[q[l.next[k]].media.ID] {
			return true
		}
		l.next[k]++
	}
	return false
}

// release frees a live viewer's place in its stratum.
func (l *lane) release(stratum string) {
	if l != nil && l.live[stratum] > 0 {
		l.live[stratum]--
	}
}

// newCandidatePool is a pool whose every viewer type shares one draw, in one
// stratum.
func newCandidatePool(candidates []MediaCandidate, runDuration time.Duration) *candidatePool {
	return &candidatePool{candidates: candidates, def: newLane(candidates, nil, runDuration)}
}

// drawnLane is one viewer type's drawn titles and their strata.
type drawnLane struct {
	titles  []MediaCandidate
	classes []string
}

// newLanedPool gives each viewer type in byType its own draw; candidates is
// their union.
func newLanedPool(candidates []MediaCandidate, byType map[viewerType]drawnLane, runDuration time.Duration) *candidatePool {
	p := &candidatePool{candidates: candidates, lanes: make(map[viewerType]*lane, len(byType))}
	for vt, d := range byType {
		p.lanes[vt] = newLane(d.titles, d.classes, runDuration)
	}
	return p
}

func (p *candidatePool) laneFor(vt viewerType) *lane {
	if l, ok := p.lanes[vt]; ok {
		return l
	}
	// A client viewer negotiates like a natural one: the server, not the
	// harness, decides whether it is encoded, so it draws by bitrate.
	if l, ok := p.lanes[viewerTypeNatural]; ok && strings.HasPrefix(string(vt), clientViewerPrefix) {
		return l
	}
	return p.def
}

// capacity is how many viewers this pool can place in total: its smallest
// lane's, since any type may be the one that runs out.
func (p *candidatePool) capacity() int {
	n := -1
	for _, l := range append(slices.Collect(maps.Values(p.lanes)), p.def) {
		if l != nil && (n < 0 || l.size() < n) {
			n = l.size()
		}
	}
	return max(n, 0)
}

// take hands out the next assignment of the shared draw.
func (p *candidatePool) take() (assignment, bool) { return p.takeFor("") }

// takeFor hands out vt's next assignment, from the stratum furthest behind its
// share of the live audience.
func (p *candidatePool) takeFor(vt viewerType) (assignment, bool) {
	l := p.laneFor(vt)
	if l == nil {
		return assignment{}, false
	}
	k, ok := l.pick()
	if !ok {
		return assignment{}, false
	}
	asg := l.queues[k][l.next[k]]
	l.next[k]++
	l.live[k]++
	p.recordServed(asg.media)
	p.branches.record(asg.media.Path)
	return asg, true
}

// release gives back a viewer's place in its stratum (shed, dropped, skipped).
func (p *candidatePool) release(vt viewerType, stratum string) {
	if p == nil {
		return
	}
	p.laneFor(vt).release(stratum)
}

// liveStrata is how many live viewers of vt's lane sit in each stratum.
func (p *candidatePool) liveStrata(vt viewerType) map[string]int {
	l := p.laneFor(vt)
	if l == nil {
		return nil
	}
	return maps.Clone(l.live)
}

// markIneligible retires a title from the shared draw.
func (p *candidatePool) markIneligible(mediaID string) { p.markIneligibleFor("", mediaID) }

// markIneligibleFor retires a title from vt's lane for the rest of the run:
// the server has shown it will not serve it the way that type measures. The
// other types keep it - a title mkvgo cannot package is still a fine encode.
func (p *candidatePool) markIneligibleFor(vt viewerType, mediaID string) {
	l := p.laneFor(vt)
	if l == nil {
		return
	}
	if l.dead == nil {
		l.dead = map[string]bool{}
	}
	l.dead[mediaID] = true
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
	p.served[SourceMixEntry{Codec: m.VideoCodec, Height: m.VideoHeight, Range: m.VideoRange}]++
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

// bitrateBands buckets the titles by what they weigh to stream: four bands cut
// at the quartiles of their own bitrates, so each band holds a quarter of the
// library whatever its formats are (a 4K web release streams lighter than a
// 1080p remux, so the picture height does not tell). A title of unknown
// bitrate is a band of its own. It returns each title's band and the band
// floors in bit/s, for the log.
func bitrateBands(candidates []MediaCandidate) (band []string, floors []int64) {
	var known []int64
	for _, c := range candidates {
		if c.BitrateBps > 0 {
			known = append(known, c.BitrateBps)
		}
	}
	slices.Sort(known)
	if len(known) > 0 {
		for q := range 4 {
			floors = append(floors, known[q*len(known)/4])
		}
	}
	band = make([]string, len(candidates))
	for i, c := range candidates {
		if c.BitrateBps <= 0 {
			band[i] = "unknown"
			continue
		}
		b := 0
		for q := 1; q < len(floors); q++ {
			if c.BitrateBps >= floors[q] {
				b = q
			}
		}
		band[i] = fmt.Sprintf("q%d", b+1)
	}
	return band, floors
}

// representativeDraw orders the titles so that ANY number of viewers watches
// the library's own mix of bitrates, chosen at random within each band.
// A plain shuffle let the first fifty viewers of one run draw mostly 4K
// (11.7 Mb/s each) and those of the next mostly light files (4.8 Mb/s): the
// same server then "held" 38 or 136 viewers; grouping by picture height
// still left 111 against 176 (a 4K title streams at 15 to 60 Mb/s). The bands
// are the drawn candidates' own bitrate quartiles, themselves a random slice
// of the library. Each slot goes to the band furthest behind its share. It
// returns the order, each band's count and the band floors in bit/s.
func representativeDraw(candidates []MediaCandidate, seed uint64) ([]MediaCandidate, map[string]int, []int64) {
	out, counts, floors, _ := representativeDrawOnDisks(candidates, nil, seed)
	return out, counts, floors
}

// representativeDrawOnDisks is representativeDraw that also keeps each disk to
// its share: disks[i] names the disk candidates[i] is read from ("" unknown),
// and every prefix of viewers holds each (disk, bitrate band) pair in its
// share of the library. A plain bitrate draw held the heavy files' share
// overall but could stack them on one disk of a pool and saturate it while
// the others idled (measured 2026-10-12: sda at 97 %, the others under 12 %).
// A disk that holds most of the heavy files still carries most of them: the
// draw follows the library, it cannot spread what the library does not.
func representativeDrawOnDisks(candidates []MediaCandidate, disks []string, seed uint64) ([]MediaCandidate, map[string]int, []int64, []string) {
	bands, floors := bitrateBands(candidates)
	counts := make(map[string]int, 5)
	classes := make([]string, len(candidates))
	for i, b := range bands {
		counts[b]++
		classes[i] = b
		if i < len(disks) && disks[i] != "" {
			classes[i] = disks[i] + "|" + b
		}
	}
	out, _, outClasses := stratifiedDrawClasses(candidates, classes, seed)
	return out, counts, floors, outClasses
}

// diskKeys names the disk each candidate is read from: its branch on a
// mergerfs pool (branchOf), else the device it sits on; "" when unknown.
func diskKeys(candidates []MediaCandidate, branchOf func(string) string) []string {
	keys := make([]string, len(candidates))
	for i, c := range candidates {
		if branchOf != nil {
			if br := branchOf(c.Path); br != "" {
				keys[i] = br
				continue
			}
		}
		if dev, ok := diskOf(c.Path); ok {
			keys[i] = strconv.FormatUint(dev, 10)
		}
	}
	return keys
}

// costDraw is representativeDraw for viewers the server ENCODES: what such a
// viewer costs is the source's codec, height and dynamic range (a 4K HDR10
// title needs a tone map, a 1080p H.264 one barely a decode), not its bitrate.
// Each class keeps its share of the library, drawn at random within it.
func costDraw(candidates []MediaCandidate, seed uint64) ([]MediaCandidate, map[string]int, []string) {
	classes := make([]string, len(candidates))
	for i, c := range candidates {
		classes[i] = costClass(c)
	}
	return stratifiedDrawClasses(candidates, classes, seed)
}

// costClass names what a title costs to encode, e.g. "hevc 2160p hdr10".
func costClass(c MediaCandidate) string {
	if c.VideoCodec == "" || c.VideoHeight <= 0 {
		return "unknown"
	}
	rng := c.VideoRange
	if rng == "" {
		rng = RangeSDR
	}
	return c.VideoCodec + " " + heightClass(c.VideoHeight) + " " + rng
}

// heightClass buckets a picture height into the rungs an encode is priced at.
func heightClass(h int) string {
	switch {
	case h <= 576:
		return "sd"
	case h <= 720:
		return "720p"
	case h <= 1080:
		return "1080p"
	case h <= 1440:
		return "1440p"
	default:
		return "2160p"
	}
}

// stratifiedDraw orders the titles so that every prefix holds each class in
// its share of the whole, titles shuffled at random within their class. Each
// slot goes to the class furthest behind its share. It returns the order and
// each class's count.
func stratifiedDraw(candidates []MediaCandidate, classOf []string, seed uint64) ([]MediaCandidate, map[string]int) {
	out, counts, _ := stratifiedDrawClasses(candidates, classOf, seed)
	return out, counts
}

// stratifiedDrawClasses is stratifiedDraw that also returns each drawn title's
// class, aligned with the order.
func stratifiedDrawClasses(candidates []MediaCandidate, classOf []string, seed uint64) ([]MediaCandidate, map[string]int, []string) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) //nolint:gosec // a replayable draw of titles, not a secret
	byClass := map[string][]MediaCandidate{}
	for i, c := range candidates {
		byClass[classOf[i]] = append(byClass[classOf[i]], c)
	}
	classes := make([]string, 0, len(byClass))
	for k := range byClass {
		classes = append(classes, k)
	}
	// Sorted before shuffling: the generator must be consumed in a fixed order
	// for a seed to replay its draw (map order differs from run to run).
	sort.Strings(classes)
	counts := make(map[string]int, len(byClass))
	for _, k := range classes {
		list := byClass[k]
		rng.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
		counts[k] = len(list)
	}
	total := len(candidates)
	taken := make(map[string]int, len(classes))
	out := make([]MediaCandidate, 0, total)
	outClasses := make([]string, 0, total)
	for slot := 1; slot <= total; slot++ {
		best, bestDeficit := "", -1.0
		for _, k := range classes {
			if taken[k] >= counts[k] {
				continue
			}
			deficit := float64(counts[k])*float64(slot)/float64(total) - float64(taken[k])
			if deficit > bestDeficit {
				best, bestDeficit = k, deficit
			}
		}
		out = append(out, byClass[best][taken[best]])
		outClasses = append(outClasses, best)
		taken[best]++
	}
	return out, counts, outClasses
}
