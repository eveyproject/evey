package library

import (
	"context"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// ServingStrategy is Evey's domain vocabulary for a file's playback delivery
// decision, mirroring mkvgo's ops.ServingStrategy one-for-one (translated at
// the adapter boundary so the mkvgo type never leaks into this package).
type ServingStrategy string

const (
	ServingStrategyDirectPlay ServingStrategy = "direct-play"
	ServingStrategyRemuxHLS   ServingStrategy = "remux-hls"
	ServingStrategyTranscode  ServingStrategy = "transcode"
)

// ServingPlan is the precomputed playback decision for one media item's
// source file, produced at scan time by IngestPlanner and persisted via
// ServingPlanRepository. It kills first-play latency for a freshly-added
// file by moving the Playability + ladder + seek-index analysis mkvgo's
// ops.Ingest performs out of the request path and into the scan.
//
// Consuming this stored plan from the playback decision engine is a future
// iteration - this type only carries the computed result to disk.
type ServingPlan struct {
	MediaItemID shared.MediaItemID
	// Target is the playback target name Ingest decided against (e.g.
	// "mse-generic").
	Target string
	// Strategy is the top-level serving decision.
	Strategy ServingStrategy
	// NeedsReindex mirrors mkvgo's ServingPlan.NeedsReindex: true when
	// Strategy is ServingStrategyRemuxHLS and the source has no
	// head-discoverable seek index yet. Not persisted as its own column -
	// derivable from PlanJSON - but returned by IngestPlanner.Plan so the
	// scan hook can decide whether to repair before storing.
	NeedsReindex bool
	// PlanJSON is the full mkvgo ops.ServingPlan (reasons, ladder,
	// playability report), marshaled by the adapter so no mkvgo type crosses
	// this port. Opaque to every consumer but the admin/debug surface that
	// will eventually render it.
	PlanJSON  []byte
	CreatedAt time.Time
}

// IngestPlanner computes the serving plan for a file, head-only (no decode,
// no transcode) - the domain-facing port over mkvgo's ops.Ingest. Implemented
// by the mkvgo adapter (internal/adapters/driven/mkvgo/ingest.go).
type IngestPlanner interface {
	// Plan decides the serving strategy for path against target (e.g.
	// "mse-generic"; empty defaults to the adapter's own default). The
	// returned plan is never persisted by the planner itself - callers
	// decide what to do with NeedsReindex and where to store the result.
	Plan(ctx context.Context, path, target string) (*ServingPlan, error)
}

// ServingPlanRepository persists the precomputed plan per media item. Kept
// separate from MediaItemCommandRepo (same reasoning as
// MediaStreamRepository) so a plan write never risks the media_items row;
// the FK is ON DELETE CASCADE so a deleted media item takes its plan with it.
type ServingPlanRepository interface {
	// Upsert inserts or replaces the plan for plan.MediaItemID. CreatedAt is
	// stamped by the repository on first insert and left untouched by a
	// later update (a repair-then-replan cycle updates the decision, not the
	// original ingest time).
	Upsert(ctx context.Context, plan *ServingPlan) error
	GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) (*ServingPlan, error)
	DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error
}
