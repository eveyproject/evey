package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// introPendingSeasonsDefaultLimit bounds a ListSeasonsPendingIntroDetection
// batch when the caller asks for none. Unlike the paginated List queries this
// one is not covered by DefaultListLimit, so it carries its own bound: an
// unbounded query is what let a 500-season truncation go unnoticed for weeks.
const introPendingSeasonsDefaultLimit = 100

// segmentSourceFingerprintSQL stamps a written segment with the file it was
// found in: the caller's MediaSegment.SourceFingerprint, else the file the row
// points at now. An operator mark ($7 locked) is no file's and stays NULL.
const segmentSourceFingerprintSQL = `CASE WHEN $7 IN ('manual', 'manual_propagated') THEN NULL
	ELSE COALESCE(NULLIF($9, ''), (SELECT mf.file_fingerprint FROM media_files mf WHERE mf.media_item_id = $2)) END`

// segmentOfCurrentFileSQL keeps the segments that describe the file the item
// points at now: an operator mark, a row from before stamping (NULL), or one
// stamped with that file. A segment found in a replaced file is neither served
// nor counted as coverage, so the sweep detects the new one.
func segmentOfCurrentFileSQL(alias string) string {
	return `(` + alias + `.source_fingerprint IS NULL OR ` + alias + `.source_fingerprint =
	    (SELECT mf.file_fingerprint FROM media_files mf WHERE mf.media_item_id = ` + alias + `.media_item_id))`
}

type MediaSegmentRepository struct {
	db *sql.DB
}

func NewMediaSegmentRepository(db *sql.DB) *MediaSegmentRepository {
	return &MediaSegmentRepository{db: db}
}

func (r *MediaSegmentRepository) GetByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) ([]*library.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.GetByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT ms.id, ms.media_item_id, ms.type, ms.start_ms, ms.end_ms, ms.confidence, ms.source, COALESCE(ms.source_fingerprint, ''), ms.created_at
		 FROM media_segments ms WHERE ms.media_item_id = $1 AND `+segmentOfCurrentFileSQL("ms")+` ORDER BY ms.start_ms`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaSegment
	for rows.Next() {
		seg := &library.MediaSegment{}
		if err := rows.Scan(&seg.ID, &seg.MediaItemID, &seg.Type, &seg.StartMs, &seg.EndMs, &seg.Confidence, &seg.Source, &seg.SourceFingerprint, &seg.CreatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, seg)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// GetByMediaItemIDs batches GetByMediaItemID for many items into a single
// query, keyed by media item id. An id with no rows is simply absent from the
// returned map. Used by the intro detection season sweep to replace a
// per-episode GetByMediaItemID call with one query for the whole page.
func (r *MediaSegmentRepository) GetByMediaItemIDs(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]*library.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.GetByMediaItemIDs")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return map[shared.MediaItemID][]*library.MediaSegment{}, nil
	}
	ids := make([]string, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		ids[i] = string(id)
	}

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT ms.id, ms.media_item_id, ms.type, ms.start_ms, ms.end_ms, ms.confidence, ms.source, COALESCE(ms.source_fingerprint, ''), ms.created_at
		 FROM media_segments ms WHERE ms.media_item_id = ANY($1) AND `+segmentOfCurrentFileSQL("ms")+` ORDER BY ms.media_item_id, ms.start_ms`, pq.Array(ids))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[shared.MediaItemID][]*library.MediaSegment, len(mediaItemIDs))
	for rows.Next() {
		seg := &library.MediaSegment{}
		if err := rows.Scan(&seg.ID, &seg.MediaItemID, &seg.Type, &seg.StartMs, &seg.EndMs, &seg.Confidence, &seg.Source, &seg.SourceFingerprint, &seg.CreatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result[seg.MediaItemID] = append(result[seg.MediaItemID], seg)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MediaSegmentRepository) Upsert(ctx context.Context, seg *library.MediaSegment) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(seg.MediaItemID)), attribute.String("segment.type", seg.Type))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if seg.ID == "" {
		seg.ID = uuid.New().String()
	}
	if seg.CreatedAt.IsZero() {
		seg.CreatedAt = time.Now()
	}

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_segments (id, media_item_id, type, start_ms, end_ms, confidence, source, created_at, source_fingerprint)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, `+segmentSourceFingerprintSQL+`)
		 ON CONFLICT (media_item_id, type) DO UPDATE SET
		   start_ms   = EXCLUDED.start_ms,
		   end_ms     = EXCLUDED.end_ms,
		   confidence = EXCLUDED.confidence,
		   source     = EXCLUDED.source,
		   source_fingerprint = EXCLUDED.source_fingerprint`,
		seg.ID, seg.MediaItemID, seg.Type, seg.StartMs, seg.EndMs, seg.Confidence, seg.Source, seg.CreatedAt, seg.SourceFingerprint,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// UpsertUnlessManual inserts the segment, or updates the existing
// (media_item_id, type) row only when its source is not an operator mark. The
// ON CONFLICT ... DO UPDATE ... WHERE clause evaluates the manual-lock test
// against the locked existing row, so this is atomic: a concurrent
// auto-detection can't read "no manual lock" and then overwrite a manual mark
// set in the race window (the read-then-write that GetByMediaItemID + Upsert
// allowed). Returns applied=false when the existing row's manual lock skipped
// the update.
func (r *MediaSegmentRepository) UpsertUnlessManual(ctx context.Context, seg *library.MediaSegment) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.UpsertUnlessManual")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(seg.MediaItemID)), attribute.String("segment.type", seg.Type))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if seg.ID == "" {
		seg.ID = uuid.New().String()
	}
	if seg.CreatedAt.IsZero() {
		seg.CreatedAt = time.Now()
	}

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_segments (id, media_item_id, type, start_ms, end_ms, confidence, source, created_at, source_fingerprint)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, `+segmentSourceFingerprintSQL+`)
		 ON CONFLICT (media_item_id, type) DO UPDATE SET
		   start_ms   = EXCLUDED.start_ms,
		   end_ms     = EXCLUDED.end_ms,
		   confidence = EXCLUDED.confidence,
		   source     = EXCLUDED.source,
		   source_fingerprint = EXCLUDED.source_fingerprint
		 WHERE media_segments.source NOT IN ('manual', 'manual_propagated')`,
		seg.ID, seg.MediaItemID, seg.Type, seg.StartMs, seg.EndMs, seg.Confidence, seg.Source, seg.CreatedAt, seg.SourceFingerprint,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpsertUnlessOperatorManual is UpsertUnlessManual that locks ONLY against the
