//go:build linux

package loadtest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The real evictor must accept any readable file (no privileges involved) and
// refuse a path that doesn't exist - a silent success there would report a
// "cold cache" run that never cooled anything.
func TestEvictFromCache_RealFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 1<<20), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.ReadFile(path); err != nil { // pull it through the page cache
		t.Fatalf("read: %v", err)
	}
	if err := evictFromCache(path); err != nil {
		t.Fatalf("evictFromCache on a plain readable file: %v", err)
	}
	if err := evictFromCache(filepath.Join(t.TempDir(), "missing.mkv")); err == nil {
		t.Fatalf("evicting a missing file must fail, not pretend")
	}
}
