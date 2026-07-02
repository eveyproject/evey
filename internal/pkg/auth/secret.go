package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrGenerateSecret loads a hex-encoded symmetric secret of n bytes from
// path, generating and persisting one (0600) on first use — the same
// bootstrap-on-first-run model as the JWT key. Used for the HLS AES key
// derivation secret (per-media keys are HMAC(secret, mediaID)). A loose-perm
// file is tightened to 0600, matching LoadOrGenerateRSAKey.
func LoadOrGenerateSecret(path string, n int) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		secret, decErr := hex.DecodeString(strings.TrimSpace(string(data)))
		if decErr != nil || len(secret) != n {
			return nil, fmt.Errorf("secret at %s is corrupted (want %d hex-encoded bytes)", path, n)
		}
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm()&0077 != 0 {
			slog.Warn("secret has loose permissions, fixing to 0600", slog.String("path", path))
			if err2 := os.Chmod(path, 0600); err2 != nil {
				return nil, fmt.Errorf("cannot fix secret permissions: %w", err2)
			}
		}
		return secret, nil
	}

	if dir := filepath.Dir(path); dir != "" {
		if err2 := os.MkdirAll(dir, 0700); err2 != nil {
			return nil, fmt.Errorf("cannot create secret directory %s: %w", dir, err2)
		}
	}
	slog.Info("generating symmetric secret", slog.String("path", path), slog.Int("bytes", n))
	secret := make([]byte, n)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(secret)), 0600); err != nil {
		return nil, fmt.Errorf("cannot save secret to %s: %w", path, err)
	}
	return secret, nil
}
