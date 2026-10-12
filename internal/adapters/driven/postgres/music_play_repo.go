package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// MusicPlayRepository persists per-profile music play counts (music_track_plays).
type MusicPlayRepository struct {
	db *sql.DB
}

func NewMusicPlayRepository(db *sql.DB) *MusicPlayRepository {
	return &MusicPlayRepository{db: db}
}

// IncrementPlay bumps the (profile, track) play count by one and stamps the
// last-played time, inserting the row on first play.
func (r *MusicPlayRepository) IncrementPlay(ctx context.Context, profileID, trackID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.musicplay.IncrementPlay")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID), attribute.String("track.id", trackID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO music_track_plays (profile_id, track_id, play_count, last_played_at)
		 VALUES ($1, $2, 1, NOW())
		 ON CONFLICT (profile_id, track_id) DO UPDATE SET
		   play_count = music_track_plays.play_count + 1,
		   last_played_at = NOW()`,
		profileID, trackID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("MusicPlayRepo.IncrementPlay: %w", err)
	}
	return nil
}

// PlayCountsForProfile returns track id → play count for the profile.
func (r *MusicPlayRepository) PlayCountsForProfile(ctx context.Context, profileID string) (map[string]int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musicplay.PlayCountsForProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT track_id, play_count FROM music_track_plays WHERE profile_id = $1`, profileID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("MusicPlayRepo.PlayCountsForProfile: %w", err)
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var trackID string
		var count int
		if err := rows.Scan(&trackID, &count); err != nil {
			return nil, fmt.Errorf("MusicPlayRepo.PlayCountsForProfile scan: %w", err)
		}
		counts[trackID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MusicPlayRepo.PlayCountsForProfile rows: %w", err)
	}
	return counts, nil
}

// GenreAffinityForProfile returns, per lowercased genre, the profile's total
// play count across that genre - the taste signal the playlist generator scores
// candidates against. Computed in SQL from the full play history so it stays
// accurate even though the generator's candidate pool is only a bounded random
// sample (joining plays to tracks is cheap: the history is small).
func (r *MusicPlayRepository) GenreAffinityForProfile(ctx context.Context, profileID string) (map[string]float64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musicplay.GenreAffinityForProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT LOWER(mt.genre) AS genre, SUM(mtp.play_count)::float8 AS weight
		FROM music_track_plays mtp
		JOIN music_tracks mt ON mt.id = mtp.track_id
		WHERE mtp.profile_id = $1 AND mt.genre <> ''
		GROUP BY LOWER(mt.genre)`, profileID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("MusicPlayRepo.GenreAffinityForProfile: %w", err)
	}
	defer rows.Close()
	weights := make(map[string]float64)
	for rows.Next() {
		var genre string
		var weight float64
		if err := rows.Scan(&genre, &weight); err != nil {
			return nil, fmt.Errorf("MusicPlayRepo.GenreAffinityForProfile scan: %w", err)
		}
		weights[genre] = weight
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MusicPlayRepo.GenreAffinityForProfile rows: %w", err)
	}
	return weights, nil
}
