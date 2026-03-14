package postgres

import (
	"context"
	"database/sql"
)

type FCMDeviceTokenRepository struct {
	db *sql.DB
}

func NewFCMDeviceTokenRepository(db *sql.DB) *FCMDeviceTokenRepository {
	return &FCMDeviceTokenRepository{db: db}
}

func (r *FCMDeviceTokenRepository) Upsert(ctx context.Context, userID, profileID, token, platform string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO fcm_device_tokens (user_id, profile_id, token, platform, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (token) DO UPDATE
		SET user_id = EXCLUDED.user_id,
		    profile_id = EXCLUDED.profile_id,
		    platform = EXCLUDED.platform,
		    updated_at = NOW()
	`, userID, profileID, token, platform)
	return err
}

func (r *FCMDeviceTokenRepository) Delete(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM fcm_device_tokens WHERE token = $1`, token)
	return err
}

func (r *FCMDeviceTokenRepository) GetByProfile(ctx context.Context, profileID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT token FROM fcm_device_tokens WHERE profile_id = $1`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTokens(rows)
}

func (r *FCMDeviceTokenRepository) GetByUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT token FROM fcm_device_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTokens(rows)
}

func scanTokens(rows *sql.Rows) ([]string, error) {
	var tokens []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}
