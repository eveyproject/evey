package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/config"
)

func openDB(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err2 := db.Ping(); err2 != nil {
		return nil, fmt.Errorf("ping db: %w", err2)
	}
	db.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	db.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

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
