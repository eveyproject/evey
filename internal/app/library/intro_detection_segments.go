package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// Manual and propagated segment mutations for intro detection (set/delete/propagate).

// upsertSegmentRespectingManual upserts unless an existing segment for the same
// (media_item_id, type) has source "manual" or "manual_propagated" - operator
// choices must never be silently overwritten by re-runs of auto-detection.
//
// The manual-lock check and the write are done in ONE atomic statement
// (UpsertUnlessManual) rather than read-then-write: a separate
// GetByMediaItemID + Upsert let a concurrent detection (or an operator
// SetManualSegment) slip a manual mark into the window between the read and
// the write, which the auto-upsert would then clobber.
func (s *IntroDetectionService) upsertSegmentRespectingManual(ctx context.Context, seg *domainlibrary.MediaSegment) error {
	applied, err := s.segmentRepo.UpsertUnlessManual(ctx, seg)
	if err != nil {
		return fmt.Errorf("upsert segment with manual-lock guard: %w", err)
	}
	if !applied {
		slog.InfoContext(ctx, "intro_detection: skipping auto upsert, manual lock present",
			slog.String("media_id", string(seg.MediaItemID)),
			slog.String("type", seg.Type))
	}
	return nil
}

// SetManualSegment upserts a single segment with source="manual" and confidence=1.0.
// This is the operator-driven override path. The (media_item_id, type) DB unique
// constraint makes this idempotent: posting the same type updates start/end.
func (s *IntroDetectionService) SetManualSegment(ctx context.Context, mediaItemID, segType string, startMs, endMs int64) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.SetManualSegment")
	defer span.End()
	if err := validateSegmentInput(segType, startMs, endMs); err != nil {
		return err
	}
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	if item.File != nil && !item.File.Duration.IsZero() {
		durMs := item.File.Duration.Milliseconds()
		if endMs > durMs {
			return fmt.Errorf("end_ms %d exceeds media duration %d", endMs, durMs)
		}
	}
	seg := &domainlibrary.MediaSegment{
		ID:          uuid.New().String(),
		MediaItemID: shared.MediaItemID(mediaItemID),
		Type:        segType,
		StartMs:     startMs,
		EndMs:       endMs,
		Confidence:  1.0,
		Source:      "manual",
	}
	return s.segmentRepo.Upsert(ctx, seg)
}

// DeleteSegment removes a segment of the given type for the given media item.
func (s *IntroDetectionService) DeleteSegment(ctx context.Context, mediaItemID, segType string) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.DeleteSegment")
	defer span.End()
	return s.segmentRepo.DeleteByMediaItemIDAndType(ctx, shared.MediaItemID(mediaItemID), segType)
}

