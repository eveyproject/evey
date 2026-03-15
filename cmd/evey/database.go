package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/config"
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

	if err2 := postgres.RunMigrations(db); err2 != nil {
		return nil, fmt.Errorf("migrations: %w", err2)
	}
	return db, nil
}
