package transcoding

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// CapOverrideService consumes the CMCD telemetry persisted in
// playback_qoe_events and writes downward caps when a source class
// shows sustained starvation, so the calibration-set ceiling is
// dynamically lowered for problematic content without locking the
// admin out of the ladder forever (TTL'd).
//
// Architecture:
//
//   - A periodic worker (default 30 min) joins playback_qoe_events
//     with media_streams to bucket each event by source resolution
//     (the bucket the override system reasons about — see
//     domain SourceClassFor).
//   - For each bucket with at least minSamples plays AND a
//     starved_ratio > starvedThreshold, an override is written:
//     max_height = next_lower_height(currently_capped_max). The
//     override TTL is overrideTTL (default 7 days).
//   - The handler reads via OverrideForHeight at request time. The
//     service caches the override list in-memory + reloads on every
//     Refresh + every worker tick, so the handler stays lock-free.
//
// The aggregation query is intentionally simple — Phase C MVP.
// Per-codec / per-title granularity is a follow-up if real data
// shows the bucket granularity is too coarse.
type CapOverrideService struct {
	repo   domaintranscoding.CapOverrideRepo
	db     *sql.DB // raw connection for the aggregation join
	logger *slog.Logger

	opts CapOverrideServiceOpts

	cached atomic.Pointer[[]domaintranscoding.CapOverride]

	tickerStop chan struct{}
	tickerDone chan struct{}

	startOnce sync.Once
	closeOnce sync.Once
}

type CapOverrideServiceOpts struct {
	// TickInterval — how often the worker aggregates QoE events and
	// updates overrides. 30 min is conservative; lower it for faster
	// reaction on busy hosts.
	TickInterval time.Duration

	// AggregationWindow — how far back we look at QoE events when
	// computing starvation ratios. 24 h gives enough volume to be
	// stable without dragging in old network conditions.
	AggregationWindow time.Duration

	// MinSamples — minimum playback events in a bucket before we
	// consider applying an override. Below this the ratio is too
	// noisy.
	MinSamples int

	// StarvedThreshold — fraction of starved events that triggers an
	// override. 0.30 = 30 %.
	StarvedThreshold float64

	// OverrideTTL — how long an override sticks before being deleted
	// (and re-evaluated by the worker on the next tick).
	OverrideTTL time.Duration
}

func (o *CapOverrideServiceOpts) applyDefaults() {
	if o.TickInterval == 0 {
		o.TickInterval = 30 * time.Minute
	}
	if o.AggregationWindow == 0 {
		o.AggregationWindow = 24 * time.Hour
	}
	if o.MinSamples == 0 {
		o.MinSamples = 50
	}
	if o.StarvedThreshold == 0 {
		o.StarvedThreshold = 0.30
	}
	if o.OverrideTTL == 0 {
		o.OverrideTTL = 7 * 24 * time.Hour
	}
}

func NewCapOverrideService(repo domaintranscoding.CapOverrideRepo, db *sql.DB, opts CapOverrideServiceOpts, logger *slog.Logger) *CapOverrideService {
	opts.applyDefaults()
	if logger == nil {
		logger = slog.Default()
	}
	s := &CapOverrideService{
		repo:       repo,
		db:         db,
		logger:     logger,
		opts:       opts,
		tickerStop: make(chan struct{}),
		tickerDone: make(chan struct{}),
	}
	empty := []domaintranscoding.CapOverride{}
	s.cached.Store(&empty)
	return s
}

// Start runs the worker loop in a goroutine. Safe to call once; later
// calls are no-ops.
func (s *CapOverrideService) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		go s.loop(ctx)
	})
}

// Close stops the worker and waits for the loop to exit.
func (s *CapOverrideService) Close() {
	// sync.Once, not a check-then-close select: two concurrent Close() calls could
	// both take the default branch and double-close → panic.
	s.closeOnce.Do(func() { close(s.tickerStop) })
	select {
	case <-s.tickerDone:
	case <-time.After(2 * time.Second):
		// loop never started or already exited
	}
}

func (s *CapOverrideService) loop(ctx context.Context) {
	defer close(s.tickerDone)
	// Refresh cache once immediately so handler reads aren't always
	// "empty" on the first tick.
	if err := s.Refresh(ctx); err != nil {
		s.logger.Warn("caps override: initial refresh failed", slog.Any("err", err))
	}
	t := time.NewTicker(s.opts.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-s.tickerStop:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Tick(ctx); err != nil {
				s.logger.Warn("caps override: tick failed", slog.Any("err", err))
			}
		}
	}
}

// Tick runs one aggregation pass + refresh. Exposed so tests + the
// admin-side "recompute now" action can trigger it on demand.
func (s *CapOverrideService) Tick(ctx context.Context) error {
	if _, err := s.repo.DeleteExpired(ctx); err != nil {
		s.logger.Warn("caps override: expire failed", slog.Any("err", err))
	}
	if err := s.aggregateAndUpsert(ctx); err != nil {
		return fmt.Errorf("aggregate: %w", err)
	}
	return s.Refresh(ctx)
}

