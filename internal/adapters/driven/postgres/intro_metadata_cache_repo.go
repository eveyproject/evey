package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// IntroMetadataCacheRepository is the postgres backing for the external
// intro/recap/credits provider cache (intro_metadata_cache table). The
// payload is stored as raw JSONB - the introproviders package owns the
// shape, so this repo is intentionally schema-agnostic at the
// application boundary.
type IntroMetadataCacheRepository struct {
	db *sql.DB
}

func NewIntroMetadataCacheRepository(db *sql.DB) *IntroMetadataCacheRepository {
	return &IntroMetadataCacheRepository{db: db}
}

// Get returns the cached payload bytes and the time the row was stored,
// or (nil, zero, nil) on a cache miss. Any other DB error is propagated.
// Source filtering is intentional - the chain provider stores under
// whichever upstream answered, so a single cache row maps to a single
// source. Reading the most-recent row keeps the API simple at the cost
// of a tiny scan when multiple sources have answered for the same key
// (rare).
func (r *IntroMetadataCacheRepository) Get(ctx context.Context, tmdbID, season, episode int) ([]byte, time.Time, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_metadata_cache.Get")
	defer span.End()
	span.SetAttributes(
		attribute.Int("intro_metadata.tmdb_id", tmdbID),
		attribute.Int("intro_metadata.season", season),
		attribute.Int("intro_metadata.episode", episode),
	)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var payload []byte
	var fetchedAt time.Time
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT payload, fetched_at FROM intro_metadata_cache
		 WHERE tmdb_id = $1 AND season = $2 AND episode = $3
		 ORDER BY fetched_at DESC LIMIT 1`,
		tmdbID, season, episode,
	).Scan(&payload, &fetchedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, time.Time{}, err
	}
	return payload, fetchedAt, nil
}

// Put upserts the cache row. The PK is (tmdb_id, season, episode,
// source) so multiple sources can co-exist; only the per-source row is
// rewritten on each call.
func (r *IntroMetadataCacheRepository) Put(ctx context.Context, tmdbID, season, episode int, source string, payload []byte) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_metadata_cache.Put")
	defer span.End()
	span.SetAttributes(
		attribute.Int("intro_metadata.tmdb_id", tmdbID),
		attribute.String("intro_metadata.source", source),
	)
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO intro_metadata_cache (tmdb_id, season, episode, source, payload, fetched_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (tmdb_id, season, episode, source) DO UPDATE
		   SET payload = EXCLUDED.payload, fetched_at = NOW()`,
		tmdbID, season, episode, source, payload,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// Invalidate deletes all cached rows for a given (tmdb_id, season,
// episode) tuple - useful when an admin notices stale data on a
// specific episode and wants the next detection to refetch upstream.
// Returns the number of rows deleted.
func (r *IntroMetadataCacheRepository) Invalidate(ctx context.Context, tmdbID, season, episode int) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.intro_metadata_cache.Invalidate")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM intro_metadata_cache WHERE tmdb_id = $1 AND season = $2 AND episode = $3`,
		tmdbID, season, episode)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return res.RowsAffected()
}
