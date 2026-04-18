package postgres

import (
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupOpts configures pre-migration database backups. When Dir is
// empty, RunMigrations runs without taking a backup — convenient for
// tests where the container is ephemeral. Production callers should set
// Dir + the connection params so a rollback dump exists if Up() fails.
type BackupOpts struct {
	Dir       string
	Host      string
	Port      int
	User      string
	Password  string
	DBName    string
	Retention int // keep N most recent dumps, default 3
}

const defaultBackupRetention = 3

const backupFilePrefix = "pre-migration-"

// dumpDatabase runs pg_dump and gzips the output to a file under
// opts.Dir named by the schema_migrations version we're about to leave.
// Returns the path of the written file. Caller is expected to log this
// path on migration failure so the operator knows what to restore from.
//
// pg_dump must be on PATH — fail-closed if missing rather than silently
// migrating without a safety net. The Dockerfile installs
// postgresql-client.
func dumpDatabase(opts BackupOpts, fromVersion uint) (string, error) {
	pgDump, err := exec.LookPath("pg_dump")
	if err != nil {
		return "", fmt.Errorf("pg_dump not found in PATH (install postgresql-client): %w", err)
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	name := fmt.Sprintf("%s%d-%s.sql.gz", backupFilePrefix, fromVersion, timestamp)
	dest := filepath.Join(opts.Dir, name)

	cmd := exec.Command(pgDump,
		"--host", opts.Host,
		"--port", fmt.Sprintf("%d", opts.Port),
		"--username", opts.User,
		"--dbname", opts.DBName,
		"--no-password",
		"--format=plain",
		"--no-owner",
		"--no-privileges",
	)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+opts.Password)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("pg_dump stdout pipe: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create dump file: %w", err)
	}
	defer out.Close() //nolint:errcheck

	if err := cmd.Start(); err != nil {
		os.Remove(dest)
		return "", fmt.Errorf("start pg_dump: %w", err)
	}

	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, stdout); err != nil {
		_ = cmd.Wait()
		os.Remove(dest)
		return "", fmt.Errorf("gzip pg_dump output: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = cmd.Wait()
		os.Remove(dest)
		return "", fmt.Errorf("close gzip: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		os.Remove(dest)
		return "", fmt.Errorf("pg_dump exit: %w (stderr: %s)", err, stderr.String())
	}
	return dest, nil
}

// pruneOldBackups keeps only the `keep` most recent pre-migration
// dumps under dir, deleting the rest. Sort order is filename — they
// embed the migration version + UTC timestamp so lexicographic sort ==
// chronological. Errors are logged, not returned: a failed prune
// shouldn't block a successful migration.
func pruneOldBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("prune: read backup dir", slog.String("dir", dir), slog.Any("err", err))
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), backupFilePrefix) {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return
	}
	sort.Strings(names)
	toDelete := names[:len(names)-keep]
	for _, n := range toDelete {
		path := filepath.Join(dir, n)
		if err := os.Remove(path); err != nil {
			slog.Warn("prune: remove old backup", slog.String("path", path), slog.Any("err", err))
		}
	}
}
