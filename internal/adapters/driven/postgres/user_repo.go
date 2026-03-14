package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/user"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, username, email, password_hash, is_admin, enabled, oidc_subject, oidc_issuer, max_streams, max_bitrate_kbps, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		u.ID, u.Username, u.Email, u.PasswordHash, u.IsAdmin, u.Enabled,
		u.OIDCSubject, u.OIDCIssuer, u.MaxStreams, u.MaxBitrateKbps, u.CreatedAt, u.UpdatedAt,
	)
	return err
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, is_admin, enabled, oidc_subject, oidc_issuer, max_streams, max_bitrate_kbps, created_at, updated_at
		 FROM users WHERE id = $1`, id))
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, is_admin, enabled, oidc_subject, oidc_issuer, max_streams, max_bitrate_kbps, created_at, updated_at
		 FROM users WHERE username = $1`, username))
}

func (r *UserRepository) GetByOIDCSubject(ctx context.Context, issuer, subject string) (*user.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx,
		`SELECT id, username, email, password_hash, is_admin, enabled, oidc_subject, oidc_issuer, max_streams, max_bitrate_kbps, created_at, updated_at
		 FROM users WHERE oidc_issuer = $1 AND oidc_subject = $2`, issuer, subject))
}

func (r *UserRepository) List(ctx context.Context) ([]*user.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, username, email, password_hash, is_admin, enabled, oidc_subject, oidc_issuer, max_streams, max_bitrate_kbps, created_at, updated_at
		 FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*user.User
	for rows.Next() {
		u := &user.User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.Enabled,
			&u.OIDCSubject, &u.OIDCIssuer, &u.MaxStreams, &u.MaxBitrateKbps, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET username=$1, email=$2, password_hash=$3, is_admin=$4, enabled=$5,
		 oidc_subject=$6, oidc_issuer=$7, max_streams=$8, max_bitrate_kbps=$9, updated_at=$10 WHERE id=$11`,
		u.Username, u.Email, u.PasswordHash, u.IsAdmin, u.Enabled,
		u.OIDCSubject, u.OIDCIssuer, u.MaxStreams, u.MaxBitrateKbps, u.UpdatedAt, u.ID,
	)
	return err
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

func (r *UserRepository) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = TRUE`).Scan(&count)
	return count, err
}

func (r *UserRepository) scanUser(row *sql.Row) (*user.User, error) {
	u := &user.User{}
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.Enabled,
		&u.OIDCSubject, &u.OIDCIssuer, &u.MaxStreams, &u.MaxBitrateKbps, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
