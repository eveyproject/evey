package filesystem_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/filesystem"
)

// MusicScanner.ComputeHash delegates to filehash.Compute - asserts a stable
// hash for fixed content and that different content produces different hashes.
func TestMusicScanner_ComputeHash_StableAndUnique(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.bin")
	b := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(a, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("different content"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &filesystem.MusicScanner{}
	hA1, err := s.ComputeHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hA2, err := s.ComputeHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hB, err := s.ComputeHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if hA1 == "" || hA1 != hA2 {
		t.Fatalf("same file should yield same hash: %q vs %q", hA1, hA2)
	}
	if hA1 == hB {
		t.Fatalf("different files should have different hashes: both %q", hA1)
	}
}

// ComputeHash on a missing file returns an error.
func TestMusicScanner_ComputeHash_MissingFile(t *testing.T) {
	t.Parallel()
	s := &filesystem.MusicScanner{}
	if _, err := s.ComputeHash("/nonexistent/path/f.bin"); err == nil {
		t.Fatal("expected error on missing file")
	}
}

// PhotoScanner.ComputeHash delegates to the same filehash implementation.
func TestPhotoScanner_ComputeHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "img.bin")
	if err := os.WriteFile(p, []byte("png-like-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := filesystem.NewPhotoScanner()
	h, err := s.ComputeHash(p)
	if err != nil {
		t.Fatal(err)
	}
	if h == "" {
		t.Fatal("expected non-empty hash")
	}
}
