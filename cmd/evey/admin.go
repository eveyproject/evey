package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/evey-project/evey/internal/adapters/driven/meilisearch"
	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	applibrary "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/pkg/auth"
)

// runAdmin dispatches `evey admin <subcommand> [flags]` to the right
// recovery handler. These commands run *instead of* booting the
// server — they're meant for cases where the operator can no longer
// log in or the DB needs a known-good restore.
func runAdmin(args []string) error {
	if len(args) < 1 {
		return adminUsage()
	}
	switch args[0] {
	case "reset-password":
		return adminResetPassword(args[1:])
	case "rotate-jwt-key":
		return adminRotateJWTKey(args[1:])
	case "restore-backup":
		return adminRestoreBackup(args[1:])
	case "reindex-search":
		return adminReindexSearch(args[1:])
	case "-h", "--help", "help":
		_ = adminUsage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "unknown admin subcommand: %s\n\n", args[0])
		return adminUsage()
	}
}

func adminUsage() error {
	fmt.Fprintf(os.Stderr, `usage: evey admin <subcommand> [flags]

Recovery subcommands:
  reset-password --user <email>     Generate a new password for the user, print it once.
  rotate-jwt-key                    Regenerate the JWT signing key. Invalidates ALL active sessions.
  restore-backup <path>             Restore the database from a pg_dump.gz file.
  reindex-search [--library <id>]   Rebuild the Meilisearch index from the database (no metadata fetch).

Configuration is read from EVEY_* env vars (same as the server).
See docs/runbooks/recovery.md for full procedures.
`)
	return fmt.Errorf("admin: subcommand required")
}

// adminResetPassword generates a fresh password, hashes it via the
// existing UserService.ChangePassword path, and prints the new
// plaintext to stdout. Operator must capture stdout — the password
// isn't logged or stored anywhere else.
func adminResetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
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

	fmt.Printf("user:     %s\n", *email)
	fmt.Printf("user_id:  %s\n", string(user.ID))
	fmt.Printf("password: %s\n", newPassword)
	fmt.Println()
	fmt.Println("Save this — it is the only time the new password is shown.")
	fmt.Fprintf(os.Stderr, "[admin] reset password for user %s (%s)\n", *email, string(user.ID))
	return nil
}

// adminRotateJWTKey deletes the on-disk RSA private key. The next
// server boot regenerates it via LoadOrGenerateRSAKey. Old access
// tokens fail signature verification against the new public key →
// every session is logged out. Refresh tokens are JWTs too, so they're
// dead by the same mechanism (no separate DB cleanup needed).
func adminRotateJWTKey(args []string) error {
	fs := flag.NewFlagSet("rotate-jwt-key", flag.ExitOnError)
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
			fmt.Fprintf(os.Stderr, "[admin] no existing key at %s — server will generate one on next boot\n", path)
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
	fmt.Println()
	fmt.Println("WARNING: every active session is now invalid — all users must re-login.")
	fmt.Fprintf(os.Stderr, "[admin] rotated jwt key (old → %s)\n", backup)
	return nil
}

// adminRestoreBackup feeds a pg_dump.gz file (as produced by the
// pre-migration backup) through gunzip + psql. The DB must exist and
// be reachable using the configured EVEY_DB_* env vars. Existing data
// in the target DB will be overwritten by the dump's CREATE/INSERT
// statements — call with care.
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
	if err := gzCmd.Wait(); err != nil {
		_ = psqlCmd.Process.Kill()
		return fmt.Errorf("gunzip exit: %w", err)
	}
	if err := psqlCmd.Wait(); err != nil {
		return fmt.Errorf("psql exit: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[admin] restored backup %s\n", path)
	return nil
}

// openAdminDB opens a short-lived DB connection for an admin command.
// It does NOT run migrations — the operator may be restoring from a
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
// the server running — for when the index was lost/migrated (e.g. embedded↔
// external switch, version bump, or a DB restore that doesn't carry the
// index). Reads straight from Postgres, no metadata-provider calls.
func adminReindexSearch(args []string) error {
	fs := flag.NewFlagSet("reindex-search", flag.ExitOnError)
	libraryID := fs.String("library", "", "library ID to reindex (default: all libraries)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, db, cleanup, err := openAdminDB()
	if err != nil {
		return err
	}
	defer cleanup()

	search, err := meilisearch.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey)
	if err != nil {
		return fmt.Errorf("connect meilisearch (%s): %w", cfg.Meilisearch.URL, err)
	}
	items := postgres.NewMediaItemQueryRepoAdapter(db)

	fmt.Printf("Reindexing media items into Meilisearch at %s ...\n", cfg.Meilisearch.URL)
	n, err := applibrary.ReindexSearchIndex(context.Background(), items, search, *libraryID, func(done, total int) {
		fmt.Printf("\r  %d/%d", done, total)
	})
	fmt.Println()
	if err != nil {
		return fmt.Errorf("reindex: %w", err)
	}
	fmt.Printf("Done. Indexed %d media items.\n", n)
	return nil
}