// operator's hard 'manual' mark - 'manual_propagated' rows ARE overwritable. The
// intro propagation path needs this: it writes 'manual_propagated' and must be
// able to refresh its own previous propagation on a re-run, while the atomic
// WHERE still closes the TOCTOU against an operator 'manual' that lands between
// the propagation read-guard and this write. Returns applied=false when an
// existing 'manual' mark skipped the update.
func (r *MediaSegmentRepository) UpsertUnlessOperatorManual(ctx context.Context, seg *library.MediaSegment) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.UpsertUnlessOperatorManual")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(seg.MediaItemID)), attribute.String("segment.type", seg.Type))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if seg.ID == "" {
		seg.ID = uuid.New().String()
	}
	if seg.CreatedAt.IsZero() {
		seg.CreatedAt = time.Now()
	}

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO media_segments (id, media_item_id, type, start_ms, end_ms, confidence, source, created_at, source_fingerprint)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, `+segmentSourceFingerprintSQL+`)
		 ON CONFLICT (media_item_id, type) DO UPDATE SET
		   start_ms   = EXCLUDED.start_ms,
		   end_ms     = EXCLUDED.end_ms,
		   confidence = EXCLUDED.confidence,
		   source     = EXCLUDED.source,
		   source_fingerprint = EXCLUDED.source_fingerprint
		 WHERE media_segments.source <> 'manual'`,
		seg.ID, seg.MediaItemID, seg.Type, seg.StartMs, seg.EndMs, seg.Confidence, seg.Source, seg.CreatedAt, seg.SourceFingerprint,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *MediaSegmentRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID shared.MediaItemID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.DeleteByMediaItemID")
	defer span.End()
	span.SetAttributes(attribute.String("media_item.id", string(mediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM media_segments WHERE media_item_id = $1`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *MediaSegmentRepository) DeleteByMediaItemIDAndType(ctx context.Context, mediaItemID shared.MediaItemID, segType string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.DeleteByMediaItemIDAndType")
	defer span.End()
	span.SetAttributes(
		attribute.String("media_item.id", string(mediaItemID)),
		attribute.String("segment.type", segType),
	)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM media_segments WHERE media_item_id = $1 AND type = $2`,
		string(mediaItemID), segType,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// ListSeasonsPendingIntroDetection returns the parent_ids of seasons that still
// have intro-detection work to do. It IS the sweep's work list, re-derived from
// the tables on every batch - the same resumability mechanism as the keyframe
// backfill, and the reason a redeployment landing mid-sweep costs nothing.
//
// The predicate it replaced was `ms.id IS NULL` ("no media_segments row at
// all"), which is dead by construction on a mature catalogue: ~99% of prod
// episodes carry an end_credits row, so the boot sweep found 9 seasons and
// finished in 28s while the real detection work sat untouched.
//
// The coverage rules are NOT restated here: lockedSources, coveringSources and
// minCoveringMs come from IntroDetectionSettled's own lists, so the SQL and the
// in-process sweep cannot drift apart. Episodes with no intro row at all are
// pending via the IS NULL branch. The trickplay JOIN is gone too: chapters, the
// season audio cluster and the providers all detect without sprites, and an
// episode that truly cannot be detected is parked by the attempt counter rather
// than by a join.
func (r *MediaSegmentRepository) ListSeasonsPendingIntroDetection(ctx context.Context, algoVersion, maxAttempts, limit int, exclude []string) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.ListSeasonsPendingIntroDetection")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = introPendingSeasonsDefaultLimit
	}
	if exclude == nil {
		exclude = []string{}
	}

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT DISTINCT mi.parent_id
		FROM `+introFromSQL+`
		WHERE `+introEligibleSQL+`
		  AND `+introPendingSQL("$8")+`
		  AND NOT (mi.parent_id = ANY($6))
		ORDER BY mi.parent_id
		LIMIT $7
	`,
		pq.Array(library.IntroLockedSources()),
		pq.Array(library.IntroCoveringSources()),
		library.MinCoveringIntroMs,
		algoVersion, maxAttempts,
		pq.Array(exclude), limit,
		pq.Array(library.IntroVersionedSources()),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return ids, nil
}

