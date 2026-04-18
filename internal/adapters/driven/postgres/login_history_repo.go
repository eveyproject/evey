package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/shared"
)

type LoginHistoryRepository struct {
	db *sql.DB
}

func NewLoginHistoryRepository(db *sql.DB) *LoginHistoryRepository {
	return &LoginHistoryRepository{db: db}
}

func (r *LoginHistoryRepository) Create(ctx context.Context, e *activity.LoginEntry) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO login_history (id, user_id, client_ip, user_agent, auth_method, success, failure_reason, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.ID, e.UserID, e.ClientIP, e.UserAgent, e.AuthMethod, e.Success, e.FailureReason, e.CreatedAt,
	)
	return err
}

func (r *LoginHistoryRepository) GetByUserID(ctx context.Context, userID shared.UserID, limit, offset int) ([]*activity.LoginEntry, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, client_ip, user_agent, auth_method, success, failure_reason, created_at
		 FROM login_history WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*activity.LoginEntry
	for rows.Next() {
		e := &activity.LoginEntry{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.ClientIP, &e.UserAgent, &e.AuthMethod, &e.Success, &e.FailureReason, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *LoginHistoryRepository) ListByUserIDCursor(ctx context.Context, userID shared.UserID, successOnly bool, from, to *time.Time, cursor *activity.LoginHistoryCursor, limit int) ([]*activity.LoginEntry, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query := `SELECT id, user_id, client_ip, user_agent, auth_method, success, failure_reason, created_at
		 FROM login_history WHERE user_id = $1`
	args := []any{userID}
	i := 2
	if successOnly {
		query += " AND success = true"
	}
	if from != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", i)
		args = append(args, *from)
		i++
	}
	if to != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", i)
		args = append(args, *to)
		i++
	}
	if cursor != nil {
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", i, i+1)
		args = append(args, cursor.StartedAt, cursor.EntryID)
		i += 2
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", i)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*activity.LoginEntry
	for rows.Next() {
		e := &activity.LoginEntry{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.ClientIP, &e.UserAgent, &e.AuthMethod, &e.Success, &e.FailureReason, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

var _ activity.LoginHistoryRepository = (*LoginHistoryRepository)(nil)
