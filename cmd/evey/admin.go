package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/eveyproject/evey/internal/adapters/driven/meilisearch"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// runAdmin dispatches `evey admin <subcommand> [flags]` to the right
// recovery handler. These commands run *instead of* booting the
// server - they're meant for cases where the operator can no longer
// log in or the DB needs a known-good restore.
func runAdmin(args []string) error {
	if len(args) < 1 {
		return adminUsage()
	}
	if isHelpArg(args[0]) {
		printAdminUsage(os.Stdout)
		return nil
	}
	cmd := findAdminCommand(args[0])
	if cmd == nil {
		fmt.Fprintf(os.Stderr, "unknown admin subcommand: %s\n\n", args[0])
		return adminUsage()
	}
	if cmd.noFlags && len(args) > 1 && isHelpArg(args[1]) {
		printAdminCommandHelp(os.Stdout, cmd)
		return nil
	}
	return cmd.run(args[1:])
}

// adminResetPassword generates a fresh password, hashes it via the
// existing UserService.ChangePassword path, and prints the new
// plaintext to stdout. Operator must capture stdout - the password
// isn't logged or stored anywhere else.
func adminResetPassword(args []string) error {
	fs := adminFlags("reset-password")
	email := fs.String("user", "", "user email (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		fs.Usage()
		return fmt.Errorf("--user is required")
	}

	cfg, db, cleanup, err := openAdminDB()
	if err != nil {
		return err
	}
	defer cleanup()
	_ = cfg

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userRepo := postgres.NewUserRepository(db)
	user, err := userRepo.GetByEmail(ctx, *email)
	if err != nil {
		return fmt.Errorf("lookup user %s: %w", *email, err)
	}

	newPassword, err := generateRandomPassword(24)
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	hashed, err := auth.BcryptHasher{}.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user.PasswordHash = hashed
	user.UpdatedAt = time.Now()
	if err := userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	// A reset is usually a response to a compromise: end the user's sessions
	// too, as Admin -> Users -> Disable does (refresh tokens + token epoch).
	if err := postgres.NewRefreshTokenRepository(db).DeleteByUserID(ctx, user.ID); err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}
	if err := userRepo.BumpTokenEpoch(ctx, user.ID); err != nil {
		return fmt.Errorf("revoke access tokens: %w", err)
	}

	fmt.Printf("user:     %s\n", *email)
	fmt.Printf("user_id:  %s\n", string(user.ID))
	fmt.Printf("password: %s\n", newPassword)
	fmt.Println()
	fmt.Println("Save this - it is the only time the new password is shown.")
	fmt.Println("Every session of this user has been ended.")
	fmt.Fprintf(os.Stderr, "[admin] reset password for user %s (%s)\n", *email, string(user.ID))
	return nil
}

// adminRotateJWTKey moves the on-disk RSA private key aside; the next server
// boot regenerates it via LoadOrGenerateRSAKey. Refresh tokens are checked by
// their hash in the database, not by the key, so they are deleted here, and
// every token epoch is bumped so live access tokens die within the epoch cache
// TTL instead of at the next restart.
func adminRotateJWTKey(args []string) error {
	fs := adminFlags("rotate-jwt-key")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	path := cfg.JWT.PrivateKeyPath
	if path == "" {
		return fmt.Errorf("EVEY_JWT_PRIVATE_KEY_PATH is empty")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[admin] no existing key at %s - server will generate one on next boot\n", path)
			return nil
		}
		return fmt.Errorf("stat key: %w", err)
	}
	backup := path + ".rotated-" + time.Now().UTC().Format("20060102T150405Z")
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("backup old key: %w", err)
	}
	fmt.Printf("Old key backed up to: %s\n", backup)
	fmt.Printf("New key will be generated on next server start.\n")
	if err := endAllSessions(); err != nil {
		return fmt.Errorf("key rotated, but sessions could not be ended (restart the server, then run this again): %w", err)
	}
	fmt.Println()
	fmt.Println("Every session has been ended - all users must sign in again.")
	fmt.Fprintf(os.Stderr, "[admin] rotated jwt key (old → %s)\n", backup)
	return nil
}

