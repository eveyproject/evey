package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/feed"
)

type FeedAdminConfigRepository struct {
	db *sql.DB
}

func NewFeedAdminConfigRepository(db *sql.DB) *FeedAdminConfigRepository {
	return &FeedAdminConfigRepository{db: db}
}

func (r *FeedAdminConfigRepository) GetAll(ctx context.Context) ([]*feed.AdminConfig, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT feed_type, enabled, display_order, max_items, params FROM feed_admin_config ORDER BY display_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*feed.AdminConfig
	for rows.Next() {
		cfg := &feed.AdminConfig{}
		var ft string
		var paramsJSON []byte
		if err := rows.Scan(&ft, &cfg.Enabled, &cfg.DisplayOrder, &cfg.MaxItems, &paramsJSON); err != nil {
			return nil, err
		}
		cfg.FeedType = feed.Type(ft)
		if err := json.Unmarshal(paramsJSON, &cfg.Params); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "params"), slog.Any("err", err))
		}
		result = append(result, cfg)
	}
	return result, rows.Err()
}

func (r *FeedAdminConfigRepository) Upsert(ctx context.Context, cfg *feed.AdminConfig) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	paramsJSON, err := json.Marshal(cfg.Params)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO feed_admin_config (feed_type, enabled, display_order, max_items, params)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (feed_type) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			display_order = EXCLUDED.display_order,
			max_items = EXCLUDED.max_items,
			params = EXCLUDED.params`,
		string(cfg.FeedType), cfg.Enabled, cfg.DisplayOrder, cfg.MaxItems, paramsJSON,
	)
	return err
}

func (r *FeedAdminConfigRepository) BatchUpsert(ctx context.Context, cfgs []*feed.AdminConfig) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
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
		return err
	}
	defer stmt.Close()

	for _, cfg := range cfgs {
		paramsJSON, err := json.Marshal(cfg.Params)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, string(cfg.FeedType), cfg.Enabled, cfg.DisplayOrder, cfg.MaxItems, paramsJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *FeedAdminConfigRepository) DeleteByType(ctx context.Context, feedType feed.Type) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM feed_admin_config WHERE feed_type = $1`, string(feedType))
	return err
}

type FeedProfileConfigRepository struct {
	db *sql.DB
}

func NewFeedProfileConfigRepository(db *sql.DB) *FeedProfileConfigRepository {
	return &FeedProfileConfigRepository{db: db}
}

func (r *FeedProfileConfigRepository) GetByProfile(ctx context.Context, profileID string) ([]*feed.ProfileConfig, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT feed_type, enabled, display_order, max_items, params FROM feed_profile_config WHERE profile_id = $1 ORDER BY display_order LIMIT 100`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*feed.ProfileConfig
	for rows.Next() {
		cfg := &feed.ProfileConfig{ProfileID: profileID}
		var ft string
		var paramsJSON []byte
		if err := rows.Scan(&ft, &cfg.Enabled, &cfg.DisplayOrder, &cfg.MaxItems, &paramsJSON); err != nil {
			return nil, err
		}
		cfg.FeedType = feed.Type(ft)
		if err := json.Unmarshal(paramsJSON, &cfg.Params); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "params"), slog.Any("err", err))
		}
		result = append(result, cfg)
	}
	return result, rows.Err()
}

func (r *FeedProfileConfigRepository) Upsert(ctx context.Context, cfg *feed.ProfileConfig) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	paramsJSON, err := json.Marshal(cfg.Params)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO feed_profile_config (profile_id, feed_type, enabled, display_order, max_items, params)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (profile_id, feed_type) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			display_order = EXCLUDED.display_order,
			max_items = EXCLUDED.max_items,
			params = EXCLUDED.params`,
		cfg.ProfileID, string(cfg.FeedType), cfg.Enabled, cfg.DisplayOrder, cfg.MaxItems, paramsJSON,
	)
	return err
}

func (r *FeedProfileConfigRepository) Delete(ctx context.Context, profileID string, feedType feed.Type) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM feed_profile_config WHERE profile_id = $1 AND feed_type = $2`,
		profileID, string(feedType))
	return err
}

func (r *FeedProfileConfigRepository) DeleteByType(ctx context.Context, feedType feed.Type) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM feed_profile_config WHERE feed_type = $1`, string(feedType))
	return err
}

var _ feed.AdminConfigRepository = (*FeedAdminConfigRepository)(nil)
var _ feed.ProfileConfigRepository = (*FeedProfileConfigRepository)(nil)
