package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/gravity-zero/evey/internal/domain/notification"
)

type NotificationRepository struct {
	db *sql.DB
}

func NewNotificationRepository(db *sql.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(ctx context.Context, n *notification.Notification) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (id, profile_id, type, title, message, media_item_id, level, created_at, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		n.ID, n.ProfileID, string(n.Type), n.Title, n.Message, n.MediaItemID,
		string(n.Level), n.CreatedAt, n.ExpiresAt,
	)
	return err
}

func (r *NotificationRepository) ListByProfile(ctx context.Context, profileID string, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	q := `SELECT id, profile_id, type, title, message, media_item_id, level, created_at, expires_at, delivered_at, read_at
	      FROM notifications WHERE profile_id = $1`
	if unreadOnly {
		q += ` AND read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`
	if limit > 0 {
		q += ` LIMIT $2 OFFSET $3`
		rows, err := r.db.QueryContext(ctx, q, profileID, limit, offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanNotifications(rows)
	}
	rows, err := r.db.QueryContext(ctx, q, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotifications(rows)
}

func scanNotifications(rows *sql.Rows) ([]*notification.Notification, error) {
	var result []*notification.Notification
	for rows.Next() {
		n := &notification.Notification{}
		var typ, lvl string
		if err := rows.Scan(&n.ID, &n.ProfileID, &typ, &n.Title, &n.Message, &n.MediaItemID, &lvl,
			&n.CreatedAt, &n.ExpiresAt, &n.DeliveredAt, &n.ReadAt); err != nil {
			return nil, err
		}
		n.Type = notification.Type(typ)
		n.Level = notification.Level(lvl)
		result = append(result, n)
	}
	return result, rows.Err()
}

func (r *NotificationRepository) MarkRead(ctx context.Context, id string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `UPDATE notifications SET read_at = $1 WHERE id = $2 AND read_at IS NULL`, now, id)
	return err
}

func (r *NotificationRepository) MarkAllRead(ctx context.Context, profileID string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `UPDATE notifications SET read_at = $1 WHERE profile_id = $2 AND read_at IS NULL`, now, profileID)
	return err
}

func (r *NotificationRepository) UnreadCount(ctx context.Context, profileID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE profile_id = $1 AND read_at IS NULL`, profileID,
	).Scan(&count)
	return count, err
}

func (r *NotificationRepository) DeleteAll(ctx context.Context, profileID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM notifications WHERE profile_id = $1`, profileID)
	return err
}

func (r *NotificationRepository) CleanupExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM notifications WHERE expires_at IS NOT NULL AND expires_at < NOW()`)
	return err
}
