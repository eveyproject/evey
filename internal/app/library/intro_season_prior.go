package library

import (
	"context"
	"log/slog"
	"slices"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
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

// introDurationStats is the season's robust intro-duration distribution.
type introDurationStats struct {
	medianMs int64
	madMs    int64
	n        int
}

// seasonIntroDurationStats computes median and MAD (median absolute deviation)
// of the given durations. ok is false below priorMinSamples.
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

// durationExceedsPrior reports whether durMs is an over-long outlier against
// the season distribution (absorbed recap / music-bleed class). Only the long
// side is policed - see the file comment.
func durationExceedsPrior(durMs int64, st introDurationStats) bool {
	robustSigma := float64(st.madMs) * 1.4826
	band := max(int64(priorSigmaFactor*robustSigma), priorMinExcessMs)
	return durMs-st.medianMs > band
}

// int64Median returns the median of xs (lower of the two middles for even n,
// which biases the prior slightly conservative). xs is not mutated.
func int64Median(xs []int64) int64 {
	s := append([]int64(nil), xs...)
	slices.Sort(s)
	return s[(len(s)-1)/2]
}

// reconcileSeasonIntroDurations applies the duration prior over every
// non-manual intro of the season: over-long outliers are shrunk to
// [start, start+median]. Runs after the per-episode detection loop so it sees
// the season's final state, whatever mix of sources produced it. Manual marks
// are never touched (the repo guard enforces it too).
func (s *IntroDetectionService) reconcileSeasonIntroDurations(ctx context.Context, parentID string) {
	episodeType := string(domainlibrary.Episode)
	type introRow struct {
		seg *domainlibrary.MediaSegment
	}
	var intros []introRow
	const pageSize = 200
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
		for _, segs := range byID {
			for _, sg := range segs {
				if sg.Type != "intro" || sg.Source == "manual" || sg.Source == "manual_propagated" {
					continue
				}
				if sg.EndMs > sg.StartMs {
					intros = append(intros, introRow{seg: sg})
				}
			}
		}
		if len(page) < pageSize {
			break
		}
	}

	durations := make([]int64, len(intros))
	for i, r := range intros {
		durations[i] = r.seg.EndMs - r.seg.StartMs
	}
	st, ok := seasonIntroDurationStats(durations)
	if !ok {
		return
	}

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
		switch {
		case durationExceedsPrior(dur, st):
			corrected.EndMs = corrected.StartMs + st.medianMs
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