// endAllSessions deletes every refresh token and bumps every token epoch, in
// one transaction.
func endAllSessions() error {
	_, db, cleanup, err := openAdminDB()
	if err != nil {
		return err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM refresh_tokens`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET token_epoch = token_epoch + 1`); err != nil {
		return err
	}
	return tx.Commit()
}

// adminRestoreBackup feeds a pg_dump.gz file (as produced by the
// pre-migration backup) through gunzip + psql. The DB must exist and
// be reachable using the configured EVEY_DB_* env vars. Existing data
// in the target DB will be overwritten by the dump's CREATE/INSERT
// statements - call with care.
func adminRestoreBackup(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: evey admin restore-backup <path-to-dump.sql.gz>")
	}
	path := args[0]
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("stat dump: %w", err)
	}
	if !strings.HasSuffix(path, ".sql.gz") {
		return fmt.Errorf("expected a .sql.gz file (got %s)", filepath.Base(path))
	}

	// Verify the dump is whole BEFORE anything is dropped: a backup produced
	// with --clean replaces the schema, so a truncated or corrupt file would
	// drop the live tables and then fail to recreate them. verifyDumpComplete
	// refuses up front, leaving the database untouched.
	if err := verifyDumpComplete(path); err != nil {
		return fmt.Errorf("refusing to restore: %w", err)
	}

	psql, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql not in PATH: %w", err)
	}
	gunzip, err := exec.LookPath("gunzip")
	if err != nil {
		return fmt.Errorf("gunzip not in PATH: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Printf("Restoring %s into %s/%s as %s\n", path, cfg.DB.Host, cfg.DB.DBName, cfg.DB.User)
	fmt.Printf("This will OVERWRITE existing rows. Press Ctrl+C within 5s to abort.\n")
	time.Sleep(5 * time.Second)

	gzCmd := exec.Command(gunzip, "-c", path)
	psqlCmd := exec.Command(psql,
		"--host", cfg.DB.Host,
		"--port", fmt.Sprintf("%d", cfg.DB.Port),
		"--username", cfg.DB.User,
		"--dbname", cfg.DB.DBName,
		"--no-password",
		"--single-transaction",
		"--set", "ON_ERROR_STOP=1",
	)
	psqlCmd.Env = append(os.Environ(), "PGPASSWORD="+cfg.DB.Password)
	psqlCmd.Stdout = os.Stdout
	psqlCmd.Stderr = os.Stderr

	pipe, err := gzCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	psqlCmd.Stdin = pipe

	if err := gzCmd.Start(); err != nil {
		return fmt.Errorf("start gunzip: %w", err)
	}
	if err := psqlCmd.Start(); err != nil {
		_ = gzCmd.Process.Kill()
		return fmt.Errorf("start psql: %w", err)
	}
	// Wait on psql first. If it aborts (ON_ERROR_STOP) it stops reading, so
	// gunzip must be killed or it blocks forever writing to a dead pipe - the
	// 11-minute hang seen on 2026-10-08. On success psql has drained stdin, so
	// gunzip has already exited and the kill is a no-op.
	psqlErr := psqlCmd.Wait()
	_ = gzCmd.Process.Kill()
	gzErr := gzCmd.Wait()
	if psqlErr != nil {
		return fmt.Errorf("psql exit: %w", psqlErr)
	}
	if gzErr != nil {
		return fmt.Errorf("gunzip exit: %w", gzErr)
	}
	fmt.Fprintf(os.Stderr, "[admin] restored backup %s\n", path)
	return nil
}

// verifyDumpComplete decompresses the whole dump (catching a truncated or
// corrupt gzip) and checks for pg_dump's completion marker, which is written
// only at the end of a finished dump. Its absence means the file was cut short.
func verifyDumpComplete(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open dump: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("not valid gzip (corrupt?): %w", err)
	}
	defer gz.Close() //nolint:errcheck // read-only decompression reader; a close error is irrelevant
	marker := []byte("-- PostgreSQL database dump complete")
	tail := make([]byte, 0, 8192)
	buf := make([]byte, 32*1024)
	for {
		n, rerr := gz.Read(buf)
		if n > 0 {
			tail = append(tail, buf[:n]...)
			if len(tail) > 4096 {
				tail = tail[len(tail)-4096:]
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return fmt.Errorf("unreadable, likely truncated: %w", rerr)
		}
	}
	if !bytes.Contains(tail, marker) {
		return errors.New("no pg_dump completion marker: truncated or not a pg_dump")
	}
	return nil
}

// openAdminDB opens a short-lived DB connection for an admin command.
// It does NOT run migrations - the operator may be restoring from a
// backup of an older schema. Use sparingly: if the schema mismatches
// what the command writes, the failure is on the operator.
func openAdminDB() (*config.Config, *sql.DB, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load config: %w", err)
	}
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, nil, nil, fmt.Errorf("ping db: %w", err)
	}
	return cfg, db, func() { _ = db.Close() }, nil
}

