package postgres

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// Library-wide progress of the heavy background tasks, for the admin's
// system-health view. Each count is the twin of the query its worker takes
// its work from: both are built from the same eligible/pending fragments
// below, so "what is left" on screen is exactly what the worker will pick up.
// The in-run skip-sets (exclude) are not applied: an item a worker skips for
// this run is still undone.

const (
	// Complexity classifier: a video file with a known duration.
	complexityEligibleSQL = `mf.file_path <> '' AND mi.type IN ('movie', 'episode') AND mf.duration_ms > 0`
	complexityPendingSQL  = `mi.complexity_class IS NULL`

	// Level backfill: the video codecs it probes, and what each still lacks.
	levelEligibleSQL = `stream_type = 'video'
	   AND lower(codec) IN ('h264', 'hevc', 'av1', 'vp9', 'mpeg2video', 'mpeg4', 'msmpeg4v3')`

	// Keyframe backfill: streams the video-copy rung can serve.
	keyframesEligibleSQL = `stream_type = 'video'
	   AND level IS NOT NULL
	   AND (lower(codec) = 'h264'
	     OR (lower(codec) IN ('hevc', 'av1', 'vp9') AND color_transfer IS NOT NULL
	         AND (dovi_profile IS NULL
	           OR (dovi_profile = 8 AND dovi_bl_compat IN (1, 2, 4)))))`

	// Trickplay: every playable film and episode.
	trickplayEligibleSQL = `mf.file_path != '' AND mi.type IN ('movie', 'episode')`

	// Intro detection: episodes filed under a season.
	introEligibleSQL = `mi.type = 'episode' AND mi.parent_id <> ''`
)

// levelPendingSQL is the level backfill's remainder. A probe that came back
// with nothing to fill stamps level_probe_version, so the h264 and
// hevc/av1/vp9 branches stop being candidates; the idet branch converges on
// its own verdict column (a failed idet probe must stay retryable). The
// version is an integer constant inlined as a literal: no injection surface.
var levelPendingSQL = fmt.Sprintf(`((lower(codec) = 'h264' AND level IS NULL
	      AND level_probe_version < %[1]d)
	     OR (lower(codec) IN ('hevc', 'av1', 'vp9') AND (level IS NULL OR color_transfer IS NULL)
	      AND level_probe_version < %[1]d)
	     OR (lower(codec) IN ('mpeg2video', 'mpeg4', 'msmpeg4v3') AND idet_interlaced IS NULL))`,
	library.LevelProbeVersion)

// keyframesPendingSQL is the keyframe backfill's remainder, the probe
// version bound to the placeholder the caller numbers.
func keyframesPendingSQL(version string) string {
	return `keyframes_ms IS NULL AND keyframes_probe_version < ` + version
}

// trickplayPendingSQL: no manifest at the current version and no failure
// recorded for this very file (size + mtime).
func trickplayPendingSQL(version string) string {
	return `NOT EXISTS (
	     SELECT 1 FROM trickplay_manifests tm
	     WHERE tm.media_item_id = mi.id AND tm.algo_version >= ` + version + `)
	   AND NOT EXISTS (
	     SELECT 1 FROM trickplay_failures tf
	     WHERE tf.media_item_id = mi.id
	       AND tf.file_size = mf.file_size
	       AND tf.mod_time = mf.mod_time)`
}

// introPendingSQL: no covering intro yet, and attempts left at this
// algorithm version. $1..$5 are the locked sources, the covering sources, the
// minimum covering length, the algorithm version and the attempt cap; versioned
// numbers the placeholder of the versioned sources, whose intro only covers when
// the last attempt ran at $4 (IS NOT DISTINCT FROM: an episode never attempted has
// a NULL version, which must read "not current", not an unknown that drops it).
func introPendingSQL(versioned string) string {
	return `(
	        intro.id IS NULL
	     OR NOT (
	            intro.source = ANY($1)
	         OR (intro.source = ANY($2) AND intro.end_ms - intro.start_ms >= $3
	             AND (NOT intro.source = ANY(` + versioned + `) OR att.algo_version IS NOT DISTINCT FROM $4))
	        )
	      )
	  AND (att.media_item_id IS NULL OR att.algo_version <> $4 OR att.attempts < $5)`
}

