package library

import (
	"context"
	"log/slog"
	"slices"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// Season-level intro DURATION prior.
//
// Within a season the intro length is nearly constant: the strongest structural
// invariant we have. A detected intro much LONGER than the season's median has
// almost always absorbed a neighbour segment (the "previously on" recap fused
// into the intro window, or theme music bleeding into the first scene). The
// reconciliation pass shrinks those outliers back to the season's median
// duration. It never extends a short intro: a shorter-than-usual detection is
// as likely a cold-open variant as an error, and extending would create
// overshoot where none existed.
//
// The two over-long classes carry their excess at OPPOSITE ends: an absorbed
// recap sits in FRONT of the true title sequence, music bleed sits BEHIND it.
// Shrinking always toward start+median would, on the recap class, keep the
// recap and truncate the actual intro - worse than no correction. The season
// START prior disambiguates: a start far EARLIER than the season's median
// start says the excess is in front, so the END is held and the start
// advances to end-median. On a season of variable cold opens the start MAD
// blows the band open and the start prior disables itself - the shrink then
// stays start-anchored, never a speculative start move.
//
// A season is not always ONE distribution: a mid-season theme change (cour
// split, shortened late-season opening) makes it bimodal. A single median
// then lands on the majority mode and - the lower-median MAD collapsing to
// zero on clean modes - every episode of the LONGER mode reads as an
// over-long outlier and gets truncated to the other mode's duration. The
// reconciliation therefore first looks for one changepoint in episode order
// (both sides significantly separated, each big enough to carry its own
// prior) and runs the whole pass per era.

// priorMinSamples is the fewest season intros required before the duration
// distribution is trusted at all - below this, a single bad detection would
// dominate the median.
const priorMinSamples = 4

// priorMinExcessMs is the floor on how far beyond the robust spread a duration
// must sit before it is treated as an outlier. Keeps the prior away from the
// ±2-4s jitter that is inherent to the fingerprint/tile resolution.
const priorMinExcessMs = 12_000

// priorSigmaFactor scales the MAD-derived robust sigma (1.4826*MAD) into the
// rejection band. 4 sigma keeps legitimate per-episode variation (a slightly
// extended title sequence) inside the band.
const priorSigmaFactor = 4.0

// priorSnapMinDeltaMs is the smallest duration deviation the gentle
// median-snap corrects. Below it we are inside the detector's intrinsic
// ±2-4s jitter and a correction would be noise chasing noise.
const priorSnapMinDeltaMs = 3_000

// priorSnapSupportConf splits corroborated boundaries from lone-witness ones.
// A fused end where independent signals agreed (subtitle + audio + silence)
// stores ≥0.9; a lone-audio end stores 0.8 (0.6 + 0.4*0.5 with the lone-vote
// cap). Only segments BELOW this line get snapped, and only a median
// supported by segments AT OR ABOVE it is trusted as the snap target - that
// is how the subtitle-exact ends of sibling episodes reach the episodes that
// have no subtitles (the music-bleed class the audio signals can't fix:
// four independent per-second features were probed and all were flat at the
// true boundary).
const priorSnapSupportConf = 0.9

// introDurationStats is a robust median/MAD distribution over one intro axis:
// durations, and since the recap-in-front fix, start offsets too.
type introDurationStats struct {
	medianMs int64
	madMs    int64
	n        int
}

// seasonIntroDurationStats computes median and MAD (median absolute deviation)
// of the given values. ok is false below priorMinSamples.
func seasonIntroDurationStats(durationsMs []int64) (introDurationStats, bool) {
	if len(durationsMs) < priorMinSamples {
		return introDurationStats{}, false
	}
	median := int64Median(durationsMs)
	devs := make([]int64, len(durationsMs))
	for i, d := range durationsMs {
		dev := d - median
		if dev < 0 {
			dev = -dev
		}
		devs[i] = dev
	}
	return introDurationStats{medianMs: median, madMs: int64Median(devs), n: len(durationsMs)}, true
}

// priorBand is the rejection band shared by both axes: the MAD-derived robust
// sigma scaled to priorSigmaFactor, floored at priorMinExcessMs.
func priorBand(st introDurationStats) int64 {
	robustSigma := float64(st.madMs) * 1.4826
	return max(int64(priorSigmaFactor*robustSigma), priorMinExcessMs)
}

// durationExceedsPrior reports whether durMs is an over-long outlier against
// the season distribution (absorbed recap / music-bleed class). Only the long
// side is policed - see the file comment.
func durationExceedsPrior(durMs int64, st introDurationStats) bool {
	return durMs-st.medianMs > priorBand(st)
}

// startPrecedesPrior reports whether startMs opens markedly earlier than the
// season's median start: the excess of an over-long window then sits in FRONT
// (absorbed recap), and the end is the boundary to trust. Same statistic,
// same band, applied to the other axis - a season of variable cold opens
// carries a wide start MAD and never triggers this.
func startPrecedesPrior(startMs int64, st introDurationStats) bool {
	return st.medianMs-startMs > priorBand(st)
}

// int64Median returns the median of xs (lower of the two middles for even n,
// which biases the prior slightly conservative). xs is not mutated.
func int64Median(xs []int64) int64 {
	s := append([]int64(nil), xs...)
	slices.Sort(s)
	return s[(len(s)-1)/2]
}

// introRow is one season intro with its episode order, the axis the
// changepoint search runs along.
type introRow struct {
	seg *domainlibrary.MediaSegment
	idx int
}

// introChangepoint looks for the single split, in episode order, where the
// season's intro durations fall into two significantly separated modes (cour
// split, shortened late-season opening). Both sides must be big enough to
// carry their own prior, and the gap between the side medians must clear the
// band of the wider side - the same significance vocabulary as the outlier
// rule, so a noisy season never splits. Among significant candidates the one
// fitting both sides tightest wins. 0 means no split.
func introChangepoint(durationsMs []int64) int {
	n := len(durationsMs)
	if n < 2*priorMinSamples {
		return 0
	}
	best, bestScore := 0, int64(-1)
	for k := priorMinSamples; k <= n-priorMinSamples; k++ {
		stL, _ := seasonIntroDurationStats(durationsMs[:k])
		stR, _ := seasonIntroDurationStats(durationsMs[k:])
		gap := absInt64(stL.medianMs - stR.medianMs)
		if gap <= max(priorBand(stL), priorBand(stR)) {
			continue
		}
		score := totalAbsDev(durationsMs[:k], stL.medianMs) + totalAbsDev(durationsMs[k:], stR.medianMs)
		if bestScore < 0 || score < bestScore {
			best, bestScore = k, score
		}
	}
	return best
}

func totalAbsDev(xs []int64, center int64) int64 {
	var sum int64
	for _, x := range xs {
		sum += absInt64(x - center)
	}
	return sum
}

// reconcileSeasonIntroDurations applies the duration prior over every
// non-manual intro of the season: over-long outliers are shrunk to
// [start, start+median]. Runs after the per-episode detection loop so it sees
// the season's final state, whatever mix of sources produced it. Manual marks
// are never touched (the repo guard enforces it too).
// The same walk feeds the season credits prior (intro_credits_prior.go), so
// the credits rows cost no third pass over the season.
func (s *IntroDetectionService) reconcileSeasonIntroDurations(ctx context.Context, parentID string) {
	episodeType := string(domainlibrary.Episode)
	var intros []introRow
	var credits []creditsPriorRow
	const pageSize = 200
	pos := 0
	for offset := 0; ; offset += pageSize {
		page, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			ParentID: &parentID, Type: &episodeType, Limit: pageSize, Offset: offset,
		})
		if err != nil || len(page) == 0 {
			break
		}
		ids := make([]shared.MediaItemID, len(page))
		for i, ep := range page {
			ids[i] = ep.ID
		}
		byID, err := s.segmentRepo.GetByMediaItemIDs(ctx, ids)
		if err != nil {
			slog.WarnContext(ctx, "intro_prior: batch get segments", slog.String("parent_id", parentID), slog.Any("err", err))
			return
		}
		// Page order, not map order: the changepoint search needs episode
		// order, carried by IndexNumber with the walk position as fallback.
		for _, ep := range page {
			pos++
			idx := pos
			if ep.IndexNumber != nil {
				idx = *ep.IndexNumber
			}
			for _, sg := range byID[ep.ID] {
				if creditsPriorVoter(sg) {
					credits = append(credits, creditsPriorRow{seg: sg, item: ep})
					continue
				}
				if sg.Type != "intro" || sg.Source == "manual" || sg.Source == "manual_propagated" {
					continue
				}
				if sg.EndMs > sg.StartMs {
					intros = append(intros, introRow{seg: sg, idx: idx})
				}
			}
		}
		if len(page) < pageSize {
			break
		}
	}
	slices.SortStableFunc(intros, func(a, b introRow) int { return a.idx - b.idx })
	s.reconcileIntroEras(ctx, parentID, intros)
	s.reconcileSeasonCredits(ctx, credits)
}