// generateRandomPassword returns a base64-encoded random byte string of
// the given byte length. For passwords we use URL-safe base64 minus
// padding so the result is copy-pasteable without escaping.
func generateRandomPassword(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// adminReindexSearch rebuilds the Meilisearch index from the database without
// the server running - for when the index was lost/migrated (e.g. embedded↔
// external switch, version bump, or a DB restore that doesn't carry the
// index). Reads straight from Postgres, no metadata-provider calls.
func adminReindexSearch(args []string) error {
	fs := adminFlags("reindex-search")
	libraryID := fs.String("library", "", "library ID to reindex (default: all libraries)")
	purge := fs.Bool("purge", false, "clear the index first, then rebuild - removes stale/orphan docs an upsert can't (brief empty-search window)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, db, cleanup, err := openAdminDB()
	if err != nil {
		return err
	}
	defer cleanup()

	// Meili-aware: with the engine down, search is served by the Postgres ILIKE
	// fallback (a live query, no index to rebuild), so report and stop cleanly
	// rather than erroring on a missing engine.
	url := strings.TrimRight(cfg.Meilisearch.URL, "/")
	if url == "" {
		fmt.Println("Meilisearch not configured - search uses the Postgres fallback (ILIKE). Nothing to rebuild.")
		return nil
	}
	if code, _, herr := httpGet(url+"/health", "", 3*time.Second); herr != nil || code != 200 {
		fmt.Printf("Meilisearch unavailable at %s - search is served by the Postgres fallback (ILIKE), which needs no index. Nothing to rebuild.\n", url)
		return nil
	}

	search, err := meilisearch.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey)
	if err != nil {
		return fmt.Errorf("connect meilisearch (%s): %w", cfg.Meilisearch.URL, err)
	}
	if *purge {
		fmt.Println("Purging the index first (clearing stale docs) ...")
		if clearErr := search.Clear(context.Background()); clearErr != nil {
			return fmt.Errorf("purge index: %w", clearErr)
		}
	}
	items := postgres.NewMediaItemQueryRepoAdapter(db)
	translations := postgres.NewMediaItemTranslationRepository(db)
	cast := postgres.NewMediaPersonRepository(db)

	fmt.Printf("Reindexing media items into Meilisearch at %s ...\n", cfg.Meilisearch.URL)
	n, err := applibrary.ReindexSearchIndex(context.Background(), items, search, translations, cast, *libraryID, func(done, total int) {
		fmt.Printf("\r  %d/%d", done, total)
	})
	fmt.Println()
	if err != nil {
		return fmt.Errorf("reindex: %w", err)
	}
	fmt.Printf("Done. Indexed %d media items.\n", n)
	return nil
}

// adminMigrate dispatches `evey admin migrate <up|down|version>`. Same
// env-var contract as the server boot path.
func adminMigrate(args []string) error {
	if len(args) < 1 {
		return adminMigrateUsage()
	}
	cmd := args[0]
	noBackup := false
	for _, a := range args[1:] {
		if a == "--no-backup" {
			noBackup = true
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err = db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	backupOpts := postgres.BackupOpts{
		Dir:       cfg.Backup.Dir,
		Host:      cfg.DB.Host,
		Port:      cfg.DB.Port,
		User:      cfg.DB.User,
		Password:  cfg.DB.Password,
		DBName:    cfg.DB.DBName,
		Retention: 3,
	}
	if noBackup {
		backupOpts.Dir = ""
	}

	switch cmd {
	case "up":
		if err := postgres.RunMigrations(db, backupOpts); err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		if err := handOverToAppRole(cfg, db); err != nil {
			return err
		}
		fmt.Println("migrations: up - schema at latest version")
		return nil
	case "down":
		if err := postgres.RollbackLastMigration(db, backupOpts); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		if err := handOverToAppRole(cfg, db); err != nil {
			return err
		}
		fmt.Println("migrations: down - one step rolled back")
		return nil
	case "version":
		v, dirty, err := postgres.CurrentMigrationVersion(db)
		if err != nil {
			return fmt.Errorf("migrate version: %w", err)
		}
		if dirty {
			fmt.Printf("schema version: %d (DIRTY - previous migration failed)\n", v)
			return fmt.Errorf("schema is dirty")
		}
		fmt.Printf("schema version: %d\n", v)
		return nil
	default:
		return adminMigrateUsage()
	}
}

func adminMigrateUsage() error {
	fmt.Fprintf(os.Stderr, `usage: evey admin migrate <up|down|version> [--no-backup]

  up                Apply all pending migrations.
  down              Roll back exactly one migration step.
  version           Print current schema version (exit 1 if DB is in a dirty state).

  --no-backup       Skip the pre-migration pg_dump (use on hosts where
                    pg_dump isn't installed - typical WSL dev). Defaults
                    to taking the dump when EVEY_BACKUP_DIR is set.
`)
	return fmt.Errorf("admin migrate: subcommand required")
}

// adminReclassify wipes the per-title complexity row for one or every media
// item, which puts the items back into the worker's unclassified queue
// (the worker picks them up on its next tick - no separate trigger).
//
//	evey admin reclassify <media-id>     just this one
//	evey admin reclassify --all          everything
func adminReclassify(args []string) error {
	if len(args) < 1 {
		return adminReclassifyUsage()
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err = db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	if args[0] == "--all" {
		res, execErr := db.Exec(`UPDATE media_items
			SET complexity_class = NULL,
			    complexity_metrics = NULL,
			    complexity_computed_at = NULL
			WHERE complexity_class IS NOT NULL`)
		if execErr != nil {
			return fmt.Errorf("reclassify --all: %w", execErr)
		}
		n, _ := res.RowsAffected()
		fmt.Printf("reclassify: %d media items requeued; worker will pick them up\n", n)
		return nil
	}
	mediaID := args[0]
	res, err := db.Exec(`UPDATE media_items
		SET complexity_class = NULL,
		    complexity_metrics = NULL,
		    complexity_computed_at = NULL
		WHERE id = $1`, mediaID)
	if err != nil {
		return fmt.Errorf("reclassify %s: %w", mediaID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no media item with id %q", mediaID)
	}
	fmt.Printf("reclassify: media %s requeued\n", mediaID)
	return nil
}

func adminReclassifyUsage() error {
	fmt.Fprintf(os.Stderr, `usage: evey admin reclassify <media-id|--all>

  <media-id>        Wipe the complexity row for one item; worker will redo it.
  --all             Wipe every complexity row; the worker re-classifies the
                    whole library at its background pace (default ~5 items
                    per ~25s burst, 15 min idle between bursts).
`)
	return fmt.Errorf("admin reclassify: target required")
}
