package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type LibraryRepository struct {
	db *sql.DB
}

func NewLibraryRepository(db *sql.DB) *LibraryRepository {
	return &LibraryRepository{db: db}
}

func (r *LibraryRepository) Create(ctx context.Context, lib *library.Library) error {
	pathsJSON, err := json.Marshal(lib.Paths)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO libraries (id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		lib.ID, lib.Name, lib.Type, pathsJSON, lib.ScanIntervalMinutes, lib.Enabled, lib.LastScannedAt, lib.CreatedAt, lib.UpdatedAt,
	)
	return err
}

func (r *LibraryRepository) GetByID(ctx context.Context, id string) (*library.Library, error) {
	lib := &library.Library{}
	var pathsJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at
		 FROM libraries WHERE id = $1`, id,
	).Scan(&lib.ID, &lib.Name, &lib.Type, &pathsJSON, &lib.ScanIntervalMinutes, &lib.Enabled, &lib.LastScannedAt, &lib.CreatedAt, &lib.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(pathsJSON, &lib.Paths); err != nil {
		return nil, err
	}
	return lib, nil
}

func (r *LibraryRepository) List(ctx context.Context) ([]*library.Library, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at
		 FROM libraries ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.Library
	for rows.Next() {
		lib := &library.Library{}
		var pathsJSON []byte
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Type, &pathsJSON, &lib.ScanIntervalMinutes, &lib.Enabled, &lib.LastScannedAt, &lib.CreatedAt, &lib.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(pathsJSON, &lib.Paths); err != nil {
			return nil, err
		}
		result = append(result, lib)
	}
	return result, rows.Err()
}

func (r *LibraryRepository) Update(ctx context.Context, lib *library.Library) error {
	pathsJSON, err := json.Marshal(lib.Paths)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE libraries SET name=$1, type=$2, paths=$3, scan_interval_minutes=$4, enabled=$5, last_scanned_at=$6, updated_at=$7
		 WHERE id=$8`,
		lib.Name, lib.Type, pathsJSON, lib.ScanIntervalMinutes, lib.Enabled, lib.LastScannedAt, lib.UpdatedAt, lib.ID,
	)
	return err
}

func (r *LibraryRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM libraries WHERE id = $1`, id)
	return err
}

var _ library.LibraryRepository = (*LibraryRepository)(nil)
