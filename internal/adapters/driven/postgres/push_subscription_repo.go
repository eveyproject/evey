package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/notification"
)

type PushSubscriptionRepository struct {
	db *sql.DB
}

func NewPushSubscriptionRepository(db *sql.DB) *PushSubscriptionRepository {
	return &PushSubscriptionRepository{db: db}
}

func (r *PushSubscriptionRepository) Upsert(ctx context.Context, userID, profileID, endpoint, p256dh, auth string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (user_id, profile_id, endpoint, p256dh, auth, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (endpoint) DO UPDATE
		SET user_id = EXCLUDED.user_id,
		    profile_id = EXCLUDED.profile_id,
		    p256dh = EXCLUDED.p256dh,
		    auth = EXCLUDED.auth,
		    updated_at = NOW()
	`, userID, profileID, endpoint, p256dh, auth)
	return err
}

func (r *PushSubscriptionRepository) DeleteByEndpoint(ctx context.Context, userID, endpoint string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1 AND user_id = $2`, endpoint, userID)
	return err
}

func (r *PushSubscriptionRepository) DeleteByUser(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1`, userID)
	return err
}

func (r *PushSubscriptionRepository) GetByProfile(ctx context.Context, profileID string) ([]notification.PushSubscription, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, profile_id, endpoint, p256dh, auth, created_at, updated_at
		FROM push_subscriptions WHERE profile_id = $1
	`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func (r *PushSubscriptionRepository) ListByUser(ctx context.Context, userID string) ([]notification.PushSubscription, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, profile_id, endpoint, p256dh, auth, created_at, updated_at
		FROM push_subscriptions WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func scanSubscriptions(rows *sql.Rows) ([]notification.PushSubscription, error) {
	var out []notification.PushSubscription
	for rows.Next() {
		var s notification.PushSubscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.ProfileID, &s.Endpoint, &s.P256dh, &s.Auth, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
