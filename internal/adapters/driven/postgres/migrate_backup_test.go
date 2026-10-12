package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgres brings up an ephemeral postgres and returns a live handle
// plus the BackupOpts pointing at a fresh backup dir.
func startPostgres(t *testing.T) (*sql.DB, postgres.BackupOpts) {
	t.Helper()
	testhelpers.SkipIfNoDocker(t)

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
		// Port-listen wait races the postgres init: during first-boot
		// initdb, the server opens the listen socket briefly while still
		// bootstrapping, and Ping lands on the "connection reset by
		// peer" that we just hit in CI. The ready-log appears twice
		// (init + final restart), so WithOccurrence(2) is the standard
		// belt-and-braces pattern for testcontainers + pg.
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
	// Ping retry loop to survive the final "connection reset by peer"
	// race when the postgres container flips its listen socket during
	// init - same pattern as testhelpers.StartShared.
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

	return db, postgres.BackupOpts{
		Dir:      t.TempDir(),
		Host:     host,
		Port:     port,
		User:     "evey_test",
		Password: "evey_test",
		DBName:   "evey_test",
	}
}

// TestRunMigrations_WritesBackup spins an ephemeral postgres + runs
// migrations with a tempdir backup target: a `pre-migration-*` gzip dump
// must land under the dir, because that first call really does move the
// schema from nothing to the latest version.
func TestRunMigrations_WritesBackup(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skipf("pg_dump not in PATH: %v", err)
	}
	db, opts := startPostgres(t)

	if err := postgres.RunMigrations(db, opts); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}

	dumps := listBackups(t, opts.Dir)
	if len(dumps) != 1 {
		t.Fatalf("expected 1 backup after first migration, got %d: %v", len(dumps), dumps)
	}
	if !strings.HasSuffix(dumps[0], ".sql.gz") {
		t.Errorf("dump must be gzip: %s", dumps[0])
	}
	info, err := os.Stat(filepath.Join(opts.Dir, dumps[0]))
	if err != nil {
		t.Fatalf("stat dump: %v", err)
	}
	if info.Size() < 100 {
		t.Errorf("dump suspiciously small (%d bytes)", info.Size())
	}
}

// TestRunMigrations_NoDumpWhenSchemaIsCurrent is the boot path of a server
// that has nothing to migrate - the overwhelmingly common case. Measured in
// production: a 14 s pg_dump on a start that took 14.5 s in total, taken
// while the schema was already at the target version. A dump nobody will
// ever restore is not a safety net, it is the whole startup budget.
//
// Deliberately does NOT require pg_dump: the point is that the dump path is
// never entered, so its absence must not change the outcome.
func TestRunMigrations_NoDumpWhenSchemaIsCurrent(t *testing.T) {
	db, opts := startPostgres(t)

	// Bring the schema to the latest version without leaving a dump behind.
	noBackup := opts
	noBackup.Dir = ""
	if err := postgres.RunMigrations(db, noBackup); err != nil {
		t.Fatalf("seed migrations: %v", err)
	}

	if err := postgres.RunMigrations(db, opts); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}

	if dumps := listBackups(t, opts.Dir); len(dumps) != 0 {
		t.Fatalf("dumped with nothing to migrate: %v", dumps)
	}
}

// TestRunMigrations_PrunesWhenUpFails covers the crash loop. A dirty schema
// makes Up() fail on every boot; with `restart: unless-stopped` the server
// retries forever. If retention only runs after a SUCCESSFUL Up, the backup
// dir grows without bound until /data is full - on a NAS, that takes the
// media server and everything else sharing the volume down with it.
func TestRunMigrations_PrunesWhenUpFails(t *testing.T) {
	db, opts := startPostgres(t)
	opts.Retention = 3

	noBackup := opts
	noBackup.Dir = ""
	if err := postgres.RunMigrations(db, noBackup); err != nil {
		t.Fatalf("seed migrations: %v", err)
	}
	// A migration that died half way leaves the bookkeeping row dirty, which
	// is exactly what golang-migrate refuses to run on.
	if _, err := db.Exec(`UPDATE schema_migrations SET dirty = true`); err != nil {
		t.Fatalf("dirty the schema: %v", err)
	}

	seedBackups(t, opts.Dir,
		"pre-migration-148-20260101T000000Z.sql.gz",
		"pre-migration-149-20260102T000000Z.sql.gz",
		"pre-migration-150-20260103T000000Z.sql.gz",
		"pre-migration-151-20260104T000000Z.sql.gz",
		"pre-migration-152-20260105T000000Z.sql.gz",
	)

	if err := postgres.RunMigrations(db, opts); err == nil {
		t.Fatal("expected RunMigrations to fail on a dirty schema")
	}

	left := listBackups(t, opts.Dir)
	if len(left) != 3 {
		t.Fatalf("retention did not run after a failed Up: %d dumps left: %v", len(left), left)
	}
	// And nothing new was written: Up applied nothing, so there was nothing
	// to take a rollback artefact for.
	for _, n := range left {
		if !strings.Contains(n, "2026010") {
			t.Errorf("unexpected fresh dump on a failed migration: %s", n)
		}
	}
}

