package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/favorite"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type FavoriteRepository struct {
	db *sql.DB
}

func NewFavoriteRepository(db *sql.DB) *FavoriteRepository {
	return &FavoriteRepository{db: db}
}

func (r *FavoriteRepository) Add(ctx context.Context, fav *favorite.Favorite) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.favorite.Add")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(fav.ProfileID)), attribute.String("media_item.id", string(fav.MediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO favorites (id, profile_id, media_item_id, created_at)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (profile_id, media_item_id) DO NOTHING`,
		fav.ID, fav.ProfileID, fav.MediaItemID, fav.CreatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("FavoriteRepo.Add: %w", err)
	}
	return nil
}

func (r *FavoriteRepository) Remove(ctx context.Context, profileID, mediaItemID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.favorite.Remove")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID), attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM favorites WHERE profile_id = $1 AND media_item_id = $2`,
		profileID, mediaItemID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("FavoriteRepo.Remove: %w", err)
	}
	return nil
}

func (r *FavoriteRepository) List(ctx context.Context, profileID string, limit, offset int) ([]*favorite.Favorite, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.favorite.List")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT id, profile_id, media_item_id, created_at
		 FROM favorites WHERE profile_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`,
		profileID, limit, offset,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("FavoriteRepo.List: %w", err)
	}
	defer rows.Close()
	var result []*favorite.Favorite
	for rows.Next() {
		f := &favorite.Favorite{}
		if err := rows.Scan(&f.ID, &f.ProfileID, &f.MediaItemID, &f.CreatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("FavoriteRepo.List scan: %w", err)
		}
		result = append(result, f)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("FavoriteRepo.List rows: %w", err)
	}
	return result, nil
}

func (r *FavoriteRepository) IsFavorite(ctx context.Context, profileID, mediaItemID string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.favorite.IsFavorite")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID), attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var exists bool
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM favorites WHERE profile_id = $1 AND media_item_id = $2)`,
		profileID, mediaItemID,
	).Scan(&exists)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, fmt.Errorf("FavoriteRepo.IsFavorite: %w", err)
	}
	return exists, nil
}

func (r *FavoriteRepository) IsFavoriteMany(ctx context.Context, profileID string, mediaItemIDs []string) (map[string]bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.favorite.IsFavoriteMany")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID), attribute.Int("media_item.count", len(mediaItemIDs)))
	result := make(map[string]bool, len(mediaItemIDs))
	// A nil slice makes pq.Array render SQL NULL, and `x = ANY(NULL)` is NULL,
	// never true - same trap as media_stream_repo. Skip the query entirely.
	if len(mediaItemIDs) == 0 {
		return result, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT media_item_id FROM favorites
		 WHERE profile_id = $1 AND media_item_id = ANY($2)`,
		profileID, pq.Array(mediaItemIDs),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("FavoriteRepo.IsFavoriteMany: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("FavoriteRepo.IsFavoriteMany scan: %w", err)
		}
		result[id] = true
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("FavoriteRepo.IsFavoriteMany rows: %w", err)
	}
	return result, nil
}

var _ favorite.Repository = (*FavoriteRepository)(nil)
