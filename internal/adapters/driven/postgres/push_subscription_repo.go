package postgres

import (
	"context"
	"database/sql"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type PushSubscriptionRepository struct {
	db *sql.DB
}

func NewPushSubscriptionRepository(db *sql.DB) *PushSubscriptionRepository {
	return &PushSubscriptionRepository{db: db}
}

func (r *PushSubscriptionRepository) Upsert(ctx context.Context, userID shared.UserID, profileID shared.ProfileID, endpoint, p256dh, auth string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.push_subscription.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)), attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO push_subscriptions (user_id, profile_id, endpoint, p256dh, auth, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (endpoint) DO UPDATE
		SET user_id = EXCLUDED.user_id,
		    profile_id = EXCLUDED.profile_id,
		    p256dh = EXCLUDED.p256dh,
		    auth = EXCLUDED.auth,
		    updated_at = NOW()
	`, string(userID), string(profileID), endpoint, p256dh, auth)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PushSubscriptionRepository) DeleteByEndpoint(ctx context.Context, userID shared.UserID, endpoint string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.push_subscription.DeleteByEndpoint")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1 AND user_id = $2`, endpoint, string(userID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PushSubscriptionRepository) DeleteByUser(ctx context.Context, userID shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.push_subscription.DeleteByUser")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1`, string(userID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *PushSubscriptionRepository) GetByProfile(ctx context.Context, profileID shared.ProfileID) ([]notification.PushSubscription, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.push_subscription.GetByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, user_id, profile_id, endpoint, p256dh, auth, created_at, updated_at
		FROM push_subscriptions WHERE profile_id = $1
		ORDER BY created_at DESC
	`, string(profileID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanSubscriptions(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *PushSubscriptionRepository) ListByUser(ctx context.Context, userID shared.UserID) ([]notification.PushSubscription, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.push_subscription.ListByUser")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(userID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, user_id, profile_id, endpoint, p256dh, auth, created_at, updated_at
		FROM push_subscriptions WHERE user_id = $1
		ORDER BY created_at DESC
	`, string(userID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanSubscriptions(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
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

var _ notification.PushSubscriptionRepository = (*PushSubscriptionRepository)(nil)
