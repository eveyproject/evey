package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
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
		`INSERT INTO notifications (id, profile_id, type, title, subject, message, media_item_id, related_media_id, season_count, episode_count, level, created_at, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		n.ID, n.ProfileID, string(n.Type), n.Title, n.Subject, n.Message, n.MediaItemID,
		relatedMediaID, n.SeasonCount, n.EpisodeCount,
		string(n.Level), n.CreatedAt, n.ExpiresAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *NotificationRepository) ListByProfile(ctx context.Context, profileID shared.ProfileID, unreadOnly bool, limit, offset int) ([]*notification.Notification, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.ListByProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	q := `SELECT id, profile_id, type, title, subject, message, media_item_id, related_media_id, season_count, episode_count, level, created_at, expires_at, read_at
	      FROM notifications WHERE profile_id = $1`
	if unreadOnly {
		q += ` AND read_at IS NULL`
	}
	q += ` ORDER BY created_at DESC, id DESC`
	if limit <= 0 {
		limit = 200
	}
	q += ` LIMIT $2 OFFSET $3`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, q, string(profileID), limit, offset)
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

// FindRecentByProfilesTypeAndSeries is the batched form used during episode
// notification fan-out: one query for every profile in the fan-out instead of
// one query per profile, which was the O(profiles) round-trip on library scans
// with many series x many profiles.
func (r *NotificationRepository) FindRecentByProfilesTypeAndSeries(ctx context.Context, profileIDs []shared.ProfileID, types []notification.Type, relatedMediaID string, since time.Time) (map[shared.ProfileID][]*notification.Notification, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.FindRecentByProfilesTypeAndSeries")
	defer span.End()
	span.SetAttributes(attribute.Int("profile.count", len(profileIDs)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	if len(profileIDs) == 0 {
		return map[shared.ProfileID][]*notification.Notification{}, nil
	}

	strProfileIDs := make([]string, len(profileIDs))
	for i, id := range profileIDs {
		strProfileIDs[i] = string(id)
	}

	placeholders := make([]string, len(types))
	args := make([]interface{}, 0, len(types)+3)
	args = append(args, pq.Array(strProfileIDs), relatedMediaID, since)
	for i, t := range types {
		placeholders[i] = fmt.Sprintf("$%d", i+4)
		args = append(args, string(t))
	}
	q := `SELECT id, profile_id, type, title, subject, message, media_item_id, related_media_id, season_count, episode_count, level, created_at, expires_at, read_at
	      FROM notifications
	      WHERE profile_id = ANY($1)
	        AND related_media_id = $2
	        AND created_at >= $3
	        AND type IN (` + strings.Join(placeholders, ",") + `)
	      ORDER BY created_at DESC, id DESC`

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
	byProfile := make(map[shared.ProfileID][]*notification.Notification, len(profileIDs))
	for _, n := range result {
		byProfile[n.ProfileID] = append(byProfile[n.ProfileID], n)
	}
	return byProfile, nil
}

func (r *NotificationRepository) Update(ctx context.Context, n *notification.Notification) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.Update")
	defer span.End()
	span.SetAttributes(attribute.String("notification.id", n.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE notifications SET title = $1, subject = $2, season_count = $3, episode_count = $4 WHERE id = $5 AND profile_id = $6`,
		n.Title, n.Subject, n.SeasonCount, n.EpisodeCount, n.ID, n.ProfileID,
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
		if err := rows.Scan(&n.ID, &n.ProfileID, &typ, &n.Title, &n.Subject, &n.Message, &n.MediaItemID,
			&relatedMediaID, &n.SeasonCount, &n.EpisodeCount, &lvl,
			&n.CreatedAt, &n.ExpiresAt, &n.ReadAt); err != nil {
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

func (r *NotificationRepository) MarkRead(ctx context.Context, id string, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.MarkRead")
	defer span.End()
	span.SetAttributes(attribute.String("notification.id", id), attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	now := time.Now()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE notifications SET read_at = $1 WHERE id = $2 AND profile_id = $3 AND read_at IS NULL`, now, id, string(profileID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *NotificationRepository) MarkAllRead(ctx context.Context, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.MarkAllRead")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	now := time.Now()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `UPDATE notifications SET read_at = $1 WHERE profile_id = $2 AND read_at IS NULL`, now, string(profileID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *NotificationRepository) UnreadCount(ctx context.Context, profileID shared.ProfileID) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.UnreadCount")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE profile_id = $1 AND read_at IS NULL`, string(profileID),
	).Scan(&count)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return count, nil
}

func (r *NotificationRepository) UnreadCountExcludingTypes(ctx context.Context, profileID shared.ProfileID, excludeTypes []notification.Type) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.UnreadCountExcludingTypes")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	query := `SELECT COUNT(*) FROM notifications WHERE profile_id = $1 AND read_at IS NULL`
	args := make([]any, 0, len(excludeTypes)+1)
	args = append(args, string(profileID))
	if len(excludeTypes) > 0 {
		placeholders := make([]string, len(excludeTypes))
		for i, t := range excludeTypes {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, string(t))
		}
		query += ` AND type NOT IN (` + strings.Join(placeholders, ",") + `)`
	}

	var count int
	if err := txFromCtx(ctx, r.db).QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return count, nil
}

func (r *NotificationRepository) DeleteAll(ctx context.Context, profileID shared.ProfileID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.notification.DeleteAll")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", string(profileID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM notifications WHERE profile_id = $1`, string(profileID))
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
