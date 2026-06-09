package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const userColumns = `id, username, email, password_hash, is_admin, enabled,
	 oidc_subject, oidc_issuer, max_streams, max_profiles, max_devices_per_profile,
	 max_bitrate_kbps, push_consent, created_at, updated_at`

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.Create")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(u.ID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO users (`+userColumns+`)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		u.ID, u.Username, u.Email, u.PasswordHash, u.IsAdmin, u.Enabled,
		u.OIDCSubject, u.OIDCIssuer, u.MaxStreams, u.MaxProfiles, u.MaxDevicesPerProfile,
		u.MaxBitrateKbps, u.PushConsent, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id shared.UserID) (*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	u, err := r.scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, string(id)))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.GetByUsername")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	u, err := r.scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE username = $1`, username))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.GetByEmail")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	u, err := r.scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1`, email))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetByOIDCSubject(ctx context.Context, issuer, subject string) (*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.GetByOIDCSubject")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	u, err := r.scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE oidc_issuer = $1 AND oidc_subject = $2`, issuer, subject))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) List(ctx context.Context) ([]*user.User, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userColumns+` FROM users ORDER BY username LIMIT 1000`)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("UserRepo.List: %w", err)
	}
	defer rows.Close()

	var result []*user.User
	for rows.Next() {
		u := &user.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.Enabled,
			&u.OIDCSubject, &u.OIDCIssuer, &u.MaxStreams, &u.MaxProfiles, &u.MaxDevicesPerProfile,
			&u.MaxBitrateKbps, &u.PushConsent, &u.CreatedAt, &u.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("UserRepo.List scan: %w", err)
		}
		result = append(result, u)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("UserRepo.List rows: %w", err)
	}
	return result, nil
}

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.Update")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(u.ID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET username=$1, email=$2, password_hash=$3, is_admin=$4, enabled=$5,
		 oidc_subject=$6, oidc_issuer=$7, max_streams=$8, max_profiles=$9, max_devices_per_profile=$10,
		 max_bitrate_kbps=$11, push_consent=$12, updated_at=$13 WHERE id=$14`,
		u.Username, u.Email, u.PasswordHash, u.IsAdmin, u.Enabled,
		u.OIDCSubject, u.OIDCIssuer, u.MaxStreams, u.MaxProfiles, u.MaxDevicesPerProfile,
		u.MaxBitrateKbps, u.PushConsent, u.UpdatedAt, u.ID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("update user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, string(id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("delete user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *UserRepository) CountAdmins(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.CountAdmins")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = TRUE`).Scan(&count)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return count, nil
}

func (r *UserRepository) CountEnabledAdmins(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.CountEnabledAdmins")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = TRUE AND enabled = TRUE`).Scan(&count)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("count enabled admins: %w", err)
	}
	return count, nil
}

func (r *UserRepository) scanUser(row *sql.Row) (*user.User, error) {
	u := &user.User{}
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.Enabled,
		&u.OIDCSubject, &u.OIDCIssuer, &u.MaxStreams, &u.MaxProfiles, &u.MaxDevicesPerProfile,
		&u.MaxBitrateKbps, &u.PushConsent, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("UserRepo.scanUser: %w", err)
	}
	return u, nil
}

var _ user.UserRepository = (*UserRepository)(nil)