// Refresh re-reads the override list from the repo into the cache.
func (s *CapOverrideService) Refresh(ctx context.Context) error {
	list, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	s.cached.Store(&list)
	return nil
}

// ResetOverride drops the override for a specific source class and
// refreshes the in-memory cache so the handler sees the change
// immediately. Used by the admin "reset cap" button.
// Returns (true, nil) when a row was actually removed.
func (s *CapOverrideService) ResetOverride(ctx context.Context, sourceClass string) (bool, error) {
	removed, err := s.repo.Delete(ctx, sourceClass)
	if err != nil {
		return false, err
	}
	if removed {
		s.logger.Info("caps override: reset by admin",
			slog.String("source_class", sourceClass))
	}
	if err := s.Refresh(ctx); err != nil {
		// Cache refresh failure is non-fatal — the next tick will
		// reload anyway. Surface a warning so admins notice if
		// /admin/qoe shows the override that should be gone.
		s.logger.Warn("caps override: cache refresh failed after reset", slog.Any("err", err))
	}
	return removed, nil
}

// ListActive returns every cached non-expired override. Exposed for the
// admin UI panel — same atomic snapshot the handler reads on each
// request, so what the admin sees IS what the handler enforces.
func (s *CapOverrideService) ListActive() []domaintranscoding.CapOverride {
	list := s.cached.Load()
	if list == nil {
		return nil
	}
	// Defensive copy: callers (HTTP handler → JSON) shouldn't be able
	// to mutate the cached slice via shared backing array.
	out := make([]domaintranscoding.CapOverride, len(*list))
	copy(out, *list)
	return out
}

// OverrideForHeight returns the active override matching the
// (codec, height) bucket, or false if none applies. Called by the
// handler on every HLS-VOD request — lock-free atomic read.
//
// Match priority:
//  1. Exact "<codec>/<height>" bucket (post-per-codec rows).
//  2. "any/<height>" wildcard bucket (handler couldn't determine
//     the source codec).
//  3. Legacy "<height>" row (pre-per-codec MVP rows still in DB
//     during the rolling upgrade window; the next worker tick
//     re-aggregates and overwrites them with codec-aware ones).
//
// The first match wins; we don't merge overrides (the lowest cap
// among matches wouldn't be meaningful in practice — the worker
// already picks one rung lower than current).
func (s *CapOverrideService) OverrideForHeight(sourceCodec string, sourceHeight int) (domaintranscoding.CapOverride, bool) {
	cls := domaintranscoding.SourceClassFor(sourceCodec, sourceHeight)
	if cls == "" {
		return domaintranscoding.CapOverride{}, false
	}
	list := s.cached.Load()
	if list == nil {
		return domaintranscoding.CapOverride{}, false
	}

	// Compute fallback class strings ONCE outside the loop.
	wildcardCls := domaintranscoding.SourceClassFor("", sourceHeight) // "any/<height>"
	var legacyCls string                                              // "<height>" (pre-per-codec)
	switch {
	case sourceHeight >= 2160:
		legacyCls = "2160p"
	case sourceHeight >= 1440:
		legacyCls = "1440p"
	case sourceHeight >= 1080:
		legacyCls = "1080p"
	}

	var exact, wildcard, legacy *domaintranscoding.CapOverride
	for i := range *list {
		o := &(*list)[i]
		switch o.SourceClass {
		case cls:
			exact = o
		case wildcardCls:
			wildcard = o
		case legacyCls:
			if domaintranscoding.SourceClassMatchesLegacy(o.SourceClass) {
				legacy = o
			}
		}
	}
	if exact != nil {
		return *exact, true
	}
	if wildcard != nil {
		return *wildcard, true
	}
	if legacy != nil {
		return *legacy, true
	}
	return domaintranscoding.CapOverride{}, false
}

