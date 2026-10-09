package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGz(t *testing.T, dir, name string, body []byte, closeGz bool) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(body); err != nil {
		t.Fatal(err)
	}
	if closeGz {
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestVerifyDumpComplete(t *testing.T) {
	dir := t.TempDir()
	full := "--\n-- PostgreSQL database dump\n--\nCREATE TABLE x(id int);\n--\n-- PostgreSQL database dump complete\n--\n"

	ok := writeGz(t, dir, "ok.sql.gz", []byte(full), true)
	if err := verifyDumpComplete(ok); err != nil {
		t.Errorf("complete dump rejected: %v", err)
	}

	truncated := writeGz(t, dir, "truncated.sql.gz", []byte("--\n-- PostgreSQL database dump\n--\nCREATE TABLE x(id int);\n"), true)
	if err := verifyDumpComplete(truncated); err == nil {
		t.Error("truncated dump (no completion marker) was accepted")
	} else if !strings.Contains(err.Error(), "completion marker") {
		t.Errorf("unexpected error for truncated dump: %v", err)
	}

	// A gzip stream cut off mid-way: write without closing, then chop the trailer.
	raw := writeGz(t, dir, "corrupt.sql.gz", bytes.Repeat([]byte(full), 50), false)
	b, _ := os.ReadFile(raw)
	if err := os.WriteFile(raw, b[:len(b)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyDumpComplete(raw); err == nil {
		t.Error("corrupt/truncated gzip was accepted")
	}
}
