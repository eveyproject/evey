package library

// Season-level intro clustering (v2).
//
// The per-episode discovery in intro_audio_discovery.go anchors on a single
// target and confirms a shared run against a few siblings. This file
// generalises that to the WHOLE season: every episode contributes its cached
// Chromaprint fingerprint, we find the recurring intro SIGNATURE shared by the
// most episodes (each at its own free offset), and return a per-episode window.
// Like the discovery helpers, everything here is pure ([]uint32 in, results
// out) and unit-testable without ffmpeg/fpcalc.

// episodeFP is one episode's Chromaprint fingerprint over [0, windowSec].
type episodeFP struct {
	id string
	fp []uint32
}

// clusterParams tunes the season clustering. Defaults mirror discoverParams
// where they overlap (same seeded experiment), with a longer MinRunFrames so a
// short "previously on" recap shared across episodes can't pose as the intro.
type clusterParams struct {
	MaxFrameBits   int     // per-frame Hamming tolerance (of 32) for a frame to "match"
	MinRunFrames   int     // shortest accepted shared run (the intro signature length)
	MinStartFrames int     // ignore signatures that start before this in an anchor (anti network-bumper at 0:00)
	SimThreshold   float64 // sliding similarity required to count an episode as a carrier
	KMin           int     // minimum carriers required to accept the season cluster
	MaxAnchors     int     // bound the anchor pass so it stays sub-O(N^2) on long seasons
}

// defaultClusterParams derives the thresholds for a season of n episodes. KMin
// scales with the season (ceil(n/2), floor 3) so a single recurring false
// positive in a minority of episodes can't win the cluster.
func defaultClusterParams(n int) clusterParams {
	kMin := max((n+1)/2, 3)
	return clusterParams{
		MaxFrameBits:   12,
		MinRunFrames:   110, // ~14.5s @ 7.56fps — longer than discovery's 80 to reject short recaps
		MinStartFrames: msToFrames(25000),
		SimThreshold:   0.65,
		KMin:           kMin,
		MaxAnchors:     5,
	}
}

// clusterCarrier is an episode that carries the season intro signature, with
// the offset (in fingerprint frames) where the signature sits in that episode.
type clusterCarrier struct {
	id           string
	offsetFrames int
	sim          float64
}

// seasonCluster is the winning intro signature plus the episodes carrying it.
type seasonCluster struct {
	sig      []uint32
	carriers []clusterCarrier
}

// chooseAnchorIndices spreads up to maxAnchors anchor positions evenly across
// [0, n), always including the first and last. Spreading (rather than taking
// the first few) catches intros that evolve mid-season and avoids anchoring
// only on early episodes. Returned indices are unique and ascending.
func chooseAnchorIndices(n, maxAnchors int) []int {
	if n <= 0 || maxAnchors <= 0 {
		return nil
	}
	if n <= maxAnchors || maxAnchors == 1 {
		idx := make([]int, 0, n)
		limit := n
		if maxAnchors == 1 {
			limit = 1
		}
		for i := range limit {
			idx = append(idx, i)
		}
		return idx
	}
	seen := make(map[int]bool, maxAnchors)
	idx := make([]int, 0, maxAnchors)
	for k := range maxAnchors {
		pos := k * (n - 1) / (maxAnchors - 1) // evenly spaced, inclusive of 0 and n-1
		if !seen[pos] {
			seen[pos] = true
			idx = append(idx, pos)
		}
	}
	return idx
}

// anchorProbe returns the longest run the anchor shares with any other episode
// (its candidate intro signature in the anchor's frame space), or nil when no
// shared run reaches MinRunFrames.
func anchorProbe(anchor episodeFP, eps []episodeFP, p clusterParams) []uint32 {
	bestStart, bestEnd, ok := 0, 0, false
	for i := range eps {
		if eps[i].id == anchor.id {
			continue
		}
		s, e, f := longestSharedRun(anchor.fp, eps[i].fp, p.MaxFrameBits, p.MinRunFrames, p.MinStartFrames)
		if f && (e-s) > (bestEnd-bestStart) {
			bestStart, bestEnd, ok = s, e, true
		}
	}
	if !ok {
		return nil
	}
	return anchor.fp[bestStart:bestEnd]
}

// scoreProbe finds every episode that carries the signature `probe` (sliding
// similarity >= SimThreshold) and records its per-episode offset.
func scoreProbe(probe []uint32, eps []episodeFP, p clusterParams) []clusterCarrier {
	var carriers []clusterCarrier
	for i := range eps {
		if len(eps[i].fp) < len(probe) {
			continue
		}
		off, sim := slidingChromaprint(probe, eps[i].fp)
		if sim >= p.SimThreshold && off >= 0 {
			carriers = append(carriers, clusterCarrier{id: eps[i].id, offsetFrames: off, sim: sim})
		}
	}
	return carriers
}

// buildSeasonCluster finds the intro signature shared by the most episodes and
// returns it with each carrier's offset. found is false when no signature
// reaches KMin carriers (mini-series, no recurring intro, all-unique audio).
func buildSeasonCluster(eps []episodeFP, p clusterParams) (seasonCluster, bool) {
	if len(eps) < p.KMin {
		return seasonCluster{}, false
	}
	// 1) Candidate signatures from spread anchors, de-duplicated across anchors.
	var probes [][]uint32
	for _, ai := range chooseAnchorIndices(len(eps), p.MaxAnchors) {
		probe := anchorProbe(eps[ai], eps, p)
		if probe == nil {
			continue
		}
		dup := false
		for _, kept := range probes {
			if _, sim := slidingChromaprint(probe, kept); sim >= 0.9 {
				dup = true
				break
			}
		}
		if !dup {
			probes = append(probes, probe)
		}
	}
	// 2) Score every candidate across ALL episodes; keep the one with the most
	// carriers (tie-break: the longer signature is the more specific match).
	best := seasonCluster{}
	bestCount := 0
	for _, probe := range probes {
		carriers := scoreProbe(probe, eps, p)
		if len(carriers) > bestCount || (len(carriers) == bestCount && len(probe) > len(best.sig)) {
			best = seasonCluster{sig: probe, carriers: carriers}
			bestCount = len(carriers)
		}
	}
	if len(best.carriers) < p.KMin {
		return seasonCluster{}, false
	}
	return best, true
}
