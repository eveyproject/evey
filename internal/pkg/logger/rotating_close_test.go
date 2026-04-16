package logger

import (
	"os"
	"path/filepath"
	"testing"
)

// rotatingWriter.Close closes the current file and allows subsequent writes
// to reopen a new file.
func TestRotatingWriter_Close(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w := newRotatingWriter(dir, "evey", 10)

	// Write to open a file
	_, err := w.Write([]byte("hello\n"))
	if err != nil {
		t.Fatal(err)
	}

	// Close returns no error when file open
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Calling Close again is a no-op (current is now nil)
	if err := w.Close(); err != nil {
		t.Fatalf("second Close should be no-op: %v", err)
	}

	// File must exist on disk
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected at least one .log file written before close")
	}
}
