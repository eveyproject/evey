package postgres

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	pgDumpHeader      = []byte("-- PostgreSQL database dump")
	copyToProgramRe   = regexp.MustCompile(`(?i)COPY\s+.*\s+TO\s+PROGRAM`)
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
	header := make([]byte, 128)
	n, err := io.ReadFull(data, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("restore: read header: %w", err)
	}
	header = header[:n]
	if !bytes.HasPrefix(header, pgDumpHeader) {
		return fmt.Errorf("restore: invalid file: missing pg_dump header")
	}

	peek := make([]byte, 8192)
	pn, peekErr := io.ReadFull(data, peek)
	if peekErr != nil && peekErr != io.ErrUnexpectedEOF && peekErr != io.EOF {
		return fmt.Errorf("restore: read peek: %w", peekErr)
	}
	peek = peek[:pn]
	combined := slices.Concat(header, peek)
	if copyToProgramRe.Match(combined) {
		return fmt.Errorf("restore: rejected: dangerous COPY TO PROGRAM statement")
	}

	safe := io.MultiReader(bytes.NewReader(combined), data)

	cmd := exec.CommandContext(ctx, "psql",
		"-h", d.host,
		"-p", strconv.Itoa(d.port),
		"-U", d.user,
		"-d", d.dbName,
		"--single-transaction",
	)
	cmd.Env = d.pgEnv()
	cmd.Stdin = safe
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}
