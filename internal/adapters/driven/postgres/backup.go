package postgres

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/lib/pq"

	"github.com/evey-project/evey/internal/domain/backup"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

var pgDumpHeader = []byte("-- PostgreSQL database dump")

// Backups are produced by DumpTables as a whole-database `pg_dump --data-only
// --no-owner --no-privileges --disable-triggers`, prefixed with one TRUNCATE of
// every table so a restore replaces the data on a populated base. That output
// has a small, fixed grammar,
// so the restore validator is an ALLOWLIST (every non-data line must match one
// of these forms) rather than a blocklist - a blocklist was provably evadable
// (CREATE EXTENSION, escaped tokens, multi-line) and, crucially, missed psql
// meta-command RCE such as `\!` and `\copy ... from program`. Anything outside
// this grammar is rejected, so the restore can never carry a server-side
// program/file COPY, function/DO body, or shell escape regardless of the DB
// role's privileges. Every token is closed on its own line (no `;`, `\`, `--`,
// `/*` or unbalanced quote inside a value): psql runs EVERY `;`-separated
// statement of a line and carries an open quote over to the next one.
const (
	dumpIdent = `(?:[a-z_][a-z0-9_]*|"[^"\\;]+")`
	dumpQName = dumpIdent + `(?:\.` + dumpIdent + `)*`
	dumpValue = `(?:-?[a-z0-9_.]+|'[^'\\;]*')`
)

