package testhelpers

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var sharedDB *sql.DB

// StartShared starts a single postgres container for the whole test package.
// Call once from TestMain and defer the returned cleanup.
func StartShared() func() {
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
		panic(fmt.Sprintf("start shared postgres container: %v", err))
	}

	host, _ := container.Host(ctx)        //nolint:errcheck // panic on nil values handled below
	port, _ := container.MappedPort(ctx, "5432") //nolint:errcheck

	dsn := fmt.Sprintf("host=%s port=%s user=evey_test password=evey_test dbname=evey_test sslmode=disable", host, port.Port())
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(fmt.Sprintf("open db: %v", err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("ping db: %v", err))
	}
	if err := postgres.RunMigrations(db); err != nil {
		panic(fmt.Sprintf("run migrations: %v", err))
	}

	sharedDB = db

	return func() {
		db.Close()
		if termErr := container.Terminate(ctx); termErr != nil {
			fmt.Printf("terminate shared container: %v\n", termErr)
		}
	}
}

// StartPostgres returns the shared DB with all tables truncated for test isolation.
// Falls back to a dedicated container if called outside TestMain (e.g. standalone).
func StartPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	if sharedDB == nil {
		return startDedicated(t)
	}
	truncateAll(t, sharedDB)
	return sharedDB, func() {}
}

// seedSQL re-inserts rows that migrations seed and tests depend on as initial state.
const seedSQL = `
INSERT INTO feed_admin_config (feed_type, enabled, display_order, max_items) VALUES
    ('continue_watching', true, 0, 20),
    ('recently_added',    true, 1, 20),
    ('recommended',       true, 2, 20);
INSERT INTO notification_settings_global (id) VALUES (1);
`

func truncateAll(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename != 'schema_migrations'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close() //nolint:errcheck
	var tables []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			t.Fatalf("scan table name: %v", scanErr)
		}
		tables = append(tables, fmt.Sprintf("%q", name))
	}
	if len(tables) == 0 {
		return
	}
	if _, execErr := db.ExecContext(context.Background(),
		fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))); execErr != nil {
		t.Fatalf("truncate tables: %v", execErr)
	}
	if _, execErr := db.ExecContext(context.Background(), seedSQL); execErr != nil {
		t.Fatalf("re-seed defaults: %v", execErr)
	}
}

// startDedicated spins up an individual container (fallback for standalone test runs).
func startDedicated(t *testing.T) (*sql.DB, func()) {
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

	host, _ := container.Host(ctx)        //nolint:errcheck
	port, _ := container.MappedPort(ctx, "5432") //nolint:errcheck

	dsn := fmt.Sprintf("host=%s port=%s user=evey_test password=evey_test dbname=evey_test sslmode=disable", host, port.Port())
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	if err := postgres.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return db, func() {
		db.Close()
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("terminate container: %v", termErr)
		}
	}
}
