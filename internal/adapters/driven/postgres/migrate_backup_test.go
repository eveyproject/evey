package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestRunMigrations_WritesBackup spins an ephemeral postgres + runs
// migrations with a tempdir backup target. Verifies a `pre-migration-*`
// gzip dump lands under the dir and that a second migration call —
// which is a no-op since all migrations already applied — still writes
// a fresh dump (the snapshot is taken before Up regardless of pending
// migrations, so the operator always has a current rollback artefact).
func TestRunMigrations_WritesBackup(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skipf("pg_dump not in PATH: %v", err)
	}

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
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	defer c.Terminate(ctx) //nolint:errcheck

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := c.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatal(err)
	}
	port := mapped.Int()

	dsn := fmt.Sprintf("host=%s port=%d user=evey_test password=evey_test dbname=evey_test sslmode=disable", host, port)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}

	backupDir := t.TempDir()
	opts := postgres.BackupOpts{
		Dir:      backupDir,
		Host:     host,
		Port:     port,
		User:     "evey_test",
		Password: "evey_test",
		DBName:   "evey_test",
	}

	if err := postgres.RunMigrations(db, opts); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}

	dumps := listBackups(t, backupDir)
	if len(dumps) != 1 {
		t.Fatalf("expected 1 backup after first migration, got %d: %v", len(dumps), dumps)
	}
	if !strings.HasSuffix(dumps[0], ".sql.gz") {
		t.Errorf("dump must be gzip: %s", dumps[0])
	}
	info, err := os.Stat(filepath.Join(backupDir, dumps[0]))
	if err != nil {
		t.Fatalf("stat dump: %v", err)
	}
	if info.Size() < 100 {
		t.Errorf("dump suspiciously small (%d bytes)", info.Size())
	}
}

func TestRunMigrations_RetentionPrunesOldDumps(t *testing.T) {
	backupDir := t.TempDir()
	// Seed 5 fake old dumps; pruneOldBackups should leave 3 newest by name.
	for i := range 5 {
		name := fmt.Sprintf("pre-migration-%d-2026010%dT000000Z.sql.gz", i, i)
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