func TestRunMigrations_RetentionPrunesOldDumps(t *testing.T) {
	backupDir := t.TempDir()
	// Seed 5 fake old dumps; pruneOldBackups should leave the 3 newest. Days
	// start at 01: "20260100" is not a date, and a dump we wrote always
	// carries a real one.
	for i := range 5 {
		name := fmt.Sprintf("pre-migration-%d-2026010%dT000000Z.sql.gz", i, i+1)
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	postgres.PruneOldBackupsForTest(backupDir, 3)
	left := listBackups(t, backupDir)
	if len(left) != 3 {
		t.Fatalf("expected 3 dumps after prune, got %d: %v", len(left), left)
	}
	// The 2 oldest (i=0, i=1) should be gone.
	for _, n := range left {
		if strings.Contains(n, "-0-") || strings.Contains(n, "-1-") {
			t.Errorf("oldest dump %s should have been pruned", n)
		}
	}
}

// TestPruneOldBackups_OrdersByAgeNotByName seeds dumps whose version numbers
// differ in digit count. Lexicographically "99" sorts AFTER "152", so a name
// sort ranks the oldest dump as the newest and deletes the one the operator
// would actually restore. Ordering must follow the UTC timestamp the filename
// embeds, which is fixed-width and therefore honest.
func TestPruneOldBackups_OrdersByAgeNotByName(t *testing.T) {
	dir := t.TempDir()
	// Chronological order: 99 (oldest) -> 100 -> 152 (newest).
	seedBackups(t, dir,
		"pre-migration-99-20260101T000000Z.sql.gz",
		"pre-migration-100-20260201T000000Z.sql.gz",
		"pre-migration-152-20260301T000000Z.sql.gz",
	)

	postgres.PruneOldBackupsForTest(dir, 1)

	left := listBackups(t, dir)
	want := []string{"pre-migration-152-20260301T000000Z.sql.gz"}
	if !slices.Equal(sortedCopy(left), want) {
		t.Fatalf("prune kept the wrong dump\n got: %v\nwant: %v", left, want)
	}
}

// TestPruneOldBackups_KeepsOneRestorePointPerVersion covers the boot-loop
// shape: the same migration is retried on every restart, so several dumps
// share one from_version. Retention must count distinct restore points, not
// files - otherwise three retries evict the backups that precede the earlier,
// genuinely different, schema versions.
func TestPruneOldBackups_KeepsOneRestorePointPerVersion(t *testing.T) {
	dir := t.TempDir()
	seedBackups(t, dir,
		"pre-migration-150-20260101T000000Z.sql.gz",
		"pre-migration-151-20260102T000000Z.sql.gz",
		"pre-migration-152-20260103T000000Z.sql.gz",
		"pre-migration-152-20260104T000000Z.sql.gz",
		"pre-migration-152-20260105T000000Z.sql.gz",
	)

	postgres.PruneOldBackupsForTest(dir, 3)

	want := []string{
		"pre-migration-150-20260101T000000Z.sql.gz",
		"pre-migration-151-20260102T000000Z.sql.gz",
		"pre-migration-152-20260105T000000Z.sql.gz", // newest snapshot of the retried step
	}
	if got := sortedCopy(listBackups(t, dir)); !slices.Equal(got, want) {
		t.Fatalf("prune lost a distinct restore point\n got: %v\nwant: %v", got, want)
	}
}

// TestPruneOldBackups_LeavesForeignFilesAlone: a dump the operator renamed or
// dropped in by hand is not ours to delete. Only files matching the exact
// name we write are prune candidates.
func TestPruneOldBackups_LeavesForeignFilesAlone(t *testing.T) {
	dir := t.TempDir()
	seedBackups(t, dir,
		"pre-migration-before-the-big-one.sql.gz",
		"pre-migration-150-20260101T000000Z.sql.gz",
		"pre-migration-151-20260102T000000Z.sql.gz",
	)

	postgres.PruneOldBackupsForTest(dir, 1)

	want := []string{
		"pre-migration-151-20260102T000000Z.sql.gz",
		"pre-migration-before-the-big-one.sql.gz",
	}
	if got := sortedCopy(listBackups(t, dir)); !slices.Equal(got, want) {
		t.Fatalf("prune touched a file it does not own\n got: %v\nwant: %v", got, want)
	}
}

func seedBackups(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func sortedCopy(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}

func listBackups(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "pre-migration-") {
			names = append(names, e.Name())
		}
	}
	return names
}