// PropagateSegment takes a manually-defined segment on a reference episode and
// re-runs pHash detection across the season, using the reference's frame hashes
// (covering [startMs, endMs]) as the introHashes set. Each sibling that finds
// a matching contiguous window gets a "manual_propagated" segment. Siblings
// that already have a "manual" lock are skipped (operator choice wins).
//
// Only works on episodes; films return an error.
func (s *IntroDetectionService) PropagateSegment(ctx context.Context, refMediaItemID, segType string) (propagatedTo []string, err error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.intro.PropagateSegment")
	defer span.End()
	refItem, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(refMediaItemID))
	if err != nil {
		return nil, fmt.Errorf("get reference item: %w", err)
	}
	if refItem.Type != domainlibrary.Episode {
		return nil, fmt.Errorf("propagate only works on episodes, got %q", refItem.Type)
	}
	if refItem.ParentID == "" {
		return nil, errors.New("episode has no parent (season) to propagate within")
	}

	refSegments, err := s.segmentRepo.GetByMediaItemID(ctx, refItem.ID)
	if err != nil {
		return nil, fmt.Errorf("load reference segments: %w", err)
	}
	var refSeg *domainlibrary.MediaSegment
	for _, sg := range refSegments {
		if sg.Type == segType {
			refSeg = sg
			break
		}
	}
	if refSeg == nil {
		return nil, fmt.Errorf("no %s segment on reference episode", segType)
	}

	refManifest, err := s.trickplayRepo.GetByMediaItemID(ctx, refItem.ID)
	if err != nil {
		return nil, fmt.Errorf("reference trickplay missing: %w", err)
	}
	refFingerprint := ""
	if refItem.File != nil {
		refFingerprint = refItem.File.Fingerprint
	}
	refAllHashes, refAllMs, err := s.extractEpisodeHashesCached(ctx, string(refItem.ID), refFingerprint, refManifest, manifestTotalFrames(refManifest))
	if err != nil || len(refAllHashes) == 0 {
		return nil, fmt.Errorf("extract reference hashes: %w", err)
	}
	// Pick frames whose real timestamp falls inside [refSeg.StartMs, refSeg.EndMs).
	introHashes := make(map[uint64]bool)
	for i, ms := range refAllMs {
		if ms >= refSeg.StartMs && ms < refSeg.EndMs {
			introHashes[refAllHashes[i]] = true
		}
	}
	if len(introHashes) == 0 {
		return nil, errors.New("reference segment out of trickplay range")
	}

	episodeType := string(domainlibrary.Episode)
	// Page through the whole season - a fixed Limit:100 would silently skip
	// episodes 101+ of a long single-season (anime, daily soaps), leaving them
	// without the propagated segment. Mirrors detectSeason's pagination.
	var siblings []*domainlibrary.MediaItem
	const pageSize = 200
	for offset := 0; ; offset += pageSize {
		page, err := s.mediaItems.List(ctx, domainlibrary.ListMediaItemsFilter{
			ParentID: &refItem.ParentID,
			Type:     &episodeType,
			Limit:    pageSize,
			Offset:   offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list siblings: %w", err)
		}
		if len(page) == 0 {
			break
		}
		siblings = append(siblings, page...)
		if len(page) < pageSize {
			break
		}
	}

	cfg := loadIntroConfig(ctx, s.configRepo)
	maxHamming := cfg.MaxHamming

	// Collect sibling ids up front so the manual-lock guard and the trickplay
	// manifest read below can each be a single batched query instead of one
	// round trip per sibling - a full-season page can be up to 200 episodes.
	siblingIDs := make([]shared.MediaItemID, 0, len(siblings))
	for _, sib := range siblings {
		if sib.ID == refItem.ID {
			continue
		}
		siblingIDs = append(siblingIDs, sib.ID)
	}

	segsByID, segBatchErr := s.segmentRepo.GetByMediaItemIDs(ctx, siblingIDs)
	if segBatchErr != nil {
		// Cannot determine ANY sibling's lock state - skip the whole page rather
		// than risk clobbering an operator "manual" mark we failed to read.
		// Conservative on purpose: a partial/fail-open batch result here could
		// silently propagate over a mark we simply couldn't verify.
		slog.WarnContext(ctx, "intro_detection: propagate skipping page, cannot read segments for manual-lock guard",
			slog.Any("err", segBatchErr))
		return propagatedTo, nil
	}
	// A sibling absent here (no manifest, or the batch read failed outright)
	// is skipped below exactly like the previous per-sibling error handling -
	// trickplay is a "continue on any miss" read, unlike the segments guard
	// above, so a whole-batch failure degrading to "every id absent" is safe.
	trickByID, trickBatchErr := s.trickplayRepo.GetByMediaItemIDs(ctx, siblingIDs)
	if trickBatchErr != nil {
		slog.WarnContext(ctx, "intro_detection: propagate batch trickplay read failed, siblings without a cached manifest will be skipped",
			slog.Any("err", trickBatchErr))
		trickByID = nil
	}

	for _, sib := range siblings {
		if sib.ID == refItem.ID {
			continue
		}
		sibSegs := segsByID[sib.ID]
		manualLock := false
		for _, sg := range sibSegs {
			if sg.Type == segType && sg.Source == "manual" {
				manualLock = true
				break
			}
		}
		if manualLock {
			continue
		}

		sibManifest, ok := trickByID[sib.ID]
		if !ok {
			continue
		}
		sibFingerprint := ""
		if sib.File != nil {
			sibFingerprint = sib.File.Fingerprint
		}
		sibHashes, sibMs, hashErr := s.extractEpisodeHashesCached(ctx, string(sib.ID), sibFingerprint, sibManifest, manifestTotalFrames(sibManifest))
		if hashErr != nil || len(sibHashes) == 0 {
			continue
		}

		cands := allCandidatesGapTolerant(introHashes, sibHashes, sibMs, maxHamming)
		if len(cands) == 0 {
			continue
		}
		windows := make([]matchWindow, 0, len(cands))
		for _, c := range cands {
			windows = append(windows, matchWindow{startMs: c.startMs, endMs: c.endMs})
		}
		refDur := refSeg.EndMs - refSeg.StartMs
		chosen := pickWindowMatchingDuration(windows, refDur)
		if chosen == nil {
			continue
		}
		seg := &domainlibrary.MediaSegment{
			ID:          uuid.New().String(),
			MediaItemID: sib.ID,
			Type:        segType,
			StartMs:     chosen.startMs,
			EndMs:       chosen.endMs,
			Confidence:  0.85,
			Source:      "manual_propagated",
		}
		// Atomic manual-lock guard closes the TOCTOU: the manualLock read-check
		// above can go stale while we hash + match (expensive), so re-check the
		// operator 'manual' mark in the same statement as the write. Operator-only
		// (not 'manual_propagated') so a re-run can still refresh our own prior
		// propagation.
		applied, upsertErr := s.segmentRepo.UpsertUnlessOperatorManual(ctx, seg)
		if upsertErr != nil {
			slog.WarnContext(ctx, "propagate: upsert failed",
				slog.String("sibling", string(sib.ID)),
				slog.Any("err", upsertErr))
			continue
		}
		if !applied {
			slog.InfoContext(ctx, "intro_detection: propagate skipped, operator manual lock won the race",
				slog.String("sibling", string(sib.ID)))
			continue
		}
		propagatedTo = append(propagatedTo, string(sib.ID))
	}
	return propagatedTo, nil
}

// validateSegmentInput rejects unknown types and bad ranges.
func validateSegmentInput(segType string, startMs, endMs int64) error {
	switch segType {
	case "intro", "recap", "end_credits", "studio_logo":
	default:
		return fmt.Errorf("invalid segment type %q", segType)
	}
	if startMs < 0 {
		return errors.New("start_ms must be >= 0")
	}
	if endMs <= startMs {
		return errors.New("end_ms must be > start_ms")
	}
	return nil
}