// aggregateAndUpsert is the heart of the feedback loop. Bucket the
// last AggregationWindow of QoE events by source resolution (joined
// from media_streams), compute starved_ratio per bucket, and write an
// override one rung below the calibrated cap whenever a bucket
// exceeds the threshold.
//
// We DON'T touch the override when a bucket is below threshold —
// existing overrides stay until expires_at, then the worker
// re-evaluates. That avoids flapping caps when a class flirts with
// the threshold.
func (s *CapOverrideService) aggregateAndUpsert(ctx context.Context) error {
	if s.db == nil {
		return nil // tests without a real DB
	}

	// The aggregation joins playback_qoe_events (potentially 100k+
	// rows over the AggregationWindow) with media_items + media_streams.
	// Wrap in a deadline so a slow query can't hang the worker tick
	// indefinitely — the loop catches the error and retries on the
	// next interval. 30 s is generous vs the typical sub-second
	// query, tight enough that a stalled DB doesn't lose more than
	// half a tick.
	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Per-codec granularity: a 4K HEVC source has a heavier decode
	// profile than a 4K H.264 source (B-frame chains, multi-ref motion
	// comp), so a starvation pattern on one shouldn't auto-cap the
	// other. We bucket by (codec, height_bucket) and normalise the
	// codec name with the same rules as SourceClassFor — that way the
	// override the worker writes here matches what the handler looks
	// up at request time.
	//
	// Codec normalisation lives in SQL because we want a single
	// aggregation query instead of pulling raw rows into Go (millions
	// of QoE events at scale). The CASE mirrors normaliseSourceCodec.
	const q = `
		WITH source_meta AS (
			SELECT
				mi.id AS media_item_id,
				MAX(ms.height) AS height,
				LOWER(MAX(ms.codec) FILTER (WHERE ms.stream_type = 'video')) AS codec
			FROM media_items mi
			JOIN media_streams ms ON ms.media_item_id = mi.id AND ms.stream_type = 'video'
			GROUP BY mi.id
		), normalised AS (
			SELECT
				sm.media_item_id,
				sm.height,
				CASE
					WHEN sm.codec IN ('h264', 'avc', 'avc1')          THEN 'h264'
					WHEN sm.codec IN ('hevc', 'h265', 'hev1', 'hvc1') THEN 'hevc'
					WHEN sm.codec IN ('av1', 'av01')                  THEN 'av1'
					WHEN sm.codec = 'vp9'                              THEN 'vp9'
					ELSE 'any'
				END AS codec_bucket
			FROM source_meta sm
		), buckets AS (
			SELECT
				CASE
					WHEN nm.height >= 2160 THEN nm.codec_bucket || '/2160p'
					WHEN nm.height >= 1440 THEN nm.codec_bucket || '/1440p'
					WHEN nm.height >= 1080 THEN nm.codec_bucket || '/1080p'
					ELSE NULL
				END AS source_class,
				COUNT(*) FILTER (WHERE pe.buffer_starved = TRUE) AS starved,
				COUNT(*)                                          AS total
			FROM playback_qoe_events pe
			JOIN normalised nm ON nm.media_item_id = pe.media_item_id
			WHERE pe.ts >= NOW() - $1::interval
			GROUP BY source_class
		)
		SELECT source_class, starved, total
		FROM buckets
		WHERE source_class IS NOT NULL
		  AND total >= $2`
	rows, err := s.db.QueryContext(queryCtx, q, intervalArg(s.opts.AggregationWindow), s.opts.MinSamples)
	if err != nil {
		return fmt.Errorf("aggregation query: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	for rows.Next() {
		var cls string
		var starved, total int64
		if err := rows.Scan(&cls, &starved, &total); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		if total == 0 {
			continue
		}
		ratio := float64(starved) / float64(total)
		if ratio < s.opts.StarvedThreshold {
			continue
		}
		newCap := lowerCapFor(cls)
		if newCap == 0 {
			continue // bucket already at floor
		}
		override := domaintranscoding.CapOverride{
			SourceClass:  cls,
			MaxHeight:    newCap,
			Reason:       fmt.Sprintf("starved %d/%d=%.0f%% over %s", starved, total, ratio*100, s.opts.AggregationWindow),
			StarvedRatio: ratio,
			SamplesCount: int(total),
			AppliedAt:    now,
			ExpiresAt:    now.Add(s.opts.OverrideTTL),
		}
		if err := s.repo.Upsert(ctx, override); err != nil {
			s.logger.Warn("caps override: upsert failed",
				slog.String("class", cls), slog.Any("err", err))
			continue
		}
		s.logger.Info("caps override: applied",
			slog.String("source_class", cls),
			slog.Int("new_max_height", newCap),
			slog.Float64("starved_ratio", ratio),
			slog.Int("samples", int(total)))
	}
	return rows.Err()
}

// lowerCapFor maps a source-class bucket to the next-lower height
// rung we'd cap to. 2160p is the most likely problematic bucket; the
// stepdowns are conservative (one rung at a time so we don't
// over-correct on a transient network event).
//
// Accepts both per-codec ("hevc/2160p") and legacy ("2160p") class
// strings — only the height suffix matters for the height cap. The
// codec is informational for the override row.
func lowerCapFor(cls string) int {
	heightPart := cls
	if i := strings.IndexByte(cls, '/'); i >= 0 {
		heightPart = cls[i+1:]
	}
	switch heightPart {
	case "2160p":
		return 1080 // skip 1440p — the cap-1080 case is the proven safe one
	case "1440p":
		return 1080
	case "1080p":
		return 720
	}
	return 0
}

// intervalArg formats a duration as a postgres interval literal. The
// query uses `$1::interval` so we pass a "Ns" / "Nh" string and let
// pg parse it.
func intervalArg(d time.Duration) string {
	if d < time.Second {
		d = time.Second
	}
	return fmt.Sprintf("%d seconds", int(d.Seconds()))
}
