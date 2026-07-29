package postgres

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending up migrations. When opts.Dir is
// non-empty, a pg_dump of the current schema is written there before
// migrations start so the operator has a rollback artefact if Up()
// fails. Tests pass an empty BackupOpts to skip the dump.
func RunMigrations(db *sql.DB, opts BackupOpts) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return err
	}

	// Snapshot current version *before* taking the dump so the filename
	// names the state we're leaving - that's what the operator will roll
	// back to if Up() fails.
	currentVersion, dirty, vErr := m.Version()
	if vErr != nil && !errors.Is(vErr, migrate.ErrNilVersion) {
		return fmt.Errorf("read current migration version: %w", vErr)
	}
	if dirty {
		slog.Warn("migrations: schema is in dirty state - backup taken anyway",
			slog.Uint64("version", uint64(currentVersion)))
	}

	var dumpPath string
	if opts.Dir != "" {
		path, dumpErr := dumpDatabase(opts, currentVersion)
		if dumpErr != nil {
			return fmt.Errorf("pre-migration backup: %w", dumpErr)
		}
		dumpPath = path
		slog.Info("migrations: pre-migration backup written",
			slog.String("path", dumpPath),
			slog.Uint64("from_version", uint64(currentVersion)))
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		if dumpPath != "" {
			// Boot-time migration runner, before any request context exists.
			slog.Error("migrations: Up failed - restore from backup",
				slog.String("backup", dumpPath),
				slog.Any("err", err))
		}
		return err
	}

	if opts.Dir != "" {
		keep := opts.Retention
		if keep <= 0 {
			keep = defaultBackupRetention
		}
		pruneOldBackups(opts.Dir, keep)
	}

	return nil
}

// RollbackLastMigration reverts exactly one migration step. Intended for the
// `cmd/migrate down` operator path - surgical, not a "tear it all down"
// button. Pre-rollback backup is taken when opts.Dir is non-empty, same
// contract as RunMigrations. Returns migrate.ErrNoChange when the schema
// is already at version 0.
func RollbackLastMigration(db *sql.DB, opts BackupOpts) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return err
	}

	currentVersion, dirty, vErr := m.Version()
	if vErr != nil {
		if errors.Is(vErr, migrate.ErrNilVersion) {
			return migrate.ErrNoChange
		}
		return fmt.Errorf("read current migration version: %w", vErr)
	}
	if dirty {
		slog.Warn("migrations: schema is in dirty state - rolling back anyway",
			slog.Uint64("version", uint64(currentVersion)))
	}

	if opts.Dir != "" {
		path, dumpErr := dumpDatabase(opts, currentVersion)
		if dumpErr != nil {
			return fmt.Errorf("pre-rollback backup: %w", dumpErr)
		}
		slog.Info("migrations: pre-rollback backup written",
			slog.String("path", path),
			slog.Uint64("from_version", uint64(currentVersion)))
	}

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// CurrentMigrationVersion returns the version the schema is currently at,
// plus a "dirty" flag set when a previous migration failed halfway.
// Useful for the `cmd/migrate version` operator path. Returns 0 + false
// when no migrations have ever run.
func CurrentMigrationVersion(db *sql.DB) (uint, bool, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return 0, false, err
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return 0, false, err
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return 0, false, err
	}
	v, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, dirty, nil
}