// reconcileIntroEras runs the duration prior over the season's intros, split
// into two eras first when a changepoint separates them.
func (s *IntroDetectionService) reconcileIntroEras(ctx context.Context, parentID string, intros []introRow) {
	durationsAll := make([]int64, len(intros))
	for i, r := range intros {
		durationsAll[i] = r.seg.EndMs - r.seg.StartMs
	}
	if k := introChangepoint(durationsAll); k > 0 {
		slog.InfoContext(ctx, "intro_prior: season intro durations split into two eras",
			slog.String("parent_id", parentID),
			slog.Int("changepoint", k),
			slog.Int64("first_median_ms", int64Median(durationsAll[:k])),
			slog.Int64("second_median_ms", int64Median(durationsAll[k:])))
		s.reconcileIntroGroup(ctx, intros[:k])
		s.reconcileIntroGroup(ctx, intros[k:])
		return
	}
	s.reconcileIntroGroup(ctx, intros)
}

// reconcileIntroGroup runs the duration prior over one homogeneous group of
// season intros (the whole season, or one era of a split season).
func (s *IntroDetectionService) reconcileIntroGroup(ctx context.Context, intros []introRow) {
	durations := make([]int64, len(intros))
	starts := make([]int64, len(intros))
	for i, r := range intros {
		durations[i] = r.seg.EndMs - r.seg.StartMs
		starts[i] = r.seg.StartMs
	}
	st, ok := seasonIntroDurationStats(durations)
	if !ok {
		return
	}
	// The start prior only DISAMBIGUATES which end of an over-long window
	// holds the excess - it never flags outliers on its own.
	startSt, startOK := seasonIntroDurationStats(starts)

	// The gentle snap target: a median computed ONLY from corroborated
	// boundaries. Absent enough of those, only the outlier rule applies.
	var supported []int64
	for _, r := range intros {
		if r.seg.Confidence >= priorSnapSupportConf {
			supported = append(supported, r.seg.EndMs-r.seg.StartMs)
		}
	}
	snapSt, snapOK := seasonIntroDurationStats(supported)

	for _, r := range intros {
		dur := r.seg.EndMs - r.seg.StartMs
		corrected := *r.seg
		anchor := "start"
		switch {
		case durationExceedsPrior(dur, st):
			if startOK && startPrecedesPrior(r.seg.StartMs, startSt) {
				// Excess in front (absorbed recap): hold the end, advance the
				// start. Start-anchoring here would keep the recap and
				// truncate the true title sequence.
				corrected.StartMs = corrected.EndMs - st.medianMs
				anchor = "end"
			} else {
				corrected.EndMs = corrected.StartMs + st.medianMs
			}
			// The correction is an inference, not a measurement - cap its
			// confidence below every direct detection so a later re-detection
			// can replace it.
			if corrected.Confidence > 0.75 {
				corrected.Confidence = 0.75
			}
		case snapOK && r.seg.Confidence < priorSnapSupportConf &&
			absInt64(dur-snapSt.medianMs) > priorSnapMinDeltaMs &&
			!durationExceedsPrior(dur, st):
			// Lone-witness end a few seconds off a corroborated season median:
			// adopt the median duration (both directions - music-bleed
			// overshoot AND too-early cutoffs).
			corrected.EndMs = corrected.StartMs + snapSt.medianMs
		default:
			continue
		}
		if _, err := s.segmentRepo.UpsertUnlessManual(ctx, &corrected); err != nil {
			slog.WarnContext(ctx, "intro_prior: adjust duration", slog.String("media_id", string(corrected.MediaItemID)), slog.Any("err", err))
			continue
		}
		slog.InfoContext(ctx, "intro_prior: intro duration reconciled to season median",
			slog.String("media_id", string(corrected.MediaItemID)),
			slog.String("source", corrected.Source),
			slog.String("anchor", anchor),
			slog.Int64("duration_ms", dur),
			slog.Int64("corrected_ms", corrected.EndMs-corrected.StartMs),
			slog.Int64("mad_ms", st.madMs),
			slog.Int("season_samples", st.n))
	}
}

func absInt64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
