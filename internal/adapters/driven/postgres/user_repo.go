package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// userInsertColumns is the single source of truth for the columns Create writes
// (must match the $N VALUES count). userColumns appends last_login_at, which is
// read on SELECT but NULL at creation (only set on login), so it must NOT be in
// the INSERT list. Derive rather than duplicate: adding a base column then means
// touching one literal, not two.
const userInsertColumns = `id, username, email, password_hash, is_admin, enabled,
	 oidc_subject, oidc_issuer, max_streams, max_profiles, max_devices_per_profile,
	 max_bitrate_kbps, push_consent, created_at, updated_at`

const userColumns = userInsertColumns + `, last_login_at`

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
		`INSERT INTO users (`+userInsertColumns+`)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		u.ID, u.Username, u.Email, u.PasswordHash, u.IsAdmin, u.Enabled,
		u.OIDCSubject, u.OIDCIssuer, u.MaxStreams, u.MaxProfiles, u.MaxDevicesPerProfile,
		u.MaxBitrateKbps, u.PushConsent, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		// A unique-constraint violation (duplicate username, or the partial
		// idx_users_oidc_identity on a concurrent OIDC first-login) surfaces as
		// shared.ErrConflict so callers can recover instead of treating it as an
		// opaque failure (mirrors RefreshTokenRepo.Create).
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			conflictErr := fmt.Errorf("%w: %s", shared.ErrConflict, pqErr.Message)
			span.RecordError(conflictErr)
			span.SetStatus(codes.Error, conflictErr.Error())
			return conflictErr
		}
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
	u, err := r.scanUser(txFromCtx(ctx, r.db).QueryRowContext(ctx,
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
	// Case-insensitive on purpose: a viewer types "alice" for "Alice"
	// and expects to get in (prod, 2026-09). Uniqueness under lower() is held
	// by idx_users_username_lower (migration 000169), so one row at most.
	u, err := r.scanUser(txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(username) = lower($1)`, username))
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
	u, err := r.scanUser(txFromCtx(ctx, r.db).QueryRowContext(ctx,
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
	u, err := r.scanUser(txFromCtx(ctx, r.db).QueryRowContext(ctx,
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
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
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
			&u.MaxBitrateKbps, &u.PushConsent, &u.CreatedAt, &u.UpdatedAt, &u.LastLogin); err != nil {
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
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
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

// LinkOIDCIdentity writes the two identity columns only, and only while they
// are empty: a targeted write, unlike Update, cannot replay a stale row.
func (r *UserRepository) LinkOIDCIdentity(ctx context.Context, id shared.UserID, issuer, subject string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.LinkOIDCIdentity")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE users SET oidc_issuer = $2, oidc_subject = $3, updated_at = NOW()
		 WHERE id = $1 AND oidc_subject IS NULL`, string(id), issuer, subject)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			err = fmt.Errorf("%w: %s", shared.ErrConflict, pqErr.Message)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("UserRepo.LinkOIDCIdentity: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, getErr := r.GetByID(ctx, id); getErr != nil {
			return getErr
		}
		return fmt.Errorf("UserRepo.LinkOIDCIdentity: %w: account already linked", shared.ErrConflict)
	}
	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM users WHERE id = $1`, string(id))
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

// lastAdminGuardKey serialises every guarded admin removal/disable behind one
// tx-scoped advisory lock: two concurrent removals of distinct admins would
// otherwise each see a stale count > 1 and together leave zero enabled admins
// (a total lockout, repairable only via psql).
const lastAdminGuardKey = "evey:last-admin-guard"

// DeleteIfNotLastAdmin atomically refuses to delete the last enabled admin,
// then deletes. See the UserRepository port for the contract.
func (r *UserRepository) DeleteIfNotLastAdmin(ctx context.Context, id shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.DeleteIfNotLastAdmin")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(id)))
	return r.withLastAdminGuard(ctx, span, id, `DELETE FROM users WHERE id = $1`)
}

// DisableIfNotLastAdmin atomically refuses to disable the last enabled admin,
// then disables. See the UserRepository port for the contract.
func (r *UserRepository) DisableIfNotLastAdmin(ctx context.Context, id shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.DisableIfNotLastAdmin")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(id)))
	return r.withLastAdminGuard(ctx, span, id,
		`UPDATE users SET enabled = FALSE, updated_at = NOW() WHERE id = $1`)
}

func (r *UserRepository) withLastAdminGuard(ctx context.Context, span trace.Span, id shared.UserID, mutation string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("last-admin guard: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				slog.Warn("last-admin guard rollback failed", slog.Any("err", rbErr))
			}
		}
	}()

	// Held until commit/rollback - serialises every guarded admin mutation.
	if _, lockErr := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lastAdminGuardKey); lockErr != nil {
		span.RecordError(lockErr)
		return fmt.Errorf("last-admin guard: lock: %w", lockErr)
	}

	var isAdmin, enabled bool
	err = tx.QueryRowContext(ctx, `SELECT is_admin, enabled FROM users WHERE id = $1`, string(id)).
		Scan(&isAdmin, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("last-admin guard: lookup: %w", err)
	}
	// Only removing an ENABLED admin can lower the enabled-admin count; a
	// disabled admin is freely removable whatever the count.
	if isAdmin && enabled {
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE is_admin = TRUE AND enabled = TRUE`).Scan(&n); err != nil {
			span.RecordError(err)
			return fmt.Errorf("last-admin guard: count: %w", err)
		}
		if n <= 1 {
			return user.ErrLastEnabledAdmin
		}
	}
	if _, err := tx.ExecContext(ctx, mutation, string(id)); err != nil {
		span.RecordError(err)
		return fmt.Errorf("last-admin guard: mutate: %w", err)
	}
	if err := tx.Commit(); err != nil {
		span.RecordError(err)
		return fmt.Errorf("last-admin guard: commit: %w", err)
	}
	committed = true
	return nil
}

func (r *UserRepository) CountAdmins(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.CountAdmins")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var count int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = TRUE`).Scan(&count)
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
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = TRUE AND enabled = TRUE`).Scan(&count)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("count enabled admins: %w", err)
	}
	return count, nil
}

// GetTokenEpoch returns the user's current access-token revocation counter.
// Kept out of userColumns/Update on purpose: token_epoch is only ever mutated
// through the atomic BumpTokenEpoch, never overwritten by a stale struct.
func (r *UserRepository) GetTokenEpoch(ctx context.Context, id shared.UserID) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.GetTokenEpoch")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var epoch int64
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `SELECT token_epoch FROM users WHERE id = $1`, string(id)).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, fmt.Errorf("get token epoch: %w", err)
	}
	return epoch, nil
}

// BumpTokenEpoch atomically increments the revocation counter, invalidating
// every access token minted with the previous value.
func (r *UserRepository) BumpTokenEpoch(ctx context.Context, id shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.BumpTokenEpoch")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", string(id)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE users SET token_epoch = token_epoch + 1 WHERE id = $1`, string(id))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("bump token epoch: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *UserRepository) scanUser(row *sql.Row) (*user.User, error) {
	u := &user.User{}
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsAdmin, &u.Enabled,
		&u.OIDCSubject, &u.OIDCIssuer, &u.MaxStreams, &u.MaxProfiles, &u.MaxDevicesPerProfile,
		&u.MaxBitrateKbps, &u.PushConsent, &u.CreatedAt, &u.UpdatedAt, &u.LastLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("UserRepo.scanUser: %w", err)
	}
	return u, nil
}

func (r *UserRepository) TouchLastLogin(ctx context.Context, id shared.UserID, at time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.user.TouchLastLogin")
	defer span.End()
	// The only one of this file's 17 exported methods that had no bound, and it
	// sits on the login path - the most travelled request on the server. The
	// file-level guard could not see it: it checks that a FILE mentions the
	// helper, and this one mentions it sixteen times elsewhere.
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE users SET last_login_at = $2 WHERE id = $1`, string(id), at)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("UserRepo.TouchLastLogin: %w", err)
	}
	return nil
}

var _ user.UserRepository = (*UserRepository)(nil)
