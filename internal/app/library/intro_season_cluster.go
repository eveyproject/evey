package library

import (
	"context"
	"log/slog"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

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
		// 12/32 bits, NOT the 6/32 some detectors use: tested 6/32 on a real
		// library and it broke cluster formation (a long-running series lost its
		// intro entirely) because cross-episode fingerprints from mixed sources
		// are noisy - dubs / repacks differ enough that the
		// theme exceeds 6 bits/frame across episodes. 12/32 is calibrated for that
		// noise; tightening it didn't sharpen the fused end anyway (the fusion's
		// scene-cut/silence votes dominate the boundary).
		MaxFrameBits: 12,
		MinRunFrames: 110, // ~14.5s @ 7.56fps - longer than discovery's 80 to reject short recaps
		// No 0:00 guard for the cluster: in ripped/remuxed media the intro very
		// often starts at the very beginning (no broadcast bumper to skip), so a
		// 25s floor (used by the per-episode discovery fallback) would miss those
		// entirely. The MinRunFrames length filter plus picking the longest shared
		// run are what keep a short recurring promo from posing as the intro.
		MinStartFrames: 0,
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

// seasonClusterEpisodeCap bounds how many episodes the clustering fingerprints,
// keeping the anchor/score passes affordable on very long single seasons.
const seasonClusterEpisodeCap = 40

// clusterSeasonIntro fingerprints every episode of the season once (cached) and
// clusters the recurring intro shared across the season. It returns a per-
// episode intro hint (a raw, un-refined window) keyed by media item id, or nil
// when the feature is off, audio is unavailable, or no signature reaches KMin
// carriers. It forks no ffmpeg beyond the cached Chromaprint compute - the
// per-carrier boundary refinement happens later, in the per-episode pipeline.
func (s *IntroDetectionService) clusterSeasonIntro(ctx context.Context, parentID string) map[string]domainlibrary.MediaSegment {
	cfg := loadIntroConfig(ctx, s.configRepo)
	if !cfg.SeasonClusterEnabled || s.chromaprinter == nil {
		return nil
	}
	episodeType := string(domainlibrary.Episode)
	var candidates []*domainlibrary.MediaItem
	for offset := 0; len(candidates) < seasonClusterEpisodeCap; offset += 200 {
		page, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			ParentID: &parentID, Type: &episodeType, Limit: 200, Offset: offset,
		})
		if err != nil || len(page) == 0 {
			break
		}
		for _, ep := range page {
			if ep.File == nil || ep.File.Path == "" {
				continue
			}
			candidates = append(candidates, ep)
			if len(candidates) >= seasonClusterEpisodeCap {
				break
			}
		}
		if len(page) < 200 {
			break
		}
	}

	// One batched cache read for the whole season instead of one Get per
	// episode; only the misses (or stale entries) go through the per-episode
	// compute+upsert path.
	prefetched := s.prefetchAudioFingerprints(ctx, candidates, discoverWindowSec)
	var eps []episodeFP
	for _, ep := range candidates {
		fp, ok := prefetched[string(ep.ID)]
		if !ok {
			var ferr error
			fp, ferr = s.extractEpisodeAudioCached(ctx, ep, discoverWindowSec)
			if ferr != nil || len(fp) == 0 {
				continue
			}
		}
		eps = append(eps, episodeFP{id: string(ep.ID), fp: fp})
	}
	cluster, ok := buildSeasonCluster(eps, defaultClusterParams(len(eps)))
	if !ok {
		return nil
	}
	sigFrames := len(cluster.sig)
	out := make(map[string]domainlibrary.MediaSegment, len(cluster.carriers))
	dropped := 0
	for _, c := range cluster.carriers {
		startMs := framesToMs(c.offsetFrames)
		// Same cap as the pHash pass: a recurring segment sitting deeper than
		// MaxIntroStartMs is an in-episode element (chapter card, flashback),
		// not a title sequence. The cluster had no cap before the window grew
		// past 8 minutes.
		if cfg.MaxIntroStartMs > 0 && startMs > cfg.MaxIntroStartMs {
			dropped++
			continue
		}
		out[c.id] = domainlibrary.MediaSegment{
			Type:       "intro",
			StartMs:    startMs,
			EndMs:      framesToMs(c.offsetFrames + sigFrames),
			Source:     "audio_cluster",
			Confidence: 0.9,
		}
	}
	slog.InfoContext(ctx, "intro_detection: season cluster",
		slog.String("parent_id", parentID),
		slog.Int("episodes", len(eps)),
		slog.Int("carriers", len(cluster.carriers)),
		slog.Int("dropped_past_start_cap", dropped),
		slog.Int("sig_frames", sigFrames))
	return out
}
