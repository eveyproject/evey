package postgres

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
	cmd := exec.CommandContext(ctx, "psql",
		"-h", d.host,
		"-p", strconv.Itoa(d.port),
		"-U", d.user,
		"-d", d.dbName,
		"--single-transaction",
	)
	cmd.Env = d.pgEnv()
	cmd.Stdin = data
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}
