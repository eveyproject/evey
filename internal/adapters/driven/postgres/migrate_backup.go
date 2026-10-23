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
	"regexp"
	"slices"
	"sort"
	"strconv"
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

const (
	backupFilePrefix = "pre-migration-"
	backupFileSuffix = ".sql.gz"
	// backupTimestampLayout is the fixed-width UTC stamp dumpDatabase puts in
	// every filename. Fixed width is the point: it is the only part of the
	// name that can be compared.
	backupTimestampLayout = "20060102T150405Z"
)

// backupNameRe matches exactly what dumpDatabase writes. Anything else under
// the prefix was put there by the operator and is not ours to delete.
var backupNameRe = regexp.MustCompile(
	`^` + regexp.QuoteMeta(backupFilePrefix) + `(\d+)-(\d{8}T\d{6}Z)` + regexp.QuoteMeta(backupFileSuffix) + `$`)

// pgDumpTimeout caps the pre-migration dump. Sized well above a real NAS run
// (minutes), so it only ever trips on a dump that is not progressing at all.
const pgDumpTimeout = 30 * time.Minute

// dumpDatabase runs pg_dump and gzips the output to a file under
// opts.Dir named by the schema_migrations version we're about to leave.
// Returns the path of the written file. Caller is expected to log this
// path on migration failure so the operator knows what to restore from.
//
// pg_dump must be on PATH - fail-closed if missing rather than silently
// migrating without a safety net. The Dockerfile installs
// postgresql-client.
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
	// G204: pgDump is the absolute path exec.LookPath resolved for the fixed
	// name "pg_dump"; every argument below is a literal or a config value the
	// operator already controls.
	cmd := exec.CommandContext(ctx, pgDump, //nolint:gosec
		"--host", opts.Host,
		"--port", fmt.Sprintf("%d", opts.Port),
		"--username", opts.User,
		"--dbname", opts.DBName,
		"--no-password",
		"--format=plain",
		"--no-owner",
		"--no-privileges",
		// --clean --if-exists: restore-backup replays this onto a populated
		// database under --single-transaction + ON_ERROR_STOP; without the DROPs
		// the first CREATE TABLE fails "already exists" and everything rolls back.
		"--clean",
		"--if-exists",
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

// backupFile is one dump under the backup dir, dated by the UTC stamp its
// name carries (or, for a file we did not write, by its mtime).
type backupFile struct {
	name    string
	version uint64
	at      time.Time
	owned   bool // written by dumpDatabase, and therefore ours to prune
}

// scanBackupDir lists the dumps under dir, OLDEST FIRST.
//
// Ordering follows the timestamp, never the filename. The version comes
// first in the name and is not zero-padded, so a lexical sort puts
// "pre-migration-99-" after "pre-migration-152-" and ranks the oldest dump
// as the newest - which, in a retention routine, means deleting the backup
// the operator would have restored. Single ranking for the whole codebase:
// the CLI advisor and the retention policy used to disagree, one sorting by
// mtime and one by name.
func scanBackupDir(dir string) ([]backupFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []backupFile
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, backupFilePrefix) || !strings.HasSuffix(n, backupFileSuffix) {
			continue
		}
		bf := backupFile{name: n}
		if m := backupNameRe.FindStringSubmatch(n); m != nil {
			v, vErr := strconv.ParseUint(m[1], 10, 64)
			ts, tErr := time.Parse(backupTimestampLayout, m[2])
			if vErr == nil && tErr == nil {
				bf.version, bf.at, bf.owned = v, ts, true
			}
		}
		if !bf.owned {
			info, iErr := e.Info()
			if iErr != nil {
				continue
			}
			bf.at = info.ModTime()
		}
		out = append(out, bf)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].at.Equal(out[j].at) {
			return out[i].name < out[j].name
		}
		return out[i].at.Before(out[j].at)
	})
	return out, nil
}

// NewestBackup returns the path of the most recent pre-migration dump under
// dir, or "" when there is none. Used by the `admin rollback` advisor to
// name the artefact to restore.
func NewestBackup(dir string) string {
	if dir == "" {
		return ""
	}
	files, err := scanBackupDir(dir)
	if err != nil || len(files) == 0 {
		return ""
	}
	return filepath.Join(dir, files[len(files)-1].name)
}

// pruneOldBackups keeps one dump per restore point - the newest snapshot of
// each of the `keep` most recent schema versions - and deletes the rest.
//
// Counting distinct versions rather than files is what survives a crash
// loop. A server retrying the same failing migration writes dumps that all
// restore to the SAME version; with a file count, three restarts evict the
// backups of the earlier, genuinely different versions - including the one
// taken before the migration that broke things. Files we did not write are
// left alone.
//
// Errors are logged, not returned: a failed prune shouldn't block a
// successful migration.
func pruneOldBackups(dir string, keep int) {
	files, err := scanBackupDir(dir)
	if err != nil {
		slog.Warn("prune: read backup dir", slog.String("dir", dir), slog.Any("err", err))
		return
	}
	kept := map[uint64]bool{}
	for _, f := range slices.Backward(files) { // newest first
		if !f.owned {
			continue
		}
		if !kept[f.version] && len(kept) < keep {
			kept[f.version] = true
			continue
		}
		path := filepath.Join(dir, f.name)
		if rmErr := os.Remove(path); rmErr != nil {
			slog.Warn("prune: remove old backup", slog.String("path", path), slog.Any("err", rmErr))
		}
	}
}
