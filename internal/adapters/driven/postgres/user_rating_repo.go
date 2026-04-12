package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type UserRatingRepository struct {
	db *sql.DB
}

func NewUserRatingRepository(db *sql.DB) *UserRatingRepository {
	return &UserRatingRepository{db: db}
}

func (r *UserRatingRepository) Upsert(ctx context.Context, rating *library.UserRating) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_ratings (profile_id, media_item_id, value, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (profile_id, media_item_id)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at
	`, rating.ProfileID, rating.MediaItemID, rating.Value, time.Now())
	return err
}

func (r *UserRatingRepository) Get(ctx context.Context, profileID, mediaItemID string) (*library.UserRating, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := r.db.QueryRowContext(ctx, `
		SELECT profile_id, media_item_id, value
		FROM user_ratings
		WHERE profile_id = $1 AND media_item_id = $2
	`, profileID, mediaItemID)
	var ur library.UserRating
	if err := row.Scan(&ur.ProfileID, &ur.MediaItemID, &ur.Value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	return &ur, nil
}

func (r *UserRatingRepository) GetForItems(ctx context.Context, profileID string, mediaItemIDs []string) (map[string]*library.UserRating, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return map[string]*library.UserRating{}, nil
	}
	placeholders := make([]string, len(mediaItemIDs))
	args := make([]any, len(mediaItemIDs)+1)
	args[0] = profileID
	for i, id := range mediaItemIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT profile_id, media_item_id, value
		FROM user_ratings
		WHERE profile_id = $1 AND media_item_id IN (%s)
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]*library.UserRating)
	for rows.Next() {
		var ur library.UserRating
		if err := rows.Scan(&ur.ProfileID, &ur.MediaItemID, &ur.Value); err != nil {
			return nil, err
		}
		result[ur.MediaItemID] = &ur
	}
	return result, rows.Err()
}

var _ library.UserRatingRepository = (*UserRatingRepository)(nil)
