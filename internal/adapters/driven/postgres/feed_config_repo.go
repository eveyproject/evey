package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type FeedAdminConfigRepository struct {
	db *sql.DB
}

func NewFeedAdminConfigRepository(db *sql.DB) *FeedAdminConfigRepository {
	return &FeedAdminConfigRepository{db: db}
}

func (r *FeedAdminConfigRepository) GetAll(ctx context.Context) ([]*feed.AdminConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed_admin_config.GetAll")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT feed_type, enabled, display_order, max_items, params FROM feed_admin_config ORDER BY display_order`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*feed.AdminConfig
	for rows.Next() {
		cfg := &feed.AdminConfig{}
		var ft string
		var paramsJSON []byte
		if err := rows.Scan(&ft, &cfg.Enabled, &cfg.DisplayOrder, &cfg.MaxItems, &paramsJSON); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		cfg.FeedType = feed.Type(ft)
		if err := json.Unmarshal(paramsJSON, &cfg.Params); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "params"), slog.Any("err", err))
		}
		result = append(result, cfg)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *FeedAdminConfigRepository) Upsert(ctx context.Context, cfg *feed.AdminConfig) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed_admin_config.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("feed.type", string(cfg.FeedType)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	paramsJSON, err := json.Marshal(cfg.Params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO feed_admin_config (feed_type, enabled, display_order, max_items, params)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (feed_type) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			display_order = EXCLUDED.display_order,
			max_items = EXCLUDED.max_items,
			params = EXCLUDED.params`,
		string(cfg.FeedType), cfg.Enabled, cfg.DisplayOrder, cfg.MaxItems, paramsJSON,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *FeedAdminConfigRepository) BatchUpsert(ctx context.Context, cfgs []*feed.AdminConfig) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed_admin_config.BatchUpsert")
	defer span.End()
	span.SetAttributes(attribute.Int("feed.count", len(cfgs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO feed_admin_config (feed_type, enabled, display_order, max_items, params)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (feed_type) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			display_order = EXCLUDED.display_order,
			max_items = EXCLUDED.max_items,
			params = EXCLUDED.params`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	defer stmt.Close()

	for _, cfg := range cfgs {
		paramsJSON, err := json.Marshal(cfg.Params)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		if _, err := stmt.ExecContext(ctx, string(cfg.FeedType), cfg.Enabled, cfg.DisplayOrder, cfg.MaxItems, paramsJSON); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *FeedAdminConfigRepository) DeleteByType(ctx context.Context, feedType feed.Type) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed_admin_config.DeleteByType")
	defer span.End()
	span.SetAttributes(attribute.String("feed.type", string(feedType)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM feed_admin_config WHERE feed_type = $1`, string(feedType))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

type FeedProfileConfigRepository struct {
	db *sql.DB
}

func NewFeedProfileConfigRepository(db *sql.DB) *FeedProfileConfigRepository {
	return &FeedProfileConfigRepository{db: db}
}

func (r *FeedProfileConfigRepository) GetByProfile(ctx context.Context, profileID string) ([]*feed.ProfileConfig, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed_profile_config.GetByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT feed_type, enabled, display_order, max_items, params FROM feed_profile_config WHERE profile_id = $1 ORDER BY display_order LIMIT 100`, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*feed.ProfileConfig
	for rows.Next() {
		cfg := &feed.ProfileConfig{ProfileID: shared.ProfileID(profileID)}
		var ft string
		var paramsJSON []byte
		if err := rows.Scan(&ft, &cfg.Enabled, &cfg.DisplayOrder, &cfg.MaxItems, &paramsJSON); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		cfg.FeedType = feed.Type(ft)
		if err := json.Unmarshal(paramsJSON, &cfg.Params); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "params"), slog.Any("err", err))
		}
		result = append(result, cfg)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *FeedProfileConfigRepository) Upsert(ctx context.Context, cfg *feed.ProfileConfig) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed_profile_config.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(cfg.ProfileID)), attribute.String("feed.type", string(cfg.FeedType)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	paramsJSON, err := json.Marshal(cfg.Params)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO feed_profile_config (profile_id, feed_type, enabled, display_order, max_items, params)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (profile_id, feed_type) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			display_order = EXCLUDED.display_order,
			max_items = EXCLUDED.max_items,
			params = EXCLUDED.params`,
		cfg.ProfileID, string(cfg.FeedType), cfg.Enabled, cfg.DisplayOrder, cfg.MaxItems, paramsJSON,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *FeedProfileConfigRepository) Delete(ctx context.Context, profileID string, feedType feed.Type) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed_profile_config.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID), attribute.String("feed.type", string(feedType)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM feed_profile_config WHERE profile_id = $1 AND feed_type = $2`,
		profileID, string(feedType))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *FeedProfileConfigRepository) DeleteByType(ctx context.Context, feedType feed.Type) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.feed_profile_config.DeleteByType")
	defer span.End()
	span.SetAttributes(attribute.String("feed.type", string(feedType)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM feed_profile_config WHERE feed_type = $1`, string(feedType))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ feed.AdminConfigRepository = (*FeedAdminConfigRepository)(nil)
var _ feed.ProfileConfigRepository = (*FeedProfileConfigRepository)(nil)
