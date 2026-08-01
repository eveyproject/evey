package postgres

import (
	"compress/gzip"
	"context"
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
// empty, RunMigrations runs without taking a backup - convenient for
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
// pg_dump must be on PATH - fail-closed if missing rather than silently
// migrating without a safety net. The Dockerfile installs
// postgresql-client.
// pgDumpTimeout caps the pre-migration dump. Sized well above a real NAS run
// (minutes), so it only ever trips on a dump that is not progressing at all.
const pgDumpTimeout = 30 * time.Minute

func dumpDatabase(opts BackupOpts, fromVersion uint) (dest string, err error) {
	pgDump, err := exec.LookPath("pg_dump")
	if err != nil {
		return "", fmt.Errorf("pg_dump not found in PATH (install postgresql-client): %w", err)
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	name := fmt.Sprintf("%s%d-%s.sql.gz", backupFilePrefix, fromVersion, timestamp)
	dest = filepath.Join(opts.Dir, name)

	// Bounded, and generously: a pre-migration dump of a NAS-sized catalogue
	// legitimately takes minutes (the compose healthcheck allows 300s of boot
	// for exactly this), so the ceiling is far above any honest run. What it
	// buys is the dishonest one: pg_dump blocked on a Postgres lock or an
	// unreachable host used to hang the boot forever, with no crash, no error
	// and no log - an operator watching a server that simply never comes up.
	// Now it fails with a message naming the cause.
	ctx, cancel := context.WithTimeout(context.Background(), pgDumpTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, pgDump,
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
	// A deferred Close that ignores its error can silently lose data on
	// O_WRONLY handles - a failed fsync/flush at close time means the
	// on-disk dump is truncated but the caller sees "success". Surface
	// the Close error when no other error has superseded it, and remove
	// the (likely corrupt) partial file.
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			_ = os.Remove(dest)
			err = fmt.Errorf("close dump file: %w", cerr)
			dest = ""
		}
	}()

	if err = cmd.Start(); err != nil {
		_ = os.Remove(dest)
		return "", fmt.Errorf("start pg_dump: %w", err)
	}

	gz := gzip.NewWriter(out)
	if _, err = io.Copy(gz, stdout); err != nil {
		_ = cmd.Wait()
		_ = os.Remove(dest)
		return "", fmt.Errorf("gzip pg_dump output: %w", err)
	}
	if err = gz.Close(); err != nil {
		_ = cmd.Wait()
		_ = os.Remove(dest)
		return "", fmt.Errorf("close gzip: %w", err)
	}
	if err = cmd.Wait(); err != nil {
		_ = os.Remove(dest)
		// Name the timeout explicitly: "signal: killed" alone would send the
		// operator hunting for an OOM that never happened.
		if ctx.Err() != nil {
			return "", fmt.Errorf("pg_dump did not finish within %s and was stopped - the database may be holding a lock, or the host is unreachable (stderr: %s)", pgDumpTimeout, stderr.String())
		}
		return "", fmt.Errorf("pg_dump exit: %w (stderr: %s)", err, stderr.String())
	}
	return dest, nil
}

// pruneOldBackups keeps only the `keep` most recent pre-migration
// dumps under dir, deleting the rest. Sort order is filename - they
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
