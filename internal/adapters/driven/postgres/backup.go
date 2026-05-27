package postgres

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/evey-project/evey/internal/domain/backup"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

var pgDumpHeader = []byte("-- PostgreSQL database dump")

// Backups are produced by DumpTables as `pg_dump --data-only --no-owner
// --no-privileges --disable-triggers`. That output has a small, fixed grammar,
// so the restore validator is an ALLOWLIST (every non-data line must match one
// of these forms) rather than a blocklist — a blocklist was provably evadable
// (CREATE EXTENSION, escaped tokens, multi-line) and, crucially, missed psql
// meta-command RCE such as `\!` and `\copy ... from program`. Anything outside
// this grammar is rejected, so the restore can never carry a server-side
// program/file COPY, function/DO body, or shell escape regardless of the DB
// role's privileges.
var (
	reDumpSet         = regexp.MustCompile(`(?i)^SET\s+[a-z_]+\s*(=|TO)\s*.+;$`)
	reDumpSessionAuth = regexp.MustCompile(`(?i)^SET\s+SESSION\s+AUTHORIZATION\s+DEFAULT;$`)
	reDumpSetConfig   = regexp.MustCompile(`(?i)^SELECT\s+pg_catalog\.set_config\('[a-z_]+',\s*'[^']*',\s*(true|false)\);$`)
	reDumpAlterTrig   = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+(ONLY\s+)?[a-z0-9_."]+\s+(DISABLE|ENABLE)\s+TRIGGER\s+ALL;$`)
	reDumpTruncate    = regexp.MustCompile(`(?i)^TRUNCATE\s+(TABLE\s+)?[a-z0-9_."][a-z0-9_.,"\s]*;$`)
	reDumpCopyStdin   = regexp.MustCompile(`(?i)^COPY\s+[a-z0-9_."]+\s*\([^)]*\)\s+FROM\s+stdin;$`)
	reDumpRestrict    = regexp.MustCompile(`^\\(restrict|unrestrict)\s+\S+$`)
)

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
	case reDumpSet.MatchString(t):
		return true
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

func (d *DatabaseDumper) DumpTables(ctx context.Context, tables []string) (io.Reader, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.backup.DumpTables")
	defer span.End()
	args := []string{
		"-h", d.host,
		"-p", strconv.Itoa(d.port),
		"-U", d.user,
		"-d", d.dbName,
		"--data-only", "--no-owner", "--no-privileges", "--disable-triggers",
	}
	for _, t := range tables {
		args = append(args, "--table", t)
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
		// Recover from any panic in io.Copy and surface it on the pipe so a
		// reader blocked on pr.Read() wakes up with an error instead of
		// deadlocking forever when the goroutine dies.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in pg_dump copy goroutine", slog.Any("recover", r))
				pw.CloseWithError(fmt.Errorf("pg_dump copy panic: %v", r))
			}
		}()
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

var _ backup.DatabaseDumper = (*DatabaseDumper)(nil)