var (
	reDumpSet         = regexp.MustCompile(`(?i)^SET\s+([a-z_]+)\s*(?:=|TO)\s*` + dumpValue + `(?:\s*,\s*` + dumpValue + `)*;$`)
	reDumpSessionAuth = regexp.MustCompile(`(?i)^SET\s+SESSION\s+AUTHORIZATION\s+DEFAULT;$`)
	reDumpSetConfig   = regexp.MustCompile(`(?i)^SELECT\s+pg_catalog\.set_config\('[a-z_]+',\s*'[^'\\;]*',\s*(?:true|false)\);$`)
	// pg_dump restores sequence counters with setval in a whole-database data-only
	// dump. The sequence name is a quoted (optionally schema-qualified) identifier
	// with no quote/backslash/semicolon, the value a number, the is_called flag an
	// optional bool - no room to smuggle SQL.
	reDumpSetval      = regexp.MustCompile(`(?i)^SELECT\s+pg_catalog\.setval\('[^'\\;]+',\s*-?[0-9]+(?:,\s*(?:true|false))?\s*\);$`)
	reDumpAlterTrig   = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+(?:ONLY\s+)?` + dumpQName + `\s+(?:DISABLE|ENABLE)\s+TRIGGER\s+ALL;$`)
	reDumpTruncate    = regexp.MustCompile(`(?i)^TRUNCATE\s+(?:TABLE\s+)?(?:ONLY\s+)?` + dumpQName + `(?:\s*,\s*` + dumpQName + `)*;$`)
	reDumpCopyStdin   = regexp.MustCompile(`(?i)^COPY\s+` + dumpQName + `(?:\s*\(\s*` + dumpIdent + `(?:\s*,\s*` + dumpIdent + `)*\s*\))?\s+FROM\s+stdin;$`)
	reDumpRestrict    = regexp.MustCompile(`^\\(restrict|unrestrict)\s+\S+$`)
)

// isIdentitySetting names the GUCs that switch the session identity: `SET role
// = x` is the same privilege change as the already rejected `SET ROLE x`.
func isIdentitySetting(name string) bool {
	n := strings.ToLower(name)
	return n == "role" || n == "session_authorization"
}

// allowedDumpLine reports whether a single non-COPY-data line of a data-only
// dump is part of the safe grammar. The caller handles COPY data separately.
func allowedDumpLine(line string) bool {
	t := strings.TrimSpace(line)
	switch {
	case t == "" || strings.HasPrefix(t, "--"): // blank / comment
		return true
	case strings.HasPrefix(t, `\`): // psql meta: only \restrict/\unrestrict (reject \!, \copy, \g, \i, …)
		return reDumpRestrict.MatchString(t)
	case reDumpSessionAuth.MatchString(t):
		return true
	case reDumpSetConfig.MatchString(t):
		return true
	case reDumpSetval.MatchString(t):
		return true
	case reDumpSet.MatchString(t):
		return !isIdentitySetting(reDumpSet.FindStringSubmatch(t)[1])
	case reDumpAlterTrig.MatchString(t):
		return true
	case reDumpTruncate.MatchString(t):
		return true
	case reDumpCopyStdin.MatchString(t):
		return true
	default:
		return false
	}
}

type DatabaseDumper struct {
	host     string
	port     int
	user     string
	password string
	dbName   string
	sslMode  string
}

func NewDatabaseDumper(host string, port int, user, password, dbName, sslMode string) *DatabaseDumper {
	return &DatabaseDumper{
		host: host, port: port,
		user: user, password: password,
		dbName: dbName, sslMode: sslMode,
	}
}

// pgEnv returns env with PGPASSWORD set so the password never appears in /proc/PID/cmdline.
func (d *DatabaseDumper) pgEnv() []string {
	return append(os.Environ(),
		"PGPASSWORD="+d.password,
		"PGSSLMODE="+d.sslMode,
	)
}

// DumpTables streams a whole-database, data-only dump prefixed with a single
// TRUNCATE of every table it covers, so a restore REPLACES the data on a
// populated database instead of colliding on primary keys. The tables argument
// is ignored: a static list silently drops any table a later migration adds,
// losing it on restore (and leaving dangling references), so the dump is taken
// over every base table discovered in the schema except schema_migrations
// (owned by the migrator). The TRUNCATE lists all those tables in ONE statement
// - no CASCADE needed, since every referencing table is in the set - and
// --disable-triggers lets the COPYs reload rows through the circular FK on
// media_items. Still data-only: the restore validator's SQL allowlist holds.
func (d *DatabaseDumper) DumpTables(ctx context.Context, _ []string) (io.ReadCloser, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup.DumpTables")
	defer span.End()

	tables, err := d.allTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("DumpTables: list tables: %w", err)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("DumpTables: no tables to back up")
	}
	quoted := make([]string, len(tables))
	for i, t := range tables {
		quoted[i] = `public.` + pq.QuoteIdentifier(t)
	}
	truncate := "TRUNCATE " + strings.Join(quoted, ", ") + ";\n"

	args := []string{
		"-h", d.host,
		"-p", strconv.Itoa(d.port),
		"-U", d.user,
		"-d", d.dbName,
		"--data-only", "--no-owner", "--no-privileges", "--disable-triggers",
		"--exclude-table", "schema_migrations",
	}

	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	cmd.Env = d.pgEnv()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pg_dump stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("pg_dump start: %s: %w", strings.TrimSpace(stderr.String()), err)
	}

	pr, pw := io.Pipe()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic in pg_dump copy goroutine", slog.Any("recover", r))
				pw.CloseWithError(fmt.Errorf("pg_dump copy panic: %v", r))
			}
		}()
		// The TRUNCATE goes first so the restore clears the tables before the
		// COPYs refill them.
		if _, werr := io.WriteString(pw, truncate); werr != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			pw.CloseWithError(werr)
			return
		}
		_, copyErr := io.Copy(pw, stdout)
		waitErr := cmd.Wait()
		switch {
		case copyErr != nil:
			pw.CloseWithError(copyErr)
		case waitErr != nil:
			pw.CloseWithError(fmt.Errorf("pg_dump: %s: %w", strings.TrimSpace(stderr.String()), waitErr))
		default:
			pw.CloseWithError(nil)
		}
	}()

	return pr, nil
}

// allTables lists every base table in the public schema except schema_migrations
// (the migrator owns it; restoring it from a data-only dump would desync the
// migration bookkeeping). Opens its own short-lived connection, like the other
// dumper queries.
func (d *DatabaseDumper) allTables(ctx context.Context) ([]string, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.host, d.port, d.user, d.password, d.dbName, d.sslMode)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("allTables: open db: %w", err)
	}
	defer db.Close()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations' ORDER BY tablename`)
	if err != nil {
		return nil, fmt.Errorf("allTables: query: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("allTables: scan: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d *DatabaseDumper) RestoreTables(ctx context.Context, data io.Reader) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup.RestoreTables")
	defer span.End()
	validated, err := d.validateBackup(data)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "psql",
		"-h", d.host,
		"-p", strconv.Itoa(d.port),
		"-U", d.user,
		"-d", d.dbName,
		"--single-transaction",
		// ON_ERROR_STOP makes psql exit non-zero on the first SQL error. Without
		// it, --single-transaction still rolls the restore back on error, but
		// psql exits 0 - so the caller would report a successful restore for a
		// restore that actually applied nothing (false success).
		"-v", "ON_ERROR_STOP=1",
	)
	cmd.Env = d.pgEnv()
	cmd.Stdin = bytes.NewReader(validated)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

// validateBackup reads the entire backup, checks the pg_dump header, and
// allowlist-validates every line outside of COPY data blocks. COPY data (the
// rows streamed between `COPY … FROM stdin;` and a lone `\.`) is opaque and
// passed through verbatim; any non-data line that is not part of the safe
// data-only grammar (see allowedDumpLine) is rejected.
func (d *DatabaseDumper) validateBackup(data io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(data)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var buf bytes.Buffer
	lineNum := 0
	headerFound := false
	inCopyData := false

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if inCopyData {
			// Opaque COPY rows. psql ends the block on a line that is exactly
			// `\.`; rows can't contain a bare `\.` (it would be escaped), so this
			// matches psql's own framing and can't be used to smuggle SQL.
			if strings.TrimRight(line, "\r") == `\.` {
				inCopyData = false
			}
			buf.WriteString(line)
			buf.WriteByte('\n')
			continue
		}

		trimmed := strings.TrimRight(line, "\r")
		if !headerFound && strings.Contains(trimmed, string(pgDumpHeader)) {
			headerFound = true
		}
		if lineNum == 50 && !headerFound {
			return nil, errors.New("restore: invalid file: missing pg_dump header")
		}

		if !allowedDumpLine(trimmed) {
			return nil, fmt.Errorf("restore: rejected: disallowed statement at line %d", lineNum)
		}
		if reDumpCopyStdin.MatchString(strings.TrimSpace(trimmed)) {
			inCopyData = true
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("restore: scan backup: %w", err)
	}
	if lineNum == 0 {
		return nil, errors.New("restore: invalid file: empty backup")
	}
	if !headerFound {
		return nil, errors.New("restore: invalid file: missing pg_dump header")
	}

	return buf.Bytes(), nil
}

// CurrentSchemaVersion queries schema_migrations to return the current migration
// version as a string. Returns an error when the schema is in a dirty state
// (a previous migration failed halfway), since neither creating nor restoring
// a backup into an inconsistent schema is safe.
func (d *DatabaseDumper) CurrentSchemaVersion(ctx context.Context) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup.CurrentSchemaVersion")
	defer span.End()

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.host, d.port, d.user, d.password, d.dbName, d.sslMode)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return "", fmt.Errorf("CurrentSchemaVersion: open db: %w", err)
	}
	defer db.Close()

	// Bounded like every other repository here: this runs from the backup
	// scheduler on the process-root context, which has no deadline, and it
	// opens its OWN throwaway connection outside the pool - so a database that
	// stops answering would park the scheduler goroutine on it indefinitely,
	// and the ticker would stack another one every minute (NextRunAt only
	// advances once CreateBackup returns).
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var version int64
	var dirty bool
	row := db.QueryRowContext(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`)
	if scanErr := row.Scan(&version, &dirty); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			// No migrations have been applied yet.
			return "0", nil
		}
		return "", fmt.Errorf("CurrentSchemaVersion: query schema_migrations: %w", scanErr)
	}
	if dirty {
		return "", fmt.Errorf("CurrentSchemaVersion: schema_migrations is in dirty state at version %d; resolve the failed migration before operating on backups", version)
	}
	return strconv.FormatInt(version, 10), nil
}

var _ backup.DatabaseDumper = (*DatabaseDumper)(nil)
