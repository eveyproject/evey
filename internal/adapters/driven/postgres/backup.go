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
	"time"

	"github.com/lib/pq"

	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/domain/backup"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

var pgDumpHeader = []byte("-- PostgreSQL database dump")

// telemetryTables are the high-volume, regenerable observability tables excluded
// from a backup unless the operator opts in. They are still TRUNCATEd on restore
// (the TRUNCATE lists every table) but not reloaded, so a restore empties them -
// acceptable for metrics, and it keeps routine backups small.
var telemetryTables = []string{"playback_qoe_events", "server_metrics"}

// Backups are produced by DumpAll as a whole-database `pg_dump --data-only
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
	reDumpSetval    = regexp.MustCompile(`(?i)^SELECT\s+pg_catalog\.setval\('[^'\\;]+',\s*-?[0-9]+(?:,\s*(?:true|false))?\s*\);$`)
	reDumpAlterTrig = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+(?:ONLY\s+)?` + dumpQName + `\s+(?:DISABLE|ENABLE)\s+TRIGGER\s+ALL;$`)
	reDumpTruncate  = regexp.MustCompile(`(?i)^TRUNCATE\s+(?:TABLE\s+)?(?:ONLY\s+)?` + dumpQName + `(?:\s*,\s*` + dumpQName + `)*;$`)
	reDumpCopyStdin = regexp.MustCompile(`(?i)^COPY\s+` + dumpQName + `(?:\s*\(\s*` + dumpIdent + `(?:\s*,\s*` + dumpIdent + `)*\s*\))?\s+FROM\s+stdin;$`)
	reDumpRestrict  = regexp.MustCompile(`^\\(restrict|unrestrict)\s+\S+$`)
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

// dsn is the dumper's own connection string, escaped like the server's
// (config.DBConfig.DSN): a password with a space or a quote broke the raw
// Sprintf it used, and with it every backup, manual, scheduled and CLI.
func (d *DatabaseDumper) dsn() string {
	return config.DBConfig{Host: d.host, Port: d.port, User: d.user, Password: d.password, DBName: d.dbName, SSLMode: d.sslMode}.DSN()
}

// pgEnv returns env with PGPASSWORD set so the password never appears in /proc/PID/cmdline.
func (d *DatabaseDumper) pgEnv() []string {
	return append(os.Environ(),
		"PGPASSWORD="+d.password,
		"PGSSLMODE="+d.sslMode,
	)
}

// DumpAll streams a whole-database, data-only dump prefixed with a single
// TRUNCATE of every table, so a restore REPLACES the data on a populated
// database instead of colliding on primary keys. The dump covers every base
// table discovered in the schema except schema_migrations (owned by the
// migrator) - a static list would silently drop any table a later migration
// adds, losing it on restore. The TRUNCATE lists all tables in ONE statement
// (no CASCADE needed, since every referencing table is in the set) and
// --disable-triggers lets the COPYs reload through the circular FK on
// media_items. When includeTelemetry is false the telemetry tables are still
// TRUNCATEd but their data is not dumped (--exclude-table), so a restore leaves
// them empty and routine backups stay lean. Still data-only: the restore
// validator's SQL allowlist holds.
func (d *DatabaseDumper) DumpAll(ctx context.Context, includeTelemetry bool) (io.ReadCloser, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup.DumpAll")
	defer span.End()

	tables, err := d.allTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("DumpAll: list tables: %w", err)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("DumpAll: no tables to back up")
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
	if !includeTelemetry {
		for _, t := range telemetryTables {
			args = append(args, "--exclude-table-data", t)
		}
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
	dsn := d.dsn()
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

// RestoreTables validates the dump line by line while streaming it into psql,
// so memory stays bounded whatever the dump size. psql only starts once the
// pg_dump header is seen, and a rejected line kills it before its stdin closes:
// --single-transaction commits on EOF, so a killed psql commits nothing.
func (d *DatabaseDumper) RestoreTables(ctx context.Context, data io.Reader) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup.RestoreTables")
	defer span.End()

	p := &psqlRestore{ctx: ctx, d: d}
	if err := validateBackup(data, p); err != nil {
		p.abort()
		return err
	}
	return p.finish()
}

// psqlRestore is the validated-dump sink: it starts psql lazily and records
// the first start or write failure instead of returning it, so the validator
// still reaches a rejected line deterministically however fast psql fails.
type psqlRestore struct {
	ctx    context.Context
	d      *DatabaseDumper
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	buf    *bufio.Writer
	stderr bytes.Buffer
	err    error
}

func (p *psqlRestore) start() error {
	d := p.d
	cmd := exec.CommandContext(p.ctx, "psql",
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
	cmd.Stderr = &p.stderr
	cmd.WaitDelay = 10 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("psql stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("psql start: %w", err)
	}
	p.cmd, p.stdin = cmd, stdin
	p.buf = bufio.NewWriterSize(stdin, 256*1024)
	return nil
}

func (p *psqlRestore) Write(b []byte) (int, error) {
	if p.err != nil {
		return len(b), nil
	}
	if p.cmd == nil {
		if p.err = p.start(); p.err != nil {
			return len(b), nil
		}
	}
	if _, err := p.buf.Write(b); err != nil {
		p.err = err
	}
	return len(b), nil
}

// Flush pushes the buffered lines to psql.
func (p *psqlRestore) Flush() {
	if p.err == nil && p.buf != nil {
		p.err = p.buf.Flush()
	}
}

// abort kills psql before closing its stdin: the kill must land first or
// psql would read EOF and commit the prefix it already received.
func (p *psqlRestore) abort() {
	if p.cmd == nil {
		return
	}
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
}

func (p *psqlRestore) finish() error {
	if p.cmd == nil {
		if p.err != nil {
			return fmt.Errorf("psql restore: %w", p.err)
		}
		return errors.New("psql restore: nothing to restore")
	}
	p.Flush()
	if p.err != nil {
		p.abort()
		return fmt.Errorf("psql restore: %s: %w", strings.TrimSpace(p.stderr.String()), p.err)
	}
	if err := p.stdin.Close(); err != nil {
		p.abort()
		return fmt.Errorf("psql restore: close stdin: %w", err)
	}
	if err := p.cmd.Wait(); err != nil {
		return fmt.Errorf("psql restore: %s: %w", strings.TrimSpace(p.stderr.String()), err)
	}
	return nil
}

// validateBackup streams the backup into out, allowlist-validating every line
// outside COPY data (see allowedDumpLine); COPY rows pass through verbatim.
// Lines before the pg_dump header are held back (at most 50), so out receives
// nothing from a headerless file.
func validateBackup(data io.Reader, out *psqlRestore) error {
	scanner := bufio.NewScanner(data)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var pending bytes.Buffer
	emit := func(line string) {
		if pending.Len() > 0 {
			_, _ = out.Write(pending.Bytes())
			pending.Reset()
		}
		_, _ = out.Write([]byte(line + "\n"))
	}
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
			emit(line)
			continue
		}

		trimmed := strings.TrimRight(line, "\r")
		if !headerFound && strings.Contains(trimmed, string(pgDumpHeader)) {
			headerFound = true
		}
		if lineNum == 50 && !headerFound {
			return errors.New("restore: invalid file: missing pg_dump header")
		}

		if !allowedDumpLine(trimmed) {
			return fmt.Errorf("restore: rejected: disallowed statement at line %d", lineNum)
		}
		if reDumpCopyStdin.MatchString(strings.TrimSpace(trimmed)) {
			inCopyData = true
		}

		if !headerFound {
			pending.WriteString(line)
			pending.WriteByte('\n')
			continue
		}
		wasStarted := out.cmd != nil
		emit(line)
		if !wasStarted {
			out.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("restore: scan backup: %w", err)
	}
	if lineNum == 0 {
		return errors.New("restore: invalid file: empty backup")
	}
	if !headerFound {
		return errors.New("restore: invalid file: missing pg_dump header")
	}
	return nil
}

// CurrentSchemaVersion queries schema_migrations to return the current migration
// version as a string. Returns an error when the schema is in a dirty state
// (a previous migration failed halfway), since neither creating nor restoring
// a backup into an inconsistent schema is safe.
func (d *DatabaseDumper) CurrentSchemaVersion(ctx context.Context) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup.CurrentSchemaVersion")
	defer span.End()

	dsn := d.dsn()
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
