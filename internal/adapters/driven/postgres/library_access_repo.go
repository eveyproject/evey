package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type LibraryAccessRepository struct {
	db *sql.DB
}

func NewLibraryAccessRepository(db *sql.DB) *LibraryAccessRepository {
	return &LibraryAccessRepository{db: db}
}

func (r *LibraryAccessRepository) Grant(ctx context.Context, userID, libraryID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO library_access (user_id, library_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, libraryID)
	return err
}

func (r *LibraryAccessRepository) Revoke(ctx context.Context, userID, libraryID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM library_access WHERE user_id = $1 AND library_id = $2`,
		userID, libraryID)
	return err
}

func (r *LibraryAccessRepository) GetByUserID(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT library_id FROM library_access WHERE user_id = $1 ORDER BY granted_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (r *LibraryAccessRepository) HasAccess(ctx context.Context, userID, libraryID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM library_access WHERE user_id = $1`, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	if count == 0 {
		return true, nil // no restrictions = full access
	}
	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM library_access WHERE user_id = $1 AND library_id = $2`,
		userID, libraryID).Scan(&count)
	return count > 0, err
}

func (r *LibraryAccessRepository) ListUsersWithRestrictions(ctx context.Context) (map[string][]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, library_id FROM library_access ORDER BY user_id, granted_at LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]string)
	for rows.Next() {
		var userID, libID string
		if err := rows.Scan(&userID, &libID); err != nil {
			return nil, err
		}
		result[userID] = append(result[userID], libID)
	}
	return result, rows.Err()
}

var _ user.LibraryAccessRepository = (*LibraryAccessRepository)(nil)
