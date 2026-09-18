package library

import (
	"context"
	"log/slog"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/domain/shared"
)

// IngestServingPlanConfigKey is the server_config key admins flip to enable
// scan-time serving-plan precompute (mkvgo ops.Ingest, v0.17.0) for
// freshly-discovered files. Read fresh on every new file (see
// loadIngestServingPlanMode) - never cached process-long - so toggling it in
// the admin panel takes effect on the very next scan tick, no restart
// required.
const IngestServingPlanConfigKey = "library.ingest_serving_plan"

// IngestServingPlan mode values (the config's string payload).
const (
	// IngestServingPlanOff: no ingest-plan precompute. Default; also the
	// fallback for a missing key, a nil config repo, or an unrecognised
	// value (a typo'd admin edit degrades to off, never to an unintended
	// mode).
	IngestServingPlanOff = "off"
	// IngestServingPlanPlan: compute the verdict (logged; drives the
	// keyframe warm-up), read-only - Ingest never writes to the media file.
	IngestServingPlanPlan = "plan"
	// IngestServingPlanPlanRepair: in addition to IngestServingPlanPlan,
	// repair a missing seek index in place (mkv.Reindexer, ModeAuto) when the
	// computed verdict flags NeedsReindex, then recompute it. Mirrors app/maintenance/seek_index.go's own
	// repair-then-refresh pattern, but triggered inline at scan time for a
	// brand-new file instead of an admin-initiated library sweep.
	IngestServingPlanPlanRepair = "plan+repair"
)

// ingestItemPause / ingestPoliteItemPause mirror
// KeyframeBackfillWorker.pause: a fixed gap after each file's mkvgo Ingest
// call (and its optional repair), widened while a live playback session is
// active (WithIngestActivePlayback), so the scan-time precompute never
// competes with a stream for I/O on a weak NAS.
const (
	ingestItemPause       = 200 * time.Millisecond
	ingestPoliteItemPause = 2 * time.Second
)

// loadIngestServingPlanMode reads IngestServingPlanConfigKey fresh from the
// config repo (no process-long cache).
func (s *ScanService) loadIngestServingPlanMode(ctx context.Context) string {
	if s.ingestCfg == nil {
		return IngestServingPlanOff
	}
	entry, err := s.ingestCfg.Get(ctx, IngestServingPlanConfigKey)
	if err != nil || entry == nil {
		return IngestServingPlanOff
	}
	switch entry.Value {
	case IngestServingPlanPlan, IngestServingPlanPlanRepair:
		return entry.Value
	default:
		return IngestServingPlanOff
	}
}

// ingestPause returns the current inter-file gap: wider while
// WithIngestActivePlayback reports an active session, matching the "someone
// is watching" backoff the keyframe backfill worker already uses.
func (s *ScanService) ingestPause() time.Duration {
	if s.ingestActivePlayback != nil && s.ingestActivePlayback() > 0 {
		return ingestPoliteItemPause
	}
	return ingestItemPause
}

// applyIngestPlan precomputes and stores the mkvgo serving-plan decision for
// a freshly-created media item's source file. Called synchronously from
// createMediaFile in the scan worker goroutine, so the pacing pause below
// throttles the same per-file loop the rest of createMediaFile already pays
// for (probe, chapters, search index) - no separate background sweep needed.
//
// Every failure (Ingest, repair) is logged at WARN and swallowed: the verdict
// is a scan-time optimisation (repair, keyframe warm-up), never a reason to
// fail the scan that just added the item. Nothing is persisted: the playback
// decision never reads it.
func (s *ScanService) applyIngestPlan(ctx context.Context, mediaItemID shared.MediaItemID, path string) {
	if s.ingestPlanner == nil || path == "" {
		return
	}
	mode := s.loadIngestServingPlanMode(ctx)
	if mode == IngestServingPlanOff {
		return
	}
	// Pace even on a failed attempt below - a durably-failing file (corrupt
	// header, NAS timeout) must not spin the loop hot on every new-file scan.
	defer func() {
		select {
		case <-time.After(s.ingestPause()):
		case <-ctx.Done():
		}
	}()

	plan, err := s.ingestPlanner.Plan(ctx, path, "")
	if err != nil {
		slog.WarnContext(ctx, "ingest plan: mkvgo Ingest failed",
			slog.String("media_item.id", string(mediaItemID)), slog.String("path", path), slog.Any("err", err))
		return
	}

	if mode == IngestServingPlanPlanRepair && plan.NeedsReindex && s.reindexer != nil {
		if _, repairErr := s.reindexer.ReindexInPlace(ctx, path, mkv.ReindexOptions{Mode: mkv.ModeAuto, KeepBackup: true}); repairErr != nil {
			slog.WarnContext(ctx, "ingest plan: repair reindex failed, keeping the pre-repair verdict",
				slog.String("media_item.id", string(mediaItemID)), slog.String("path", path), slog.Any("err", repairErr))
		} else if replanned, replanErr := s.ingestPlanner.Plan(ctx, path, ""); replanErr != nil {
			slog.WarnContext(ctx, "ingest plan: re-plan after repair failed, keeping the pre-repair verdict",
				slog.String("media_item.id", string(mediaItemID)), slog.String("path", path), slog.Any("err", replanErr))
		} else {
			plan = replanned
		}
	}
	slog.DebugContext(ctx, "ingest plan: verdict",
		slog.String("media_item.id", string(mediaItemID)), slog.String("strategy", string(plan.Strategy)), slog.Bool("needs_reindex", plan.NeedsReindex))

	// Consume the plan right away for the first-play fast path: a strategy
	// that serves without a transcode (direct-play, remux-hls) needs the
	// keyframe index - without it the copy rung stays disabled until the
	// debounced backfill sweep (~10 min) or an inline budgeted on-demand walk
	// at first request. Warming it here rides the same paced per-file loop,
	// so a brand-new file is copy-eligible the moment the scan finishes.
	// Best-effort like everything else in this hook.
	if s.keyframeWarmer != nil && plan.Strategy != library.ServingStrategyTranscode {
		if !s.keyframeWarmer(ctx, string(mediaItemID), path) {
			slog.DebugContext(ctx, "ingest plan: keyframe warm-up declined or failed (backfill will catch up)",
				slog.String("media_item.id", string(mediaItemID)))
		}
	}
}
