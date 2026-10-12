package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type UserRatingRepository struct {
	db *sql.DB
}

func NewUserRatingRepository(db *sql.DB) *UserRatingRepository {
	return &UserRatingRepository{db: db}
}

func (r *UserRatingRepository) Upsert(ctx context.Context, rating *library.UserRating) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user_rating.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(rating.ProfileID)), attribute.String("media_item.id", string(rating.MediaItemID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO user_ratings (profile_id, media_item_id, value, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (profile_id, media_item_id)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at
	`, rating.ProfileID, rating.MediaItemID, rating.Value, time.Now())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *UserRatingRepository) Get(ctx context.Context, profileID, mediaItemID string) (*library.UserRating, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user_rating.Get")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID), attribute.String("media_item.id", mediaItemID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	row := txFromCtx(ctx, r.db).QueryRowContext(ctx, `
		SELECT profile_id, media_item_id, value
		FROM user_ratings
		WHERE profile_id = $1 AND media_item_id = $2
	`, profileID, mediaItemID)
	var ur library.UserRating
	if err := row.Scan(&ur.ProfileID, &ur.MediaItemID, &ur.Value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return &ur, nil
}

func (r *UserRatingRepository) GetForItems(ctx context.Context, profileID string, mediaItemIDs []string) (map[string]*library.UserRating, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user_rating.GetForItems")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID), attribute.Int("media_item.count", len(mediaItemIDs)))
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
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, fmt.Sprintf(`
		SELECT profile_id, media_item_id, value
		FROM user_ratings
		WHERE profile_id = $1 AND media_item_id IN (%s)
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]*library.UserRating)
	for rows.Next() {
		var ur library.UserRating
		if err := rows.Scan(&ur.ProfileID, &ur.MediaItemID, &ur.Value); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result[string(ur.MediaItemID)] = &ur
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

var _ library.UserRatingRepository = (*UserRatingRepository)(nil)
