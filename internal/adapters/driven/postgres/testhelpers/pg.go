package testhelpers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var sharedDB *sql.DB

var (
	dockerProbeOnce sync.Once
	dockerReachable bool
)

// dockerAvailable reports whether a Docker daemon is reachable for
// testcontainers. It lets the postgres-integration tests SKIP (rather than fail)
// where Docker is absent - e.g. inside the app container, which has no socket by
// design - while still running wherever Docker exists (host, CI). The probe is
// cached: it costs one provider handshake per test binary, not one per test.
func dockerAvailable() bool {
	dockerProbeOnce.Do(func() {
		provider, err := testcontainers.ProviderDocker.GetProvider()
		if err != nil {
			return
		}
		defer func() { _ = provider.Close() }()
		if dp, ok := provider.(*testcontainers.DockerProvider); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			dockerReachable = dp.Health(ctx) == nil
			return
		}
		dockerReachable = true
	})
	return dockerReachable
}

// SkipIfNoDocker skips the calling test when no Docker daemon is reachable, for
// integration tests that spin up their OWN testcontainer inline rather than
// going through StartPostgres. Same rationale: green everywhere, real container
// wherever Docker exists (host, CI).
func SkipIfNoDocker(t *testing.T) {
	t.Helper()
	if !dockerAvailable() {
		t.Skip("integration test: Docker not available (run on a host with Docker, or via CI)")
	}
}

// StartShared starts a single postgres container for the whole test package.
// Call once from TestMain and defer the returned cleanup.
func StartShared() func() {
	// Docker absent (e.g. inside the app container): leave sharedDB nil and let
	// each StartPostgres(t) skip. TestMain must not panic here.
	if !dockerAvailable() {
		return func() {}
	}
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		// Durability off: the database is disposable, and the per-test
		// truncateAll (TRUNCATE of ~130 tables = that many fsyncs) dominated
		// the suite's runtime - 403 tests × ~0.75s ≈ the whole 300s budget,
		// which is what actually "hung" CI on 2026-07-10/11.
		Cmd: []string{"-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off"},
		Env: map[string]string{
			"POSTGRES_USER":     "evey_test",
			"POSTGRES_PASSWORD": "evey_test",
			"POSTGRES_DB":       "evey_test",
		},
		// Port-listen alone races the postgres init (server briefly
		// listens while bootstrapping initdb, then restarts). The
		// ready log appears twice - initdb + final boot - so
		// WithOccurrence(2) is the standard pg wait.
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithStartupTimeoutDefault(90 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		panic(fmt.Sprintf("start shared postgres container: %v", err))
	}

	host, err := container.Host(ctx)
	if err != nil {
		panic(fmt.Sprintf("testhelpers: container.Host: %v", err))
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		panic(fmt.Sprintf("testhelpers: container.MappedPort: %v", err))
	}

	dsn := fmt.Sprintf("host=%s port=%s user=evey_test password=evey_test dbname=evey_test sslmode=disable", host, port.Port())
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(fmt.Sprintf("open db: %v", err))
	}
	// Retry ping - port may be listening before Postgres is fully ready
	for range 30 {
		if err = db.Ping(); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		panic(fmt.Sprintf("ping db after retries: %v", err))
	}
	if err := postgres.RunMigrations(db, postgres.BackupOpts{}); err != nil {
		panic(fmt.Sprintf("run migrations: %v", err))
	}

	sharedDB = db

	return func() {
		db.Close()
		if termErr := container.Terminate(ctx); termErr != nil {
			slog.Warn("terminate shared container", "err", termErr)
		}
	}
}

