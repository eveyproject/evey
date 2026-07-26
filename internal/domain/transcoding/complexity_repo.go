package transcoding

import (
	"context"
	"time"
)

// ComplexityRepo persists per-media classification (class + metrics +
// timestamp). Implemented by the postgres adapter; consumed by the worker
// that classifies idle items and by the playback decision path that reads
// the class to pick the ABR ladder.
type ComplexityRepo interface {
	// Get returns the persisted class + metrics for a media item. The
	// boolean `found` distinguishes "never classified" (false) from "yes,
	// class is empty" (true with class==""). When false, the caller falls
	// back to LadderFilmNormal as the safe default.
	Get(ctx context.Context, mediaItemID string) (entry ComplexityEntry, found bool, err error)

	// Set persists a classification. Overwrites any previous row for the
	// same media item. `computedAt` is the timestamp the metrics were
	// observed - typically time.Now() from the worker, but admins
	// reclassifying manually may want to backdate.
	Set(ctx context.Context, mediaItemID string, class ContentClass, metrics ComplexityMetrics, computedAt time.Time) error

	// ListUnclassified returns up to `limit` media item IDs that have no
	// complexity row yet AND are classifiable at all: video items (movie/
	// episode) with a known duration. Serving never-classifiable rows would
	// re-queue the same IDs forever, since a skip persists nothing.
	// Ordering is repo-defined; the postgres impl returns oldest-first so
	// older content gets classified ahead of new additions.
	ListUnclassified(ctx context.Context, limit int) ([]string, error)
}

// ComplexityEntry is the read-side projection.
type ComplexityEntry struct {
	Class      ContentClass
	Metrics    ComplexityMetrics
	ComputedAt time.Time
}
