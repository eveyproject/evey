package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type AuditLogRepository struct {
	db *sql.DB
}

func NewAuditLogRepository(db *sql.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Create(ctx context.Context, e *activity.AuditLogEntry) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.audit_log.Create")
	defer span.End()
	span.SetAttributes(attribute.String("audit_log.id", e.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	detailsJSON, err := json.Marshal(e.Details)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	// profile_id is NULL rather than '' when absent (server-side actions, an
	// API key with no profile): a foreign-key-shaped column that is sometimes
	// an empty string makes every later JOIN and filter ambiguous.
	var profileID *string
	if e.ProfileID != "" {
		p := string(e.ProfileID)
		profileID = &p
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO audit_log (id, user_id, profile_id, action, resource_type, resource_id, details, client_ip, user_agent, device_name, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		e.ID, e.UserID, profileID, e.Action, e.ResourceType, e.ResourceID, detailsJSON, e.ClientIP, e.UserAgent, e.DeviceName, e.CreatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *AuditLogRepository) List(ctx context.Context, f activity.AuditLogFilter) ([]*activity.AuditLogEntry, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.audit_log.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var conditions []string
	var args []any
	argIdx := 1

	if f.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("a.user_id = $%d", argIdx))
		args = append(args, *f.UserID)
		argIdx++
	}
	if f.ProfileID != nil {
		conditions = append(conditions, fmt.Sprintf("a.profile_id = $%d", argIdx))
		args = append(args, *f.ProfileID)
		argIdx++
	}
	if f.Action != nil {
		conditions = append(conditions, fmt.Sprintf("a.action = $%d", argIdx))
		args = append(args, *f.Action)
		argIdx++
	}
	if f.ResourceType != nil {
		conditions = append(conditions, fmt.Sprintf("a.resource_type = $%d", argIdx))
		args = append(args, *f.ResourceType)
	}

	query := `SELECT a.id, a.user_id, COALESCE(u.username,'') AS username,
	           COALESCE(a.profile_id,'') AS profile_id, COALESCE(p.name,'') AS profile_name,
	           a.action, a.resource_type, a.resource_id,
	           COALESCE(
	             CASE a.resource_type
	               WHEN 'user' THEN ru.username
	               WHEN 'media' THEN CASE rm.type
	                 WHEN 'episode' THEN
	                   COALESCE(series.title, '') || ' - S' ||
	                   LPAD(COALESCE(season.index_number, 0)::text, 2, '0') || 'E' ||
	                   LPAD(COALESCE(rm.index_number, 0)::text, 2, '0')
	                 ELSE rm.title END
	               WHEN 'media_item' THEN CASE rm.type
	                 WHEN 'episode' THEN
	                   COALESCE(series.title, '') || ' - S' ||
	                   LPAD(COALESCE(season.index_number, 0)::text, 2, '0') || 'E' ||
	                   LPAD(COALESCE(rm.index_number, 0)::text, 2, '0')
	                 ELSE rm.title END
	               WHEN 'transcode_profile' THEN tp.name
	             END, '') AS resource_name,
	           a.details, a.client_ip, a.user_agent, a.device_name, a.created_at
	           FROM audit_log a
	           LEFT JOIN users u ON u.id = a.user_id
	           LEFT JOIN profiles p ON p.id = a.profile_id
	           LEFT JOIN users ru ON ru.id = a.resource_id AND a.resource_type = 'user'
	           LEFT JOIN media_items rm ON rm.id = a.resource_id AND a.resource_type IN ('media', 'media_item')
	           LEFT JOIN media_items season ON season.id = rm.parent_id AND rm.type = 'episode'
	           LEFT JOIN media_items series ON series.id = season.parent_id AND rm.type = 'episode'
	           LEFT JOIN transcode_profiles tp ON tp.id::text = a.resource_id AND a.resource_type = 'transcode_profile'`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY a.created_at DESC, a.id DESC"

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []*activity.AuditLogEntry
	for rows.Next() {
		e := &activity.AuditLogEntry{}
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.ProfileID, &e.ProfileName, &e.Action, &e.ResourceType, &e.ResourceID, &e.ResourceName, &detailsJSON, &e.ClientIP, &e.UserAgent, &e.DeviceName, &e.CreatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		if err := json.Unmarshal(detailsJSON, &e.Details); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "details"), slog.Any("err", err))
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// Count returns the total rows matching the filter predicates (no joins needed
// - resource_name resolution is a display concern of List only).
func (r *AuditLogRepository) Count(ctx context.Context, f activity.AuditLogFilter) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.audit_log.Count")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var conditions []string
	var args []any
	argIdx := 1
	if f.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, *f.UserID)
		argIdx++
	}
	if f.ProfileID != nil {
		conditions = append(conditions, fmt.Sprintf("profile_id = $%d", argIdx))
		args = append(args, *f.ProfileID)
		argIdx++
	}
	if f.Action != nil {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, *f.Action)
		argIdx++
	}
	if f.ResourceType != nil {
		conditions = append(conditions, fmt.Sprintf("resource_type = $%d", argIdx))
		args = append(args, *f.ResourceType)
	}
	query := "SELECT COUNT(*) FROM audit_log"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	var n int
	if err := txFromCtx(ctx, r.db).QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return n, nil
}

func (r *AuditLogRepository) Purge(ctx context.Context, before time.Time) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.audit_log.Purge")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM audit_log WHERE created_at < $1`, before)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

var _ activity.AuditLogRepository = (*AuditLogRepository)(nil)
