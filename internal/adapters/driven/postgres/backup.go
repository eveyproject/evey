package postgres

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var (
	pgDumpHeader    = []byte("-- PostgreSQL database dump")
	dangerousLineRe = regexp.MustCompile(`(?i)(COPY\s+.*\s+TO\s+PROGRAM|CREATE\s+.*\s+LANGUAGE|DO\s+\$\$|EXECUTE|pg_read_file|pg_write_file|lo_import|lo_export)`)
)

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
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pg_dump: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return &out, nil
}

func (d *DatabaseDumper) RestoreTables(ctx context.Context, data io.Reader) error {
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

// validateBackup reads the entire backup, checks the header and scans every line for dangerous SQL.
func (d *DatabaseDumper) validateBackup(data io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(data)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var buf bytes.Buffer
	lineNum := 0
	headerFound := false

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if !headerFound && strings.Contains(line, string(pgDumpHeader)) {
			headerFound = true
		}
		if lineNum == 50 && !headerFound {
			return nil, fmt.Errorf("restore: invalid file: missing pg_dump header")
		}

		if dangerousLineRe.MatchString(line) {
			return nil, fmt.Errorf("restore: rejected: dangerous SQL at line %d", lineNum)
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("restore: scan backup: %w", err)
	}
	if lineNum == 0 {
		return nil, fmt.Errorf("restore: invalid file: empty backup")
	}
	if !headerFound {
		return nil, fmt.Errorf("restore: invalid file: missing pg_dump header")
	}

	return buf.Bytes(), nil
}
