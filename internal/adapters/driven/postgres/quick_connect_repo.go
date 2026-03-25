package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type QuickConnectRepository struct {
	db *sql.DB
}

func NewQuickConnectRepository(db *sql.DB) *QuickConnectRepository {
	return &QuickConnectRepository{db: db}
}

func (r *QuickConnectRepository) Create(ctx context.Context, req *user.QuickConnectRequest) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO quick_connect_requests (id, code, secret, user_id, device_name, status, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		req.ID, req.Code, req.Secret, req.UserID, req.DeviceName, req.Status, req.ExpiresAt, req.CreatedAt,
	)
	return err
}

func (r *QuickConnectRepository) GetByCode(ctx context.Context, code string) (*user.QuickConnectRequest, error) {
	req := &user.QuickConnectRequest{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, code, secret, user_id, device_name, status, expires_at, created_at
		 FROM quick_connect_requests WHERE code = $1`, code,
	).Scan(&req.ID, &req.Code, &req.Secret, &req.UserID, &req.DeviceName, &req.Status, &req.ExpiresAt, &req.CreatedAt)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (r *QuickConnectRepository) GetByShortCode(ctx context.Context, shortCode string) (*user.QuickConnectRequest, error) {
	req := &user.QuickConnectRequest{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, code, secret, user_id, device_name, status, expires_at, created_at
		 FROM quick_connect_requests
		 WHERE UPPER(RIGHT(code, 6)) = UPPER($1) AND status = 'pending'
		 ORDER BY created_at DESC LIMIT 1`, shortCode,
	).Scan(&req.ID, &req.Code, &req.Secret, &req.UserID, &req.DeviceName, &req.Status, &req.ExpiresAt, &req.CreatedAt)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (r *QuickConnectRepository) UpdateStatus(ctx context.Context, id, status string, userID *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE quick_connect_requests SET status = $1, user_id = $2 WHERE id = $3`,
		status, userID, id)
	return err
}

func (r *QuickConnectRepository) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM quick_connect_requests WHERE expires_at < NOW()`)
	return err
}

var _ user.QuickConnectRepository = (*QuickConnectRepository)(nil)
