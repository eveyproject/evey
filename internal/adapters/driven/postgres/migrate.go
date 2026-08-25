package postgres

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// migrationHeartbeatInterval paces the "still running" log below. Package
// variable so the test can fire it without waiting half a minute.
var migrationHeartbeatInterval = 30 * time.Second

// migrationHeartbeat logs a beat until its stop function is called. The
// migration runner has no context (golang-migrate holds the bare *sql.DB), so
// on a database that accepts the connection but never answers, the whole boot
// used to hang with ZERO log lines - the first line only appeared once Up()
// succeeded or failed. Restarting a server mid-incident therefore looked
// identical to a dead binary. The heartbeat does not bound anything; it makes
// the wait visible and names the two states it can mean.
func migrationHeartbeat() (stop func()) {
	start := time.Now()
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(migrationHeartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				slog.Warn("migrations: still running - the database has not answered yet, or a long migration is in flight",
					slog.Duration("elapsed", time.Since(start).Round(time.Second)))
			}
		}
	}()
	return func() { close(done) }
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all pending up migrations. When opts.Dir is
// non-empty AND a migration is actually going to be applied, a pg_dump of
// the current schema is written there first so the operator has a rollback
// artefact if Up() fails. Tests pass an empty BackupOpts to skip the dump.
//
// Failure policy: a dump that cannot be taken FAILS the call, and therefore
// the boot. Migrating a single-operator NAS with no way back is worse than
// a server that refuses to start and says why. What makes that gate
// tolerable is that it is now only reached on a boot that would change the
// schema - a missing pg_dump or an unwritable backup dir no longer bricks
// every restart of an up-to-date server. The escape hatch, when a migration
// really must go through without a net, is `evey admin migrate up
// --no-backup`.
// migrationRunBudget bounds one whole RunMigrations call. Generous on purpose:
// a legitimate schema migration on a weak NAS can take many minutes, and the
// budget only exists for the pathological case - a database that accepts the
// connection and then never answers. Package variable so the test can trip it.
var migrationRunBudget = 20 * time.Minute

func RunMigrations(db *sql.DB, opts BackupOpts) error {
	// From the very first db touch (m.Version() below) to the end of Up():
	// everything in here can block without a deadline, so keep the operator
	// informed for the whole span - and bound the whole span, because
	// golang-migrate offers no context: an unanswered query would otherwise
	// hold the boot forever. On expiry the worker goroutine may stay parked on
	// its dead connection; the caller fails the boot right after, so the
	// process exits and takes it along.
	defer migrationHeartbeat()()

	done := make(chan error, 1)
	go func() { done <- runMigrations(db, opts) }()
	select {
	case err := <-done:
		return err
	case <-time.After(migrationRunBudget):
		return fmt.Errorf("migrations: not finished after %s - the database is not answering or a migration is stuck; failing the boot so this is an error, not a silent hang", migrationRunBudget)
	}
}

func runMigrations(db *sql.DB, opts BackupOpts) error {
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

	// Retention runs whatever happens below. It used to hang off a
	// successful Up(), so a server crash-looping on a failing migration
	// piled dumps into /data until the volume filled - the one moment an
	// operator can least afford a full disk.
	if opts.Dir != "" {
		defer pruneOldBackups(opts.Dir, retentionOrDefault(opts))
	}

	// Snapshot current version *before* taking the dump so the filename
	// names the state we're leaving - that's what the operator will roll
	// back to if Up() fails.
	currentVersion, dirty, vErr := m.Version()
	if vErr != nil && !errors.Is(vErr, migrate.ErrNilVersion) {
		return fmt.Errorf("read current migration version: %w", vErr)
	}

	pending, pendErr := hasPendingMigrations(source, currentVersion, errors.Is(vErr, migrate.ErrNilVersion))
	if pendErr != nil {
		return fmt.Errorf("read pending migrations: %w", pendErr)
	}
	if dirty {
		// golang-migrate refuses to run on a dirty schema, so Up() below will
		// apply nothing - there is no new state to take a rollback artefact for.
		slog.Warn("migrations: schema is in dirty state - a previous migration died half way; no backup taken because nothing will be applied",
			slog.Uint64("version", uint64(currentVersion)))
	}

	var dumpPath string
	if opts.Dir != "" && pending && !dirty {
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

	return nil
}

// hasPendingMigrations answers the question m.Up() is about to answer: is
// there a migration after the one the schema sits on? It mirrors
// golang-migrate's own walk (First when nothing is applied, Next otherwise)
// so the two cannot disagree about whether the boot is going to do anything.
func hasPendingMigrations(src source.Driver, current uint, nothingApplied bool) (bool, error) {
	var err error
	if nothingApplied {
		_, err = src.First()
	} else {
		_, err = src.Next(current)
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func retentionOrDefault(opts BackupOpts) int {
	if opts.Retention <= 0 {
		return defaultBackupRetention
	}
	return opts.Retention
}

// RollbackLastMigration reverts exactly one migration step. Intended for the
// `cmd/migrate down` operator path - surgical, not a "tear it all down"
// button. Returns migrate.ErrNoChange when the schema is already at
// version 0.
//
// Unlike RunMigrations it dumps unconditionally (minus the version-0 case):
// this runs because a human typed it, not on every restart, so there is no
// crash loop to protect against and the snapshot is worth its seconds.
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

	if opts.Dir != "" {
		defer pruneOldBackups(opts.Dir, retentionOrDefault(opts))
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
