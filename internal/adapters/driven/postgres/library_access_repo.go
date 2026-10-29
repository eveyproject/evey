package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lib/pq"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type LibraryAccessRepository struct {
	db *sql.DB
}

func NewLibraryAccessRepository(db *sql.DB) *LibraryAccessRepository {
	return &LibraryAccessRepository{db: db}
}

func (r *LibraryAccessRepository) Grant(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.Grant")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`WITH flag AS (UPDATE users SET library_restricted = TRUE WHERE id = $1)
		 INSERT INTO library_access (user_id, library_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		string(userID), string(libraryID))
	return err
}

// GrantMany inserts N grants for the same user in a single round-trip via
// unnest on a text[]. Used by invite registration to avoid N serial INSERTs
// when a library-scoped invite spans multiple libraries.
//
// Binds to an ambient *sql.Tx via txFromCtx so RegisterWithInvite can roll
// back grants when the surrounding tx fails - otherwise a partial registration
// could leave orphan grants pointing at a never-created user.
func (r *LibraryAccessRepository) GrantMany(ctx context.Context, userID shared.UserID, libraryIDs []shared.LibraryID) error {
	if len(libraryIDs) == 0 {
		return nil
	}
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.GrantMany")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	ids := make([]string, len(libraryIDs))
	for i, id := range libraryIDs {
		ids[i] = string(id)
	}
	// Joined against libraries: invite links carry their library ids as JSONB
	// (no FK), so a library deleted after the invite was minted used to blow
	// the FK here and fail the WHOLE registration transaction. Stale ids are
	// skipped; surviving ones are granted. The user is restricted even when
	// none survives, so stale ids deny rather than widen.
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`WITH flag AS (UPDATE users SET library_restricted = TRUE WHERE id = $1)
		 INSERT INTO library_access (user_id, library_id)
		 SELECT $1, l.id FROM libraries l WHERE l.id = ANY($2::text[])
		 ON CONFLICT DO NOTHING`,
		string(userID), pq.Array(ids))
	return err
}

// ReplaceAccess atomically swaps every grant for userID for exactly
// libraryIDs: delete-then-insert in a single transaction, so a crash or an
// error mid-operation cannot leave the user revoked-from-everything with the
// new set never granted (the window a revoke-loop-then-grant-loop had).
// Opens its own tx rather than binding to an ambient one via txFromCtx: the
// two statements below must commit or roll back together regardless of what
// the caller is doing.
func (r *LibraryAccessRepository) ReplaceAccess(ctx context.Context, userID shared.UserID, libraryIDs []shared.LibraryID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.ReplaceAccess")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("LibraryAccessRepository.ReplaceAccess begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				slog.Warn("LibraryAccessRepository.ReplaceAccess rollback failed", slog.Any("err", rbErr))
			}
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM library_access WHERE user_id = $1`, string(userID)); err != nil {
		return fmt.Errorf("replace access: delete existing: %w", err)
	}
	// Confines the user to the set: an empty set is "no library". Only
	// DeleteByUserID lifts the restriction, so an admin who unticks every box
	// never hands out the whole catalogue by accident.
	if _, err = tx.ExecContext(ctx, `UPDATE users SET library_restricted = TRUE WHERE id = $1`, string(userID)); err != nil {
		return fmt.Errorf("replace access: flag: %w", err)
	}
	if len(libraryIDs) > 0 {
		ids := make([]string, len(libraryIDs))
		for i, id := range libraryIDs {
			ids[i] = string(id)
		}
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO library_access (user_id, library_id)
			 SELECT $1, unnest($2::text[])
			 ON CONFLICT DO NOTHING`,
			string(userID), pq.Array(ids)); err != nil {
			return fmt.Errorf("replace access: insert: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("replace access: commit: %w", err)
	}
	return nil
}

// DeleteByUserID atomically revokes every grant for userID and lifts the
// restriction in a single statement: it backs RevokeAllUserLibraryAccess,
// the admin's "unrestricted" action.
func (r *LibraryAccessRepository) DeleteByUserID(ctx context.Context, userID shared.UserID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.DeleteByUserID")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`WITH flag AS (UPDATE users SET library_restricted = FALSE WHERE id = $1)
		 DELETE FROM library_access WHERE user_id = $1`, string(userID))
	return err
}

func (r *LibraryAccessRepository) Revoke(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.Revoke")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`DELETE FROM library_access WHERE user_id = $1 AND library_id = $2`,
		string(userID), string(libraryID))
	return err
}

func (r *LibraryAccessRepository) GetByUserID(ctx context.Context, userID shared.UserID) ([]shared.LibraryID, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.GetByUserID")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT library_id FROM library_access WHERE user_id = $1 ORDER BY granted_at`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []shared.LibraryID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, shared.LibraryID(id))
	}
	return result, rows.Err()
}

func (r *LibraryAccessRepository) HasAccess(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.HasAccess")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Fails closed: an unknown user has no row and gets no access.
	var restricted, matched bool
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT u.library_restricted,
		        EXISTS (SELECT 1 FROM library_access a WHERE a.user_id = u.id AND a.library_id = $2)
		 FROM users u WHERE u.id = $1`,
		string(userID), string(libraryID)).Scan(&restricted, &matched)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !restricted || matched, nil
}

// IsRestricted reports whether userID is limited to its granted libraries,
// even when no grant is left.
func (r *LibraryAccessRepository) IsRestricted(ctx context.Context, userID shared.UserID) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.IsRestricted")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var restricted bool
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT library_restricted FROM users WHERE id = $1`, string(userID)).Scan(&restricted)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return restricted, err
}

func (r *LibraryAccessRepository) ListUsersWithRestrictions(ctx context.Context) (map[shared.UserID][]shared.LibraryID, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.ListUsersWithRestrictions")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT user_id, library_id FROM library_access ORDER BY user_id, granted_at LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[shared.UserID][]shared.LibraryID)
	for rows.Next() {
		var userID, libID string
		if err := rows.Scan(&userID, &libID); err != nil {
			return nil, err
		}
		uid := shared.UserID(userID)
		result[uid] = append(result[uid], shared.LibraryID(libID))
	}
	return result, rows.Err()
}

var _ user.LibraryAccessRepository = (*LibraryAccessRepository)(nil)
