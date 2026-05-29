// Package main implements `evey-migrate`: an operator tool for applying or
// rolling back database migrations without booting the full Evey server.
//
// Why a separate binary exists when `cmd/evey` already runs migrations at
// startup: prod deployments orchestrated by CI/release pipelines need to be
// able to migrate before the app rolls over (canary deploys, blue-green,
// rollback drills). The embedded-at-boot path is convenient for dev — Air
// rebuilds the server and migrations slip in automatically — but couples
// "schema change" to "process restart". This binary breaks that coupling.
//
// Usage:
//
//	go run ./cmd/migrate up                  apply all pending migrations
//	go run ./cmd/migrate up --no-backup      skip the pg_dump (host dev path)
//	go run ./cmd/migrate down                roll back exactly one migration
//	go run ./cmd/migrate down --no-backup    skip the pg_dump
//	go run ./cmd/migrate version             print current schema version
//
// Configuration comes from the same env vars as `cmd/evey` (EVEY_DB_USER,
// EVEY_DB_PASSWORD, EVEY_DB_HOST, EVEY_DB_PORT, EVEY_DB_NAME, EVEY_BACKUP_DIR).
// A pre-migration pg_dump is written to EVEY_BACKUP_DIR when it's set
// (matches the server-startup path) unless --no-backup is passed. The flag
// matters on hosts that don't have pg_dump in PATH — typical for WSL dev
// where the postgres-client package isn't installed because Postgres only
// runs inside Docker.
package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/lib/pq"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		usageAndExit("missing command")
	}

	cmd := os.Args[1]
	noBackup := false
	for _, a := range os.Args[2:] {
		if a == "--no-backup" {
			noBackup = true
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}

	db, err := openDB(cfg)
	if err != nil {
		fatal("db: %v", err)
	}
	defer func() { _ = db.Close() }()

	backupOpts := postgres.BackupOpts{
		Dir:       cfg.Backup.Dir,
		Host:      cfg.DB.Host,
		Port:      cfg.DB.Port,
		User:      cfg.DB.User,
		Password:  cfg.DB.Password,
		DBName:    cfg.DB.DBName,
		Retention: 3,
	}
	if noBackup {
		backupOpts.Dir = ""
	}

	switch cmd {
	case "up":
		if err := postgres.RunMigrations(db, backupOpts); err != nil {
			fatal("migrate up: %v", err)
		}
		fmt.Println("migrations: up — schema at latest version")
	case "down":
		if err := postgres.RollbackLastMigration(db, backupOpts); err != nil {
			fatal("migrate down: %v", err)
		}
		fmt.Println("migrations: down — one step rolled back")
	case "version":
		v, dirty, err := postgres.CurrentMigrationVersion(db)
		if err != nil {
			fatal("migrate version: %v", err)
		}
		if dirty {
			fmt.Printf("schema version: %d (DIRTY — previous migration failed)\n", v)
			os.Exit(1)
		}
		fmt.Printf("schema version: %d\n", v)
	default:
		usageAndExit(fmt.Sprintf("unknown command %q", cmd))
	}
}

// openDB mirrors the cmd/evey openDB without the SetMaxOpenConns tuning —
// migrations open one connection, do their thing, exit. No pool to size.
func openDB(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

func usageAndExit(msg string) {
	fmt.Fprintln(os.Stderr, "evey-migrate:", msg)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/migrate up       apply pending migrations")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/migrate down     roll back one migration step")
	fmt.Fprintln(os.Stderr, "  go run ./cmd/migrate version  print current schema version")
	os.Exit(2)
}

func fatal(format string, args ...any) {
	slog.Error("migrate", "err", fmt.Sprintf(format, args...))
	os.Exit(1)
}
