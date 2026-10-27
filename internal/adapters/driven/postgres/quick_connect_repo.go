package postgres

import (
	"context"
	"database/sql"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type QuickConnectRepository struct {
	db *sql.DB
}

func NewQuickConnectRepository(db *sql.DB) *QuickConnectRepository {
	return &QuickConnectRepository{db: db}
}

func (r *QuickConnectRepository) Create(ctx context.Context, req *user.QuickConnectRequest) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.quick_connect.Create")
	defer span.End()
	span.SetAttributes(attribute.String("quick_connect.id", req.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO quick_connect_requests (id, code, secret, user_id, device_name, status, expires_at, created_at, persistent, secret_required)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		req.ID, req.Code, req.Secret, req.UserID, req.DeviceName, req.Status, req.ExpiresAt, req.CreatedAt, req.Persistent, req.SecretRequired,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (r *QuickConnectRepository) GetByCode(ctx context.Context, code string) (*user.QuickConnectRequest, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.quick_connect.GetByCode")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	req := &user.QuickConnectRequest{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, code, secret, user_id, device_name, status, expires_at, created_at, persistent, secret_required
		 FROM quick_connect_requests WHERE code = $1 AND expires_at > now()`, code,
	).Scan(&req.ID, &req.Code, &req.Secret, &req.UserID, &req.DeviceName, &req.Status, &req.ExpiresAt, &req.CreatedAt, &req.Persistent, &req.SecretRequired)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return req, nil
}

func (r *QuickConnectRepository) GetByShortCode(ctx context.Context, shortCode string) (*user.QuickConnectRequest, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.quick_connect.GetByShortCode")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	req := &user.QuickConnectRequest{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, code, secret, user_id, device_name, status, expires_at, created_at, persistent, secret_required
		 FROM quick_connect_requests
		 WHERE UPPER(RIGHT(code, 6)) = UPPER($1) AND status = 'pending' AND expires_at > now()
		 ORDER BY created_at DESC LIMIT 1`, shortCode,
	).Scan(&req.ID, &req.Code, &req.Secret, &req.UserID, &req.DeviceName, &req.Status, &req.ExpiresAt, &req.CreatedAt, &req.Persistent, &req.SecretRequired)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return req, nil
}

func (r *QuickConnectRepository) UpdateStatus(ctx context.Context, id, status string, userID *string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.quick_connect.UpdateStatus")
	defer span.End()
	span.SetAttributes(attribute.String("quick_connect.id", id), attribute.String("quick_connect.status", status))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Only transition from 'pending' - authorizing a non-pending request
	// (already used, expired, rejected) must be a no-op so a leaked-code
	// replay can't overwrite user_id on a settled authorization.
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE quick_connect_requests SET status = $1, user_id = $2 WHERE id = $3 AND status = 'pending'`,
		status, userID, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	// 0 rows = the request wasn't pending (already authorized by a concurrent
	// caller, expired, or a leaked-code replay). Returning nil here let the loser
	// of a race - and a replayer - believe they had authorized the device. Signal
	// not-found so the caller surfaces the failure instead of a false success.
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *QuickConnectRepository) Consume(ctx context.Context, code string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.quick_connect.Consume")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE quick_connect_requests SET status = 'consumed'
		 WHERE code = $1 AND status = 'authorized' AND expires_at > now()`, code)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, nErr := res.RowsAffected(); nErr != nil || n != 1 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *QuickConnectRepository) DeleteExpired(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.quick_connect.DeleteExpired")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM quick_connect_requests WHERE expires_at < NOW()`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

var _ user.QuickConnectRepository = (*QuickConnectRepository)(nil)
