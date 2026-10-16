package library

import (
	"context"
	"log/slog"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

// Season-level end-credits LEAD prior. The tail scan takes the trailing black
// cluster for the credits and, alone with one file, cannot tell it from a
// scene fade minutes before the end (Raised by Wolves S01, 2026-10-15: 7
// episodes at 62s from the end, 3 fades at 212-261s, all served). Like the
// intro duration prior: an episode whose lead sits far past the season's
// median is re-scanned inside the season's stretch, or withheld. Only
// blackdetect rows take part - their EndMs is the file duration, so the span
// is the lead; chapter and operator rows are authoritative.

// creditsPriorMinExcessMs floors the band: leads vary more than intro
// durations do (a pilot's longer crew list, a post-credits scene). The Raised
// by Wolves pilot sits 23s past the median, the fades 150-200s.
const creditsPriorMinExcessMs = 60_000

// creditsPriorWithheldConf: an outlier whose stretch holds no black drops here,
// under the serve gate (0.55) and under every direct detection. Rows at or
// below it are judged: they neither vote nor get re-scanned, so a sweep never
// re-pays the decode.
const creditsPriorWithheldConf = 0.3

type creditsPriorRow struct {
	seg  *domainlibrary.MediaSegment
	item *domainlibrary.MediaItem
}

// creditsPriorVoter: a detector-made credits row, not yet withheld, inside the
// absolute bound (past it the row is refreshSuspectCredits's, and would only
// inflate the spread here).
func creditsPriorVoter(sg *domainlibrary.MediaSegment) bool {
	if sg.Type != "end_credits" || sg.Source != "blackdetect" || sg.EndMs <= sg.StartMs {
		return false
	}
	if sg.Confidence <= creditsPriorWithheldConf {
		return false
	}
	return !domainlibrary.CreditsLeadImplausible(sg.EndMs-sg.StartMs, sg.EndMs, domainlibrary.SuspiciousCreditsLeadMs)
}

func creditsPriorBand(st introDurationStats) int64 {
	robustSigma := float64(st.madMs) * 1.4826
	return max(int64(priorSigmaFactor*robustSigma), creditsPriorMinExcessMs)
}

// reconcileSeasonCredits runs the lead prior over the season's voting rows,
// collected by the intro prior's walk.
func (s *IntroDetectionService) reconcileSeasonCredits(ctx context.Context, rows []creditsPriorRow) {
	leads := make([]int64, len(rows))
	for i, r := range rows {
		leads[i] = r.seg.EndMs - r.seg.StartMs
	}
	st, ok := seasonIntroDurationStats(leads)
	if !ok {
		return
	}
	band := creditsPriorBand(st)
	for _, r := range rows {
		if ctx.Err() != nil {
			return
		}
		if r.seg.EndMs-r.seg.StartMs-st.medianMs <= band {
			continue
		}
		s.repairOutlierCredits(ctx, r, st, st.medianMs+band)
	}
}

// repairOutlierCredits re-scans the outlier's tail held to maxLeadMs from the
// end and stores what it finds, or withholds the row.
func (s *IntroDetectionService) repairOutlierCredits(ctx context.Context, r creditsPriorRow, st introDurationStats, maxLeadMs int64) {
	item := r.item
	if item.File == nil || item.File.Path == "" || item.File.Duration.IsZero() {
		return
	}
	oldLead := r.seg.EndMs - r.seg.StartMs
	startMs, endMs, found, err := s.detector.DetectEndCreditsWithin(ctx, item.File.Path, item.File.Duration.Seconds(), float64(maxLeadMs)/1000)
	if err != nil {
		slog.WarnContext(ctx, "credits_prior: outlier re-scan failed",
			slog.String("media_id", string(item.ID)), slog.Any("err", err))
		return
	}
	corrected := *r.seg
	if found {
		corrected.StartMs, corrected.EndMs = startMs, endMs
	} else {
		corrected.Confidence = creditsPriorWithheldConf
	}
	if _, err := s.segmentRepo.UpsertUnlessManual(ctx, &corrected); err != nil {
		slog.WarnContext(ctx, "credits_prior: store outlier verdict",
			slog.String("media_id", string(item.ID)), slog.Any("err", err))
		return
	}
	if found {
		slog.InfoContext(ctx, "credits_prior: credits lead outlier re-scanned inside the season's stretch",
			slog.String("media_id", string(item.ID)),
			slog.Int64("old_lead_ms", oldLead),
			slog.Int64("new_lead_ms", corrected.EndMs-corrected.StartMs),
			slog.Int64("median_lead_ms", st.medianMs),
			slog.Int64("mad_ms", st.madMs),
			slog.Int("season_samples", st.n))
		return
	}
	slog.InfoContext(ctx, "credits_prior: credits lead outlier withheld, no black inside the season's stretch",
		slog.String("media_id", string(item.ID)),
		slog.Int64("lead_ms", oldLead),
		slog.Int64("median_lead_ms", st.medianMs),
		slog.Int64("max_lead_ms", maxLeadMs),
		slog.Int("season_samples", st.n))
}