// StartPostgres returns the shared DB with all tables truncated for test isolation.
// Falls back to a dedicated container if called outside TestMain (e.g. standalone).
func StartPostgres(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	if sharedDB == nil {
		if !dockerAvailable() {
			t.Skip("postgres integration test: Docker not available (run on a host with Docker, or via CI)")
		}
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

// LockTimeout bounds how long truncateAll waits for the ACCESS EXCLUSIVE
// locks a TRUNCATE needs. One transaction leaked anywhere in the suite (even
// a stray SELECT's ACCESS SHARE) otherwise turns the NEXT test's setup into
// an unbounded hang - CI 2026-07-11 sat five minutes in netpoll and the
// package-timeout dump named the victim, not the culprit. Exported so the
// harness's own test can shrink it.
var LockTimeout = 15 * time.Second

func truncateAll(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename != 'schema_migrations'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			t.Fatalf("scan table name: %v", scanErr)
		}
		tables = append(tables, fmt.Sprintf("%q", name))
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		t.Fatalf("rows iteration: %v", rowsErr)
	}
	if len(tables) == 0 {
		return
	}
	trunc := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))
	if execErr := execWithLockTimeout(db, trunc); execErr != nil {
		// Bounded instead of hung: name the open transactions (the culprit
		// is among them), terminate them, retry once. The loud log turns a
		// once-in-a-blue-moon CI hang into an actionable failure report.
		blockers := describeOpenTransactions(db)
		t.Logf("truncateAll: TRUNCATE blocked (%v); open transactions in the test database:\n%s", execErr, blockers)
		killOpenTransactions(db)
		if retryErr := execWithLockTimeout(db, trunc); retryErr != nil {
			t.Fatalf("truncate tables (even after terminating leaked transactions): %v\nblockers were:\n%s", retryErr, blockers)
		}
		t.Logf("truncateAll: recovered by terminating the leaked transaction(s) above - fix the test that keeps a transaction open")
	}
	if _, execErr := db.ExecContext(context.Background(), seedSQL); execErr != nil {
		t.Fatalf("re-seed defaults: %v", execErr)
	}
}

// execWithLockTimeout runs stmt on a pinned connection with lock_timeout set,
// so a lock conflict surfaces as an error after LockTimeout instead of
// blocking until the package timeout. The setting is reset before the
// connection returns to the pool.
func execWithLockTimeout(db *sql.DB, stmt string) error {
	ctx, cancel := context.WithTimeout(context.Background(), LockTimeout+30*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("SET lock_timeout = %d", LockTimeout.Milliseconds())); err != nil {
		return fmt.Errorf("set lock_timeout: %w", err)
	}
	_, execErr := conn.ExecContext(ctx, stmt)
	if _, resetErr := conn.ExecContext(ctx, "SET lock_timeout = 0"); resetErr != nil && execErr == nil {
		execErr = fmt.Errorf("reset lock_timeout: %w", resetErr)
	}
	return execErr
}

// describeOpenTransactions returns one line per backend holding an open
// transaction in the test database (excluding the caller): pid, state, age
// and last query - everything needed to identify the leaking test.
func describeOpenTransactions(db *sql.DB) string {
	rows, err := db.QueryContext(context.Background(), `
		SELECT pid, state, COALESCE(now() - xact_start, '0'::interval)::text, COALESCE(query, '')
		FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND xact_start IS NOT NULL`)
	if err != nil {
		return fmt.Sprintf("(pg_stat_activity unreadable: %v)", err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var pid int
		var state, age, query string
		if rows.Scan(&pid, &state, &age, &query) == nil {
			fmt.Fprintf(&b, "  pid=%d state=%s xact_age=%s query=%q\n", pid, state, age, query)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		fmt.Fprintf(&b, "  (pg_stat_activity iteration stopped early: %v)\n", rowsErr)
	}
	if b.Len() == 0 {
		return "  (none visible - the blocker may have ended between the timeout and this query)"
	}
	return b.String()
}

// killOpenTransactions terminates every backend with an open transaction in
// the test database except the caller's own. Test-harness only: leaked
// transactions have no legitimate owner here.
func killOpenTransactions(db *sql.DB) {
	_, _ = db.ExecContext(context.Background(), `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND xact_start IS NOT NULL`)
}

// startDedicated spins up an individual container (fallback for standalone test runs).
func startDedicated(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		// Same durability-off flags as the shared container (see StartShared).
		Cmd: []string{"-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off"},
		Env: map[string]string{
			"POSTGRES_USER":     "evey_test",
			"POSTGRES_PASSWORD": "evey_test",
			"POSTGRES_DB":       "evey_test",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithStartupTimeoutDefault(90 * time.Second),
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
	// Same retry loop as StartShared - the ready-log wait above mostly
	// covers the init race, but the kernel sometimes still hands us a
	// half-closed socket on the first Ping under load.
	var pingErr error
	for range 30 {
		if pingErr = db.Ping(); pingErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if pingErr != nil {
		t.Fatalf("ping db after retries: %v", pingErr)
	}
	if err := postgres.RunMigrations(db, postgres.BackupOpts{}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return db, func() {
		db.Close()
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("terminate container: %v", termErr)
		}
	}
}
