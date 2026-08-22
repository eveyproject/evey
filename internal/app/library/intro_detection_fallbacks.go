package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// Fallback intro/credits detection strategies when the main pipeline yields nothing.

// waitForManifest polls for the trickplay manifest, waiting up to 24h.
func (s *IntroDetectionService) waitForManifest(ctx context.Context, mediaID shared.MediaItemID) (*domainlibrary.TrickplayManifest, error) {
	manifest, err := s.trickplayRepo.GetByMediaItemID(ctx, mediaID)
	if err == nil {
		return manifest, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, fmt.Errorf("get trickplay manifest: %w", err)
	}

	deadline := time.Now().Add(24 * time.Hour)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, errors.New("trickplay manifest not available after 24h")
			}
			m, e := s.trickplayRepo.GetByMediaItemID(ctx, mediaID)
			if e == nil {
				return m, nil
			}
			if !errors.Is(e, shared.ErrNotFound) {
				return nil, fmt.Errorf("get trickplay manifest: %w", e)
			}
		}
	}
}

// concurrentFallback runs the audio silence fallback. It kept a concurrent
// subtitle-gap sibling for a while, but that path was never implemented (the
// stub always answered not-found) - the goroutine pair, channel and recover
// ceremony ran on every fallback for nothing and were removed 2026-08-06. The
// name and signature stay: the call site does not care how many detectors run
// behind it, and the season-median snap now carries the subtitle signal
// through sibling episodes instead.
func (s *IntroDetectionService) concurrentFallback(ctx context.Context, item *domainlibrary.MediaItem) (startMs, endMs int64, source string, confidence float64, err error) {
	if ctx.Err() != nil {
		return 0, 0, "", 0, ctx.Err()
	}
	start, end, found, ferr := s.detector.AudioFallback(ctx, item.File.Path, 600)
	if ferr != nil || !found {
		return 0, 0, "", 0, ferr
	}
	return start, end, "audio", 0.5, nil
}

// detectStudioLogo attempts to match the first ~90 seconds of trickplay frames
// against known studio logo fingerprints. Returns nil when no manifest is available,
// when no patterns match, or when the catalog has no fingerprints yet.
// Does NOT wait for a manifest - skips immediately if not present.
func (s *IntroDetectionService) detectStudioLogo(ctx context.Context, item *domainlibrary.MediaItem) *domainlibrary.MediaSegment {
	manifest, err := s.trickplayRepo.GetByMediaItemID(ctx, item.ID)
	if err != nil {
		return nil
	}
	if manifest.Interval <= 0 {
		return nil
	}

	// Extract only enough frames to cover 90 seconds.
	maxFrames := max(90/manifest.Interval, 1)
	hashes, _, hashErr := s.extractEpisodeHashes(manifest, maxFrames)
	if hashErr != nil || len(hashes) == 0 {
		return nil
	}

	startMs, endMs, found := s.detector.DetectStudioLogo(hashes, manifest.Interval)
	if !found {
		return nil
	}
	return &domainlibrary.MediaSegment{
		Type:       "studio_logo",
		StartMs:    startMs,
		EndMs:      endMs,
		Confidence: 0.7,
		Source:     "logo_match",
	}
}

// creditsSuspect reports whether a stored credits row starts implausibly far
// from the end of the file, using the SAME bound as the serving gate and the
// detector - the repair must target exactly what serving refuses. Reads the
// default floor rather than the admin override: it runs once per skipped
// episode of a page, and a config read per episode would put the sweep back on
// an N+1.
func creditsSuspect(credits *domainlibrary.MediaSegment, durationMs int64) bool {
	return domainlibrary.CreditsLeadImplausible(
		durationMs-credits.StartMs, durationMs, domainlibrary.SuspiciousCreditsLeadMs)
}

