package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO library_access (user_id, library_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		string(userID), string(libraryID))
	return err
}

// GrantMany inserts N grants for the same user in a single round-trip via
// unnest on a text[]. Used by invite registration to avoid N serial INSERTs
// when a library-scoped invite spans multiple libraries.
//
// Binds to an ambient *sql.Tx via txFromCtx so RegisterWithInvite can roll
// back grants when the surrounding tx fails — otherwise a partial registration
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
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO library_access (user_id, library_id)
		 SELECT $1, unnest($2::text[])
		 ON CONFLICT DO NOTHING`,
		string(userID), pq.Array(ids))
	return err
}

func (r *LibraryAccessRepository) Revoke(ctx context.Context, userID shared.UserID, libraryID shared.LibraryID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.Revoke")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM library_access WHERE user_id = $1 AND library_id = $2`,
		string(userID), string(libraryID))
	return err
}

func (r *LibraryAccessRepository) GetByUserID(ctx context.Context, userID shared.UserID) ([]shared.LibraryID, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.GetByUserID")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
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
	// Single round-trip: COUNT(*) = total restrictions, matched = whether the
	// requested library is in the user's set. Convention: zero restrictions =
	// full access.
	var total, matched int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE library_id = $2)
		 FROM library_access WHERE user_id = $1`,
		string(userID), string(libraryID)).Scan(&total, &matched)
	if err != nil {
		return false, err
	}
	if total == 0 {
		return true, nil // no restrictions = full access
	}
	return matched > 0, nil
}

func (r *LibraryAccessRepository) ListUsersWithRestrictions(ctx context.Context) (map[shared.UserID][]shared.LibraryID, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.library_access.ListUsersWithRestrictions")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
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
