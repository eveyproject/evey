package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/activity"
)

type LoginHistoryRepository struct {
	db *sql.DB
}

func NewLoginHistoryRepository(db *sql.DB) *LoginHistoryRepository {
	return &LoginHistoryRepository{db: db}
}

func (r *LoginHistoryRepository) Create(ctx context.Context, e *activity.LoginEntry) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO login_history (id, user_id, client_ip, user_agent, auth_method, success, failure_reason, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.ID, e.UserID, e.ClientIP, e.UserAgent, e.AuthMethod, e.Success, e.FailureReason, e.CreatedAt,
	)
	return err
}

func (r *LoginHistoryRepository) GetByUserID(ctx context.Context, userID string, limit, offset int) ([]*activity.LoginEntry, error) {
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

var _ activity.LoginHistoryRepository = (*LoginHistoryRepository)(nil)
