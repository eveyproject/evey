package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type AuditLogRepository struct {
	db *sql.DB
}

func NewAuditLogRepository(db *sql.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Create(ctx context.Context, e *activity.AuditLogEntry) error {
	detailsJSON, err := json.Marshal(e.Details)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO audit_log (id, user_id, action, resource_type, resource_id, details, client_ip, user_agent, device_name, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		e.ID, e.UserID, e.Action, e.ResourceType, e.ResourceID, detailsJSON, e.ClientIP, e.UserAgent, e.DeviceName, e.CreatedAt,
	)
	return err
}

func (r *AuditLogRepository) List(ctx context.Context, f activity.AuditLogFilter) ([]*activity.AuditLogEntry, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if f.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, *f.UserID)
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

	query := `SELECT id, user_id, action, resource_type, resource_id, details, client_ip, user_agent, device_name, created_at FROM audit_log`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC"

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*activity.AuditLogEntry
	for rows.Next() {
		e := &activity.AuditLogEntry{}
		var detailsJSON []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.ResourceType, &e.ResourceID, &detailsJSON, &e.ClientIP, &e.UserAgent, &e.DeviceName, &e.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(detailsJSON, &e.Details)
		result = append(result, e)
	}
	return result, rows.Err()
}
