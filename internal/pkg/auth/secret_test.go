package auth

import (
	"bytes"
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

// The HLS AES secret derives every per-media decryption key. A file left
// group- or world-readable therefore hands the whole encrypted catalogue to
// anyone with a shell on the box - which is exactly why the loader tightens it
// on sight rather than merely warning.
//
// Mutation testing (2026-08-01) showed nothing exercised that branch: negating
// the permission check survived the suite, so the tightening could have been
// removed - or silently stopped working - without a single test noticing.
func TestLoadOrGenerateSecret_TightensLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hls.key")

	original, err := LoadOrGenerateSecret(path, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Somebody (a restore, a bad umask, a hand copy) makes it world-readable.
	if chmodErr := os.Chmod(path, 0o644); chmodErr != nil {
		t.Fatalf("chmod: %v", chmodErr)
	}

	loaded, err := LoadOrGenerateSecret(path, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(loaded, original) {
		t.Error("the secret changed: tightening permissions must not rotate the key, or every encrypted stream breaks")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %#o, want 0600 - the AES secret is still readable by others", perm)
	}
}

// An already-tight file must be left exactly as it is.
func TestLoadOrGenerateSecret_LeavesTightPermissionsAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hls.key")
	if _, err := LoadOrGenerateSecret(path, 32); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Chmod(path, 0o400); err != nil { // read-only, still not group/world
		t.Fatalf("chmod: %v", err)
	}
	if _, err := LoadOrGenerateSecret(path, 32); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o400 {
		t.Errorf("permissions = %#o, want 0400 untouched - only group/world bits justify a rewrite", perm)
	}
}
