package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"

	"github.com/eveyproject/evey/internal/adapters/driven/filesystem"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	httpapi "github.com/eveyproject/evey/internal/adapters/driving/http"
	appbackup "github.com/eveyproject/evey/internal/app/backup"
	"github.com/eveyproject/evey/internal/config"
)

// adminBackup creates a database backup from the CLI - the same artifact the
// admin panel and the scheduler produce. Telemetry tables are excluded by
// default to keep the backup lean; the persisted panel setting is honoured
// unless --include-telemetry overrides it for this run.
func adminBackup(args []string) error {
	fs := adminFlags("backup")
	include := fs.Bool("include-telemetry", false,
		"dump the high-volume telemetry tables too (default: the saved panel setting, normally off)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// An explicit flag wins; otherwise fall back to the persisted panel setting
	// so the CLI and the UI agree on what a plain backup includes.
	includeTelemetry := *include
	explicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "include-telemetry" {
			explicit = true
		}
	})
	if !explicit {
		if db, derr := sql.Open("postgres", cfg.DB.DSN()); derr == nil {
			defer db.Close()
			store := httpapi.NewServerConfigStore(postgres.NewServerConfigRepository(db))
			if v, gerr := store.Get(context.Background(), httpapi.BackupIncludeTelemetryKey); gerr == nil && v == "true" {
				includeTelemetry = true
			}
		}
	}

	svc := appbackup.NewService(
		filesystem.NewBackupStore(cfg.Backup.Dir),
		postgres.NewDatabaseDumper(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.DBName, cfg.DB.SSLMode),
		buildID().Version,
		appbackup.WithIncludeTelemetry(func() bool { return includeTelemetry }),
	)
	m, err := svc.CreateBackup(context.Background())
	handBackToDirOwner(cfg.Backup.Dir)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	telemetry := "excluded"
	if includeTelemetry {
		telemetry = "included"
	}
	fmt.Printf("backup created: %s (schema %s, telemetry %s)\n", m.ID, m.SchemaVersion, telemetry)
	return nil
}
