package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/golang-migrate/migrate/v4"
	mpostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMigrate_UpDownUp_Roundtrip applies all migrations Up, rolls
// everything back via Down, then re-applies Up and asserts the resulting
// schema matches the first Up snapshot. Catches any broken Down()
// migration (missing DROP, partial rollback, typo) that would otherwise
// hide until an operator actually rolled back in prod.
func TestMigrate_UpDownUp_Roundtrip(t *testing.T) {
	testhelpers.SkipIfNoDocker(t)
	if testing.Short() {
		t.Skip("skipping integration test under -short")
	}

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		// Durability off: disposable test database (see testhelpers.StartShared).
		Cmd: []string{"-c", "fsync=off", "-c", "synchronous_commit=off", "-c", "full_page_writes=off"},
		Env: map[string]string{
			"POSTGRES_USER":     "evey_test",
			"POSTGRES_PASSWORD": "evey_test",
			"POSTGRES_DB":       "evey_test",
		},
		// During initdb, the listen socket briefly opens while postgres
		// is still bootstrapping - Ping lands on "connection reset by
		// peer". The ready-log appears twice (init + final restart),
		// so ForAll(ForLog occurrence=2, ForListeningPort) is the correct
		// wait combo used by every other test in this package.
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithStartupTimeoutDefault(90 * time.Second),
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	t.Cleanup(func() {
		if termErr := c.Terminate(ctx); termErr != nil {
			t.Logf("terminate container: %v", termErr)
		}
	})

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := c.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(mapped.Port())
	if err != nil {
		t.Fatal(err)
	}

	dsn := fmt.Sprintf("host=%s port=%d user=evey_test password=evey_test dbname=evey_test sslmode=disable", host, port)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}

	m := newMigrator(t, db)
	t.Cleanup(func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			t.Logf("migrator source close: %v", srcErr)
		}
		if dbErr != nil {
			t.Logf("migrator db close: %v", dbErr)
		}
	})

	// 1) First Up - apply everything.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("first Up: %v", err)
	}
	firstSnap := schemaSnapshot(t, db)
	if firstSnap == "" {
		t.Fatal("first snapshot is empty - no tables created")
	}

	// 2) Down - rollback all. golang-migrate Down() with no args removes
	// every migration currently applied.
	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("Down: %v", err)
	}
	postDown := schemaSnapshot(t, db)
	if postDown != "" {
		t.Fatalf("Down left tables behind:\n%s", postDown)
	}

	// 3) Second Up - everything must re-apply cleanly on the empty DB.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("second Up: %v", err)
	}
	secondSnap := schemaSnapshot(t, db)

	if firstSnap != secondSnap {
		t.Fatalf("schema diverged after down+up roundtrip\n--- first Up ---\n%s\n--- second Up ---\n%s",
			firstSnap, secondSnap)
	}
}

// newMigrator builds a migrate.Migrate pointed at the package-local
// migrations dir. Uses iofs+os.DirFS rather than file:// to sidestep URL
// escaping / absolute-path quirks.
func newMigrator(t *testing.T, db *sql.DB) *migrate.Migrate {
	t.Helper()
	src, err := iofs.New(os.DirFS("migrations"), ".")
	if err != nil {
		t.Fatalf("iofs source: %v", err)
	}
	driver, err := mpostgres.WithInstance(db, &mpostgres.Config{})
	if err != nil {
		t.Fatalf("postgres driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		t.Fatalf("migrate instance: %v", err)
	}
	return m
}

// schemaSnapshot returns a deterministic string representation of the
// public-schema tables (name + ordered columns with data types). The
// migrate bookkeeping table `schema_migrations` is excluded since it is
// an artefact of the tool, not the domain schema.
func schemaSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()

	rows, err := db.Query(`
		SELECT table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name <> 'schema_migrations'
		ORDER BY table_name, ordinal_position
	`)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()

	type col struct{ name, dtype string }
	tables := make(map[string][]col)
	for rows.Next() {
		var tbl, name, dtype string
		if err := rows.Scan(&tbl, &name, &dtype); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables[tbl] = append(tables[tbl], col{name: name, dtype: dtype})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	names := make([]string, 0, len(tables))
	for n := range tables {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, n := range names {
		cols := tables[n]
		// columns already ordered by ordinal_position; sort by name for
		// a schema-structural (not insertion-order) comparison so a column
		// added mid-table vs appended is still considered equivalent.
		sort.Slice(cols, func(i, j int) bool { return cols[i].name < cols[j].name })
		b.WriteString(n)
		b.WriteString(":\n")
		for _, c := range cols {
			b.WriteString("  ")
			b.WriteString(c.name)
			b.WriteString(" ")
			b.WriteString(c.dtype)
			b.WriteString("\n")
		}
	}
	return b.String()
}