const introFromSQL = `media_items mi
	LEFT JOIN media_segments intro
	       ON intro.media_item_id = mi.id AND intro.type = 'intro'
	LEFT JOIN intro_detection_attempts att
	       ON att.media_item_id = mi.id`

// queryCoverage sums an inner query that yields one row per item: whether it
// is still pending and its duration in ms.
func queryCoverage(ctx context.Context, r dbExecutor, span string, inner string, args ...any) (bgtasks.Coverage, error) {
	ctx, sp := tracing.StartSpan(ctx, span)
	defer sp.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	q := `SELECT COUNT(*) FILTER (WHERE NOT pending), COUNT(*),
	             COALESCE(SUM(dur) FILTER (WHERE NOT pending), 0), COALESCE(SUM(dur), 0)
	      FROM (` + inner + `) t`
	var c bgtasks.Coverage
	var doneMs, totalMs int64
	if err := txFromCtx(ctx, r).QueryRowContext(ctx, q, args...).Scan(&c.DoneItems, &c.TotalItems, &doneMs, &totalMs); err != nil {
		return bgtasks.Coverage{}, fmt.Errorf("%s: %w", span, err)
	}
	c.DoneSeconds, c.TotalSeconds = float64(doneMs)/1000, float64(totalMs)/1000
	return c, nil
}

// fileDurations is each item's longest file, for the stream-based counts.
const fileDurations = `LEFT JOIN (SELECT media_item_id, MAX(duration_ms) AS dur
	                                FROM media_files GROUP BY media_item_id) f USING (media_item_id)`

func (r *MediaComplexityRepo) ComplexityCoverage(ctx context.Context) (bgtasks.Coverage, error) {
	return queryCoverage(ctx, r.db, "postgres.complexity.Coverage", `
		SELECT bool_or(`+complexityPendingSQL+`) AS pending, MAX(mf.duration_ms) AS dur
		FROM media_items mi
		INNER JOIN media_files mf ON mf.media_item_id = mi.id
		WHERE `+complexityEligibleSQL+`
		GROUP BY mi.id`)
}

func (r *MediaStreamRepository) LevelCoverage(ctx context.Context) (bgtasks.Coverage, error) {
	return queryCoverage(ctx, r.db, "postgres.media_stream.LevelCoverage", `
		SELECT bool_or(`+levelPendingSQL+`) AS pending, MAX(COALESCE(f.dur, 0)) AS dur
		FROM media_streams `+fileDurations+`
		WHERE `+levelEligibleSQL+`
		GROUP BY media_item_id`)
}

func (r *MediaStreamRepository) KeyframesCoverage(ctx context.Context, probeVersion int16) (bgtasks.Coverage, error) {
	return queryCoverage(ctx, r.db, "postgres.media_stream.KeyframesCoverage", `
		SELECT bool_or(`+keyframesPendingSQL("$1")+`) AS pending, MAX(COALESCE(f.dur, 0)) AS dur
		FROM media_streams `+fileDurations+`
		WHERE `+keyframesEligibleSQL+`
		GROUP BY media_item_id`, probeVersion)
}

func (r *TrickplayRepository) TrickplayCoverage(ctx context.Context, minAlgoVersion int) (bgtasks.Coverage, error) {
	return queryCoverage(ctx, r.db, "postgres.trickplay.Coverage", `
		SELECT bool_or(`+trickplayPendingSQL("$1")+`) AS pending, MAX(mf.duration_ms) AS dur
		FROM media_items mi
		INNER JOIN media_files mf ON mf.media_item_id = mi.id
		WHERE `+trickplayEligibleSQL+`
		GROUP BY mi.id`, minAlgoVersion)
}

func (r *MediaSegmentRepository) IntroCoverage(ctx context.Context, algoVersion, maxAttempts int) (bgtasks.Coverage, error) {
	return queryCoverage(ctx, r.db, "postgres.mediasegment.IntroCoverage", `
		SELECT bool_or(`+introPendingSQL("$6")+`) AS pending,
		       COALESCE((SELECT MAX(duration_ms) FROM media_files WHERE media_item_id = mi.id), 0) AS dur
		FROM `+introFromSQL+`
		WHERE `+introEligibleSQL+`
		GROUP BY mi.id`,
		pq.Array(library.IntroLockedSources()),
		pq.Array(library.IntroCoveringSources()),
		library.MinCoveringIntroMs,
		algoVersion, maxAttempts,
		pq.Array(library.IntroVersionedSources()))
}
