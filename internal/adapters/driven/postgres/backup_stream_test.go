package postgres_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
)

// fakePsql installs a `psql` on PATH that appends its stdin to out and only
// writes out+".eof" once it read stdin to the end, the point where the real
// psql commits its --single-transaction. Builtins only, so a kill leaves no
// child process holding the pipes.
func fakePsql(t *testing.T) (out string) {
	t.Helper()
	dir := t.TempDir()
	out = filepath.Join(dir, "received.sql")
	script := "#!/bin/sh\n" +
		"while IFS= read -r l; do printf '%s\\n' \"$l\" >> \"" + out + "\"; done\n" +
		"echo done > \"" + out + ".eof\"\n"
	if err := os.WriteFile(filepath.Join(dir, "psql"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return out
}

func waitForContent(path, want string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), want) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// feedDump writes head, waits for psql to have received it, then writes tail: a
// restore that buffers the whole dump before starting psql never gets there.
func feedDump(t *testing.T, out, head, tail string) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		_, _ = io.WriteString(pw, head)
		if !waitForContent(out, "PostgreSQL database dump", 3*time.Second) {
			t.Errorf("psql received nothing before the end of the dump: the restore is not streamed")
		}
		_, _ = io.WriteString(pw, tail)
		_ = pw.Close()
	}()
	return pr
}

func TestRestoreTables_StreamsIntoPsql(t *testing.T) {
	out := fakePsql(t)
	d := postgres.NewDatabaseDumper("localhost", 5432, "user", "pass", "db", "disable")
	head := "-- PostgreSQL database dump\nSET statement_timeout = 0;\n"
	tail := "COPY public.t (id) FROM stdin;\n1\n\\.\n"

	if err := d.RestoreTables(context.Background(), feedDump(t, out, head, tail)); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ := os.ReadFile(out)
	if string(got) != head+tail {
		t.Fatalf("psql received %q, want %q", got, head+tail)
	}
	if _, err := os.Stat(out + ".eof"); err != nil {
		t.Fatalf("psql never reached the end of its input: %v", err)
	}
}

// A line rejected after psql already received part of the dump must kill
// psql before it sees EOF, so --single-transaction never commits the prefix.
func TestRestoreTables_RejectionMidStreamNeverReachesEOF(t *testing.T) {
	out := fakePsql(t)
	d := postgres.NewDatabaseDumper("localhost", 5432, "user", "pass", "db", "disable")
	head := "-- PostgreSQL database dump\nSET statement_timeout = 0;\n"
	tail := "COPY public.t FROM PROGRAM 'id';\n"

	err := d.RestoreTables(context.Background(), feedDump(t, out, head, tail))
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected rejection, got: %v", err)
	}
	if b, _ := os.ReadFile(out); strings.Contains(string(b), "PROGRAM") {
		t.Fatalf("the rejected line reached psql: %q", b)
	}
	if _, err := os.Stat(out + ".eof"); err == nil {
		t.Fatal("psql read its input to the end: the rejected restore would have committed")
	}
}
