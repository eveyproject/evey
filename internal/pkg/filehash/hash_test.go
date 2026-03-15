package filehash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompute_SmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := Compute(path)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}

	// same content → same hash
	path2 := filepath.Join(dir, "small2.txt")
	if err := os.WriteFile(path2, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	h2, err := Compute(path2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("same content should produce same hash: %s != %s", h1, h2)
	}

	// different content → different hash
	path3 := filepath.Join(dir, "diff.txt")
	if err := os.WriteFile(path3, []byte("different"), 0644); err != nil {
		t.Fatal(err)
	}
	h3, err := Compute(path3)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h3 {
		t.Fatal("different content should produce different hash")
	}
}

func TestCompute_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	// 256KB file (larger than 128KB threshold)
	data := make([]byte, 256*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := Compute(path)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}

	// modify middle (not in head/tail) - hash should NOT change since we only read head+tail
	data[128*1024] = 0xFF
	path2 := filepath.Join(dir, "large_mid.bin")
	if err := os.WriteFile(path2, data, 0644); err != nil {
		t.Fatal(err)
	}
	h2, err := Compute(path2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatal("middle-only change should not affect partial hash")
	}

	// modify first byte - hash SHOULD change
	data[0] = 0xFF
	path3 := filepath.Join(dir, "large_head.bin")
	if err := os.WriteFile(path3, data, 0644); err != nil {
		t.Fatal(err)
	}
	h3, err := Compute(path3)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h3 {
		t.Fatal("head change should affect hash")
	}
}

func TestCompute_DifferentSizeSameContent(t *testing.T) {
	dir := t.TempDir()

	p1 := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(p1, make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}
	p2 := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(p2, make([]byte, 200), 0644); err != nil {
		t.Fatal(err)
	}

	h1, _ := Compute(p1)
	h2, _ := Compute(p2)
	if h1 == h2 {
		t.Fatal("different sizes should produce different hashes")
	}
}

func TestCompute_NonexistentFile(t *testing.T) {
	_, err := Compute("/nonexistent/file.bin")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