// refreshSuspectCredits re-runs the credits detector for an episode whose
// stored blackdetect credits start implausibly far from the end. Called for
// episodes the season sweep otherwise skips (their intro is covered), so a
// credits-parser fix still reaches them. Self-extinguishing, which matters
// because the scan is a full blackdetect pass over the last 20% of the file:
// every outcome leaves the episode in a state the next sweep will not scan
// again. Either the refreshed row is plausible (no longer suspect), or it is
// deleted - when the detector finds nothing AND when what it finds is no better
// than the row it would replace, an irreparable file that used to be re-scanned
// on every sweep forever. A missing credits marker beats a mid-scene skip
// prompt, and the episode returns to detection through the normal pipeline if
// it ever loses its intro coverage.
func (s *IntroDetectionService) refreshSuspectCredits(ctx context.Context, item *domainlibrary.MediaItem, existing []*domainlibrary.MediaSegment) {
	var credits *domainlibrary.MediaSegment
	for _, sg := range existing {
		if sg.Type == "end_credits" {
			credits = sg
			break
		}
	}
	if credits == nil || credits.Source != "blackdetect" {
		return
	}
	if item.File == nil || item.File.Path == "" || item.File.Duration.IsZero() {
		return
	}
	durationMs := item.File.Duration.Milliseconds()
	if !creditsSuspect(credits, durationMs) {
		return
	}
	seg, err := s.detectEndCredits(ctx, item)
	if err != nil {
		slog.WarnContext(ctx, "intro_detection: suspect credits refresh failed",
			slog.String("media_id", string(item.ID)), slog.Any("err", err))
		return
	}
	if seg != nil && creditsSuspect(seg, durationMs) {
		slog.InfoContext(ctx, "intro_detection: suspect credits not repairable, dropping",
			slog.String("media_id", string(item.ID)),
			slog.Int64("old_start_ms", credits.StartMs),
			slog.Int64("new_start_ms", seg.StartMs))
		seg = nil
	}
	if seg == nil {
		if delErr := s.segmentRepo.DeleteByMediaItemIDAndType(ctx, item.ID, "end_credits"); delErr != nil {
			slog.WarnContext(ctx, "intro_detection: suspect credits delete failed",
				slog.String("media_id", string(item.ID)), slog.Any("err", delErr))
			return
		}
		slog.InfoContext(ctx, "intro_detection: suspect credits removed (nothing found on refresh)",
			slog.String("media_id", string(item.ID)),
			slog.Int64("old_start_ms", credits.StartMs))
		return
	}
	seg.ID = uuid.New().String()
	seg.MediaItemID = item.ID
	if _, err := s.segmentRepo.UpsertUnlessManual(ctx, seg); err != nil {
		slog.WarnContext(ctx, "intro_detection: suspect credits upsert failed",
			slog.String("media_id", string(item.ID)), slog.Any("err", err))
		return
	}
	slog.InfoContext(ctx, "intro_detection: suspect credits refreshed",
		slog.String("media_id", string(item.ID)),
		slog.Int64("old_start_ms", credits.StartMs),
		slog.Int64("new_start_ms", seg.StartMs))
}

// detectEndCredits runs the credits detector on the file and returns a
// MediaSegment if found. Caller is responsible for setting ID and MediaItemID
// before persisting.
func (s *IntroDetectionService) detectEndCredits(ctx context.Context, item *domainlibrary.MediaItem) (*domainlibrary.MediaSegment, error) {
	if item.File == nil || item.File.Path == "" {
		return nil, nil
	}
	durationSec := item.File.Duration.Seconds()
	if durationSec < 60 {
		return nil, nil
	}
	startMs, endMs, found, err := s.detector.DetectEndCredits(ctx, item.File.Path, durationSec)
	if err != nil {
		return nil, fmt.Errorf("detect end credits: %w", err)
	}
	if !found {
		return nil, nil
	}
	return &domainlibrary.MediaSegment{
		Type:       "end_credits",
		StartMs:    startMs,
		EndMs:      endMs,
		Confidence: 0.6,
		Source:     "blackdetect",
	}, nil
}
