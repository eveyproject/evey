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
	// names the state we're leaving — that's what the operator will roll
	// back to if Up() fails.
	currentVersion, dirty, vErr := m.Version()
	if vErr != nil && !errors.Is(vErr, migrate.ErrNilVersion) {
		return fmt.Errorf("read current migration version: %w", vErr)
	}
	if dirty {
		slog.Warn("migrations: schema is in dirty state — backup taken anyway",
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
			slog.Error("migrations: Up failed — restore from backup",
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