// RecordIntroDetectionAttempt upserts the per-item attempt counter. When the
// stored algo_version differs from algoVersion the algorithm changed, so the
// count resets to 1 (a fresh bounded retry budget at the new version); otherwise
// it increments. See migration 000122.
func (r *MediaSegmentRepository) RecordIntroDetectionAttempt(ctx context.Context, mediaItemID string, algoVersion int) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.RecordIntroDetectionAttempt")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO intro_detection_attempts (media_item_id, attempts, algo_version, last_attempt_at)
		VALUES ($1, 1, $2, now())
		ON CONFLICT (media_item_id) DO UPDATE SET
			attempts = CASE WHEN intro_detection_attempts.algo_version = $2
			                THEN intro_detection_attempts.attempts + 1
			                ELSE 1 END,
			algo_version = $2,
			last_attempt_at = now()`,
		mediaItemID, algoVersion,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// IntroDetectionExhaustedBatch is IntroDetectionExhausted for many items in a
// single query, keyed by media item id. An id at an older algo_version, with
// attempts under the cap, or with no row at all is simply absent from the
// returned map (not exhausted) - mirroring the single-item semantics. Used by
// the intro detection season sweep to replace a per-candidate-episode query
// with one query for the whole page.
func (r *MediaSegmentRepository) IntroDetectionExhaustedBatch(ctx context.Context, mediaItemIDs []string, algoVersion, maxAttempts int) (map[string]bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.IntroDetectionExhaustedBatch")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return map[string]bool{}, nil
	}

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT media_item_id FROM intro_detection_attempts
		WHERE media_item_id = ANY($1) AND algo_version = $2 AND attempts >= $3`,
		pq.Array(mediaItemIDs), algoVersion, maxAttempts,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool, len(mediaItemIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result[id] = true
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// IntroDetectionAttemptVersions returns each attempted item's last algorithm version.
func (r *MediaSegmentRepository) IntroDetectionAttemptVersions(ctx context.Context, mediaItemIDs []string) (map[string]int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.mediasegment.IntroDetectionAttemptVersions")
	defer span.End()
	span.SetAttributes(attribute.Int("media_item.count", len(mediaItemIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return map[string]int{}, nil
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT media_item_id, algo_version FROM intro_detection_attempts
		WHERE media_item_id = ANY($1)`, pq.Array(mediaItemIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int, len(mediaItemIDs))
	for rows.Next() {
		var id string
		var v int
		if err := rows.Scan(&id, &v); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		out[id] = v
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}

var _ library.MediaSegmentRepository = (*MediaSegmentRepository)(nil)
