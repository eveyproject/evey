package library

import "sort"

// Boundary fusion: no single signal locates an intro edge reliably (audio run
// length bleeds, trickplay tiles are coarse and fail on animated titles,
// silence/black is often absent, subtitles aren't always present). So instead of
// trusting one, we gather every available signal as a weighted VOTE for the same
// edge and fuse them: where independent signals AGREE we are confident
// (deterministic); where only one is available we fall back to it at low
// confidence. This is what makes detection robust on series with no subtitles -
// the other witnesses carry the vote.

// boundaryVote is one signal's estimate of an edge (start or end), in ms, with a
// weight reflecting how trustworthy that signal is, and a source label.
type boundaryVote struct {
	ms     int64
	weight float64
	source string
}

// fusedBoundary is the consensus edge plus the agreement evidence behind it.
type fusedBoundary struct {
	ms         int64
	confidence float64  // winning cluster weight / total weight, in [0,1]
	agree      int      // number of votes inside the winning cluster
	sources    []string // their source labels
}

// fuseBoundary clusters the votes (any two within tolMs are the same edge),
// picks the cluster carrying the most weight, and returns its weight-weighted
// mean as the consensus edge. confidence is the fraction of total weight that
// agreed - high when several independent signals point to the same place, low
// when a lone signal stands unsupported. A winning cluster of ONE vote is
// capped at 0.5 regardless of its weight share: corroboration is what the
// number measures, and a lone witness has none (a lone vote is 100% of its own
// weight, which said nothing). ok is false when there are no votes.
func fuseBoundary(votes []boundaryVote, tolMs int64) (fusedBoundary, bool) {
	valid := make([]boundaryVote, 0, len(votes))
	var totalWeight float64
	for _, v := range votes {
		if v.weight > 0 {
			valid = append(valid, v)
			totalWeight += v.weight
		}
	}
	if len(valid) == 0 || totalWeight <= 0 {
		return fusedBoundary{}, false
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].ms < valid[j].ms })

	// Greedy left-to-right clustering: a vote joins the current cluster while it
	// sits within tolMs of the cluster's first member. Keep the heaviest cluster.
	best := fusedBoundary{}
	var bestWeight float64
	i := 0
	for i < len(valid) {
		j := i
		var sumW, sumWMs float64
		var srcs []string
		anchor := valid[i].ms
		for j < len(valid) && valid[j].ms-anchor <= tolMs {
			sumW += valid[j].weight
			sumWMs += valid[j].weight * float64(valid[j].ms)
			srcs = append(srcs, valid[j].source)
			j++
		}
		if sumW > bestWeight {
			bestWeight = sumW
			best = fusedBoundary{
				ms:         int64(sumWMs / sumW),
				confidence: sumW / totalWeight,
				agree:      len(srcs),
				sources:    srcs,
			}
		}
		i = j
	}
	if best.agree == 1 && best.confidence > 0.5 {
		best.confidence = 0.5
	}
	return best, true
}

// snapToNearest returns the value in points closest to target, when one lies
// within tolMs (used to snap a coarse consensus edge onto a frame-accurate
// boundary such as a scene cut or a silence edge). Returns (target, false) when
// nothing is close enough.
func snapToNearest(target int64, points []int64, tolMs int64) (int64, bool) {
	bestD := tolMs + 1
	best := target
	for _, p := range points {
		d := p - target
		if d < 0 {
			d = -d
		}
		if d <= tolMs && d < bestD {
			bestD = d
			best = p
		}
	}
	return best, best != target
}
