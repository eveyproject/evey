package transcoding

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
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
	select {
	case <-s.tickerStop:
		// already closed
	default:
		close(s.tickerStop)
	}
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

// OverrideForHeight returns the active override matching sourceHeight,
// or false if none applies. Called by the handler on every HLS-VOD
// request — lock-free atomic read.
func (s *CapOverrideService) OverrideForHeight(sourceHeight int) (domaintranscoding.CapOverride, bool) {
	cls := domaintranscoding.SourceClassFor(sourceHeight)
	if cls == "" {
		return domaintranscoding.CapOverride{}, false
	}
	list := s.cached.Load()
	if list == nil {
		return domaintranscoding.CapOverride{}, false
	}
	for _, o := range *list {
		if o.SourceClass == cls {
			return o, true
		}
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

	const q = `
		WITH source_height AS (
			SELECT mi.id AS media_item_id, MAX(ms.height) AS height
			FROM media_items mi
			JOIN media_streams ms ON ms.media_item_id = mi.id AND ms.stream_type = 'video'
			GROUP BY mi.id
		), buckets AS (
			SELECT
				CASE
					WHEN sh.height >= 2160 THEN '2160p'
					WHEN sh.height >= 1440 THEN '1440p'
					WHEN sh.height >= 1080 THEN '1080p'
					ELSE NULL
				END AS source_class,
				COUNT(*) FILTER (WHERE pe.buffer_starved = TRUE) AS starved,
				COUNT(*)                                          AS total
			FROM playback_qoe_events pe
			JOIN source_height sh ON sh.media_item_id = pe.media_item_id
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
func lowerCapFor(cls string) int {
	switch cls {
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
