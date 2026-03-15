package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gravity-zero/evey/internal/domain/rating"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/lib/pq"
)

type RatingRepository struct {
	db *sql.DB
}

func NewRatingRepository(db *sql.DB) *RatingRepository {
	return &RatingRepository{db: db}
}

func (r *RatingRepository) Upsert(ctx context.Context, rt rating.Rating) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO media_ratings (media_item_id, profile_id, value, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (media_item_id, profile_id)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at
	`, rt.MediaItemID, rt.ProfileID, rt.Value.Int(), now)
	return err
}

func (r *RatingRepository) Get(ctx context.Context, mediaItemID, profileID string) (*rating.Rating, error) {
	var rt rating.Rating
	err := r.db.QueryRowContext(ctx, `
		SELECT media_item_id, profile_id, value, created_at, updated_at
		FROM media_ratings WHERE media_item_id = $1 AND profile_id = $2
	`, mediaItemID, profileID).Scan(&rt.MediaItemID, &rt.ProfileID, &rt.Value, &rt.CreatedAt, &rt.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return &rt, nil
}

func (r *RatingRepository) Delete(ctx context.Context, mediaItemID, profileID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM media_ratings WHERE media_item_id = $1 AND profile_id = $2
	`, mediaItemID, profileID)
	return err
}

func (r *RatingRepository) GetAggregates(ctx context.Context, mediaItemIDs []string) (map[string]rating.Aggregate, error) {
	if len(mediaItemIDs) == 0 {
		return map[string]rating.Aggregate{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT media_item_id, AVG(value)::float8, COUNT(*)
		FROM media_ratings
		WHERE media_item_id = ANY($1)
		GROUP BY media_item_id
	`, pq.Array(mediaItemIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]rating.Aggregate, len(mediaItemIDs))
	for rows.Next() {
		var id string
		var agg rating.Aggregate
		if err := rows.Scan(&id, &agg.Average, &agg.Count); err != nil {
			return nil, err
		}
		result[id] = agg
	}
	return result, rows.Err()
}
