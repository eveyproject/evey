package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/favorite"
)

type FavoriteRepository struct {
	db *sql.DB
}

func NewFavoriteRepository(db *sql.DB) *FavoriteRepository {
	return &FavoriteRepository{db: db}
}

func (r *FavoriteRepository) Add(ctx context.Context, fav *favorite.Favorite) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO favorites (id, profile_id, media_item_id, created_at)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (profile_id, media_item_id) DO NOTHING`,
		fav.ID, fav.ProfileID, fav.MediaItemID, fav.CreatedAt,
	)
	return err
}

func (r *FavoriteRepository) Remove(ctx context.Context, profileID, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM favorites WHERE profile_id = $1 AND media_item_id = $2`,
		profileID, mediaItemID,
	)
	return err
}

func (r *FavoriteRepository) List(ctx context.Context, profileID string, limit, offset int) ([]*favorite.Favorite, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, profile_id, media_item_id, created_at
		 FROM favorites WHERE profile_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		profileID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*favorite.Favorite
	for rows.Next() {
		f := &favorite.Favorite{}
		if err := rows.Scan(&f.ID, &f.ProfileID, &f.MediaItemID, &f.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

func (r *FavoriteRepository) IsFavorite(ctx context.Context, profileID, mediaItemID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM favorites WHERE profile_id = $1 AND media_item_id = $2)`,
		profileID, mediaItemID,
	).Scan(&exists)
	return exists, err
}
