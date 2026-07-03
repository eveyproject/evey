package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerateSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "hls_aes.key")

	// First call generates + persists.
	s1, err := LoadOrGenerateSecret(path, 32)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(s1) != 32 {
		t.Fatalf("len = %d, want 32", len(s1))
	}
	if info, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("secret file not written: %v", statErr)
	} else if info.Mode().Perm() != 0600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}

	// Second call loads the SAME bytes (deterministic key derivation depends on it).
	s2, err := LoadOrGenerateSecret(path, 32)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if string(s1) != string(s2) {
		t.Error("reloaded secret differs from the generated one")
	}
}

func TestLoadOrGenerateSecret_CorruptRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hls_aes.key")
	if err := os.WriteFile(path, []byte("not-hex"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrGenerateSecret(path, 32); err == nil {
		t.Error("expected an error for a corrupt secret file")
	}
}
