package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type NotificationRepository struct {
	db *sql.DB
}

func NewNotificationRepository(db *sql.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(ctx context.Context, n *notification.Notification) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.Create")
	defer span.End()
	span.SetAttributes(attribute.String("notification.id", n.ID), attribute.String("profile.id", string(n.ProfileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	relatedMediaID := sql.NullString{String: string(n.RelatedMediaID), Valid: n.RelatedMediaID != ""}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO notifications (id, profile_id, type, title, message, media_item_id, related_media_id, episode_count, level, created_at, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		n.ID, n.ProfileID, string(n.Type), n.Title, n.Message, n.MediaItemID,
		relatedMediaID, n.EpisodeCount,
		string(n.Level), n.CreatedAt, n.ExpiresAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *NotificationRepository) ListByProfile(ctx context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.ListByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	q := `SELECT id, profile_id, type, title, message, media_item_id, related_media_id, episode_count, level, created_at, expires_at, delivered_at, read_at
	      FROM notifications WHERE profile_id = $1`
	if unreadOnly {
		q += ` AND read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`
	if limit <= 0 {
		limit = 200
	}
	q += ` LIMIT $2 OFFSET $3`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, profileID, limit, offset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanNotifications(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *NotificationRepository) FindRecentByProfileTypeAndSeries(ctx context.Context, profileID string, types []notification.Type, relatedMediaID string, since time.Time) ([]*notification.Notification, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.FindRecentByProfileTypeAndSeries")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	placeholders := make([]string, len(types))
	args := make([]interface{}, 0, len(types)+3)
	args = append(args, profileID, relatedMediaID, since)
	for i, t := range types {
		placeholders[i] = fmt.Sprintf("$%d", i+4)
		args = append(args, string(t))
	}
	q := `SELECT id, profile_id, type, title, message, media_item_id, related_media_id, episode_count, level, created_at, expires_at, delivered_at, read_at
	      FROM notifications
	      WHERE profile_id = $1
	        AND related_media_id = $2
	        AND created_at >= $3
	        AND type IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	result, err := scanNotifications(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *NotificationRepository) Update(ctx context.Context, n *notification.Notification) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.Update")
	defer span.End()
	span.SetAttributes(attribute.String("notification.id", n.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE notifications SET title = $1, episode_count = $2 WHERE id = $3 AND profile_id = $4`,
		n.Title, n.EpisodeCount, n.ID, n.ProfileID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func scanNotifications(rows *sql.Rows) ([]*notification.Notification, error) {
	var result []*notification.Notification
	for rows.Next() {
		n := &notification.Notification{}
		var typ, lvl string
		var relatedMediaID sql.NullString
		if err := rows.Scan(&n.ID, &n.ProfileID, &typ, &n.Title, &n.Message, &n.MediaItemID,
			&relatedMediaID, &n.EpisodeCount, &lvl,
			&n.CreatedAt, &n.ExpiresAt, &n.DeliveredAt, &n.ReadAt); err != nil {
			return nil, err
		}
		n.Type = notification.Type(typ)
		n.Level = notification.Level(lvl)
		if relatedMediaID.Valid {
			n.RelatedMediaID = shared.MediaItemID(relatedMediaID.String)
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

func (r *NotificationRepository) MarkRead(ctx context.Context, id, profileID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.MarkRead")
	defer span.End()
	span.SetAttributes(attribute.String("notification.id", id), attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	now := time.Now()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE notifications SET read_at = $1 WHERE id = $2 AND profile_id = $3 AND read_at IS NULL`, now, id, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *NotificationRepository) MarkAllRead(ctx context.Context, profileID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.MarkAllRead")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	now := time.Now()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE notifications SET read_at = $1 WHERE profile_id = $2 AND read_at IS NULL`, now, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *NotificationRepository) UnreadCount(ctx context.Context, profileID string) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.UnreadCount")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE profile_id = $1 AND read_at IS NULL`, profileID,
	).Scan(&count)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return count, nil
}

func (r *NotificationRepository) DeleteAll(ctx context.Context, profileID string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.DeleteAll")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", profileID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM notifications WHERE profile_id = $1`, profileID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *NotificationRepository) CleanupExpired(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.CleanupExpired")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM notifications WHERE expires_at IS NOT NULL AND expires_at < NOW()`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ notification.Repository = (*NotificationRepository)(nil)
