package testhelpers

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func StartPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "evey_test",
			"POSTGRES_PASSWORD": "evey_test",
			"POSTGRES_DB":       "evey_test",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")

	dsn := fmt.Sprintf("host=%s port=%s user=evey_test password=evey_test dbname=evey_test sslmode=disable", host, port.Port())
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	runMigrations(t, db)

	cleanup := func() {
		db.Close()
		container.Terminate(ctx)
	}
	return db, cleanup
}

func runMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "migrations")
	migrations := []string{
		filepath.Join(migrationsDir, "000001_init.up.sql"),
		filepath.Join(migrationsDir, "000002_transcode_profiles.up.sql"),
		filepath.Join(migrationsDir, "000003_profile_languages.up.sql"),
		filepath.Join(migrationsDir, "000004_audit_log_enrich.up.sql"),
		filepath.Join(migrationsDir, "000005_notifications.up.sql"),
		filepath.Join(migrationsDir, "000006_webhooks.up.sql"),
		filepath.Join(migrationsDir, "000007_feeds.up.sql"),
	}
	for _, path := range migrations {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			t.Fatalf("run migration %s: %v", path, err)
		}
	}
}
