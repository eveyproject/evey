package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

type LibraryRepository struct {
	db *sql.DB
}

func NewLibraryRepository(db *sql.DB) *LibraryRepository {
	return &LibraryRepository{db: db}
}

func (r *LibraryRepository) Create(ctx context.Context, lib *library.Library) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	pathsJSON, err := json.Marshal(lib.Paths)
	if err != nil {
		return fmt.Errorf("LibraryRepo.Create marshal paths: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO libraries (id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		lib.ID, lib.Name, lib.Type, pathsJSON, lib.ScanIntervalMinutes, lib.Enabled, lib.LastScannedAt, lib.CreatedAt, lib.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("LibraryRepo.Create: %w", err)
	}
	return nil
}

func (r *LibraryRepository) GetByID(ctx context.Context, id shared.LibraryID) (*library.Library, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	lib := &library.Library{}
	var pathsJSON []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at
		 FROM libraries WHERE id = $1`, string(id),
	).Scan(&lib.ID, &lib.Name, &lib.Type, &pathsJSON, &lib.ScanIntervalMinutes, &lib.Enabled, &lib.LastScannedAt, &lib.CreatedAt, &lib.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("LibraryRepo.GetByID: %w", err)
	}
	if err := json.Unmarshal(pathsJSON, &lib.Paths); err != nil {
		return nil, fmt.Errorf("LibraryRepo.GetByID unmarshal paths: %w", err)
	}
	return lib, nil
}

func (r *LibraryRepository) List(ctx context.Context, filter library.ListLibrariesFilter) ([]*library.Library, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	const baseQuery = `SELECT id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at
		 FROM libraries ORDER BY name`
	var (
		rows *sql.Rows
		err  error
	)
	if filter.Limit <= 0 {
		rows, err = r.db.QueryContext(ctx, baseQuery)
	} else {
		limit := filter.Limit
		if limit > library.MaxListLibrariesLimit {
			limit = library.MaxListLibrariesLimit
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}
		rows, err = r.db.QueryContext(ctx, baseQuery+` LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("LibraryRepo.List: %w", err)
	}
	defer rows.Close()

	var result []*library.Library
	for rows.Next() {
		lib := &library.Library{}
		var pathsJSON []byte
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Type, &pathsJSON, &lib.ScanIntervalMinutes, &lib.Enabled, &lib.LastScannedAt, &lib.CreatedAt, &lib.UpdatedAt); err != nil {
			return nil, fmt.Errorf("LibraryRepo.List scan: %w", err)
		}
		if err := json.Unmarshal(pathsJSON, &lib.Paths); err != nil {
			return nil, fmt.Errorf("LibraryRepo.List unmarshal paths: %w", err)
		}
		result = append(result, lib)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("LibraryRepo.List rows: %w", err)
	}
	return result, nil
}

func (r *LibraryRepository) Update(ctx context.Context, lib *library.Library) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	pathsJSON, err := json.Marshal(lib.Paths)
	if err != nil {
		return fmt.Errorf("LibraryRepo.Update marshal paths: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE libraries SET name=$1, type=$2, paths=$3, scan_interval_minutes=$4, enabled=$5, last_scanned_at=$6, updated_at=$7
		 WHERE id=$8`,
		lib.Name, lib.Type, pathsJSON, lib.ScanIntervalMinutes, lib.Enabled, lib.LastScannedAt, lib.UpdatedAt, lib.ID,
	)
	if err != nil {
		return fmt.Errorf("LibraryRepo.Update: %w", err)
	}
	return nil
}

func (r *LibraryRepository) Delete(ctx context.Context, id shared.LibraryID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM libraries WHERE id = $1`, string(id))
	if err != nil {
		return fmt.Errorf("LibraryRepo.Delete: %w", err)
	}
	return nil
}

var _ library.LibraryRepository = (*LibraryRepository)(nil)
