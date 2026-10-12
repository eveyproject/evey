package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type LibraryRepository struct {
	db *sql.DB
}

func NewLibraryRepository(db *sql.DB) *LibraryRepository {
	return &LibraryRepository{db: db}
}

func (r *LibraryRepository) Create(ctx context.Context, lib *library.Library) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.library.Create")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	pathsJSON, err := json.Marshal(lib.Paths)
	if err != nil {
		return fmt.Errorf("LibraryRepo.Create marshal paths: %w", err)
	}
	_, err = txFromCtx(ctx, r.db).ExecContext(ctx,
		`INSERT INTO libraries (id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at, scan_on_startup, realtime_monitoring)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		lib.ID, lib.Name, lib.Type, pathsJSON, lib.ScanIntervalMinutes, lib.Enabled, lib.LastScannedAt, lib.CreatedAt, lib.UpdatedAt, lib.ScanOnStartup, lib.RealtimeMonitoring,
	)
	if err != nil {
		return fmt.Errorf("LibraryRepo.Create: %w", err)
	}
	return nil
}

func (r *LibraryRepository) GetByID(ctx context.Context, id shared.LibraryID) (*library.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.library.GetByID")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	lib := &library.Library{}
	var pathsJSON []byte
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at, scan_on_startup, realtime_monitoring
		 FROM libraries WHERE id = $1`, string(id),
	).Scan(&lib.ID, &lib.Name, &lib.Type, &pathsJSON, &lib.ScanIntervalMinutes, &lib.Enabled, &lib.LastScannedAt, &lib.CreatedAt, &lib.UpdatedAt, &lib.ScanOnStartup, &lib.RealtimeMonitoring)
	if err != nil {
		return nil, fmt.Errorf("LibraryRepo.GetByID: %w", err)
	}
	if err := json.Unmarshal(pathsJSON, &lib.Paths); err != nil {
		return nil, fmt.Errorf("LibraryRepo.GetByID unmarshal paths: %w", err)
	}
	return lib, nil
}

func (r *LibraryRepository) List(ctx context.Context, filter library.ListLibrariesFilter) ([]*library.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.library.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	const baseQuery = `SELECT id, name, type, paths, scan_interval_minutes, enabled, last_scanned_at, created_at, updated_at, scan_on_startup, realtime_monitoring
		 FROM libraries ORDER BY name, id`
	var (
		rows *sql.Rows
		err  error
	)
	if filter.Limit <= 0 {
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx, baseQuery)
	} else {
		limit := filter.Limit
		if limit > library.MaxListLibrariesLimit {
			limit = library.MaxListLibrariesLimit
		}
		offset := filter.Offset
		if offset < 0 {
			offset = 0
		}
		rows, err = txFromCtx(ctx, r.db).QueryContext(ctx, baseQuery+` LIMIT $1 OFFSET $2`, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("LibraryRepo.List: %w", err)
	}
	defer rows.Close()

	var result []*library.Library
	for rows.Next() {
		lib := &library.Library{}
		var pathsJSON []byte
		if err := rows.Scan(&lib.ID, &lib.Name, &lib.Type, &pathsJSON, &lib.ScanIntervalMinutes, &lib.Enabled, &lib.LastScannedAt, &lib.CreatedAt, &lib.UpdatedAt, &lib.ScanOnStartup, &lib.RealtimeMonitoring); err != nil {
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
	ctx, span := tracing.StartSpan(ctx, "postgres.library.Update")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	pathsJSON, err := json.Marshal(lib.Paths)
	if err != nil {
		return fmt.Errorf("LibraryRepo.Update marshal paths: %w", err)
	}
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE libraries SET name=$1, type=$2, paths=$3, scan_interval_minutes=$4, enabled=$5, last_scanned_at=$6, updated_at=$7, scan_on_startup=$8, realtime_monitoring=$9
		 WHERE id=$10`,
		lib.Name, lib.Type, pathsJSON, lib.ScanIntervalMinutes, lib.Enabled, lib.LastScannedAt, lib.UpdatedAt, lib.ScanOnStartup, lib.RealtimeMonitoring, lib.ID,
	)
	if err != nil {
		return fmt.Errorf("LibraryRepo.Update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *LibraryRepository) Delete(ctx context.Context, id shared.LibraryID) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.library.Delete")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM libraries WHERE id = $1`, string(id))
	if err != nil {
		return fmt.Errorf("LibraryRepo.Delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

var (
	_ library.LibraryCommandRepo = (*LibraryRepository)(nil)
	_ library.LibraryQueryRepo   = (*LibraryRepository)(nil)
)
