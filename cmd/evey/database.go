package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/lib/pq"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/config"
)

// openDBReadOnly opens the database WITHOUT running migrations. For admin
// subcommands that only read (the load-test token forge): they may run while
// the server is LIVE, and racing RunMigrations against a serving process -
// plus writing a pre-migration backup as a side effect of a load test - is
// exactly what a read never needs.
func openDBReadOnly(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err2 := db.Ping(); err2 != nil {
		return nil, fmt.Errorf("ping db: %w", err2)
	}
	return db, nil
}

// privilegeSplitBudget bounds the role check and the one-time ownership hand-over.
const privilegeSplitBudget = 2 * time.Minute

// appLogin returns the login the server's pool and migrations use: the
// unprivileged app role when the configured login is a superuser (see
// postgres.EnsureAppRole), the configured login otherwise. Fail-closed: a
// split that errors stops the boot, EVEY_DB_PRIVILEGE_SPLIT=false opts out.
func appLogin(cfg *config.Config) (config.DBConfig, error) {
	if !cfg.DB.PrivilegeSplit {
		slog.Warn("database: EVEY_DB_PRIVILEGE_SPLIT=false - the app runs as the configured login, superuser included")
		return cfg.DB, nil
	}
	admin, err := openDBReadOnly(cfg)
	if err != nil {
		return cfg.DB, err
	}
	defer func() { _ = admin.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), privilegeSplitBudget)
	defer cancel()
	password, applied, err := postgres.EnsureAppRole(ctx, admin, cfg.DB.AppUser)
	if err != nil {
		return cfg.DB, fmt.Errorf("database privilege split (EVEY_DB_PRIVILEGE_SPLIT=false runs as the configured login instead): %w", err)
	}
	if !applied {
		return cfg.DB, nil
	}
	app := cfg.DB
	app.User, app.Password = cfg.DB.AppUser, password
	return app, nil
}

func openDB(cfg *config.Config) (*sql.DB, error) {
	login, err := appLogin(cfg)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", login.DSN())
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err2 := db.Ping(); err2 != nil {
		return nil, fmt.Errorf("ping db as %s: %w", login.User, err2)
	}
	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// The rollback dump keeps the configured login: it must read everything.
	if err2 := postgres.RunMigrations(db, postgres.BackupOpts{
		Dir:       cfg.Backup.Dir,
		Host:      cfg.DB.Host,
		Port:      cfg.DB.Port,
		User:      cfg.DB.User,
		Password:  cfg.DB.Password,
		DBName:    cfg.DB.DBName,
		Retention: 3,
	}); err2 != nil {
		return nil, fmt.Errorf("migrations: %w", err2)
	}
	return db, nil
}

// handOverToAppRole gives the app role what an admin command just created,
// without rotating its password under the live server. No-op when the split
// is off or the configured login is not a superuser.
func handOverToAppRole(cfg *config.Config, db *sql.DB) error {
	if !cfg.DB.PrivilegeSplit {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), privilegeSplitBudget)
	defer cancel()
	if _, err := postgres.HandOverToAppRole(ctx, db, cfg.DB.AppUser); err != nil {
		return fmt.Errorf("hand the schema to %s: %w", cfg.DB.AppUser, err)
	}
	return nil
}
