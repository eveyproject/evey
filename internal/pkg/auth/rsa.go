package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

func LoadOrGenerateRSAKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("jwt key at %s is corrupted (invalid PEM)", path)
		}
		var key *rsa.PrivateKey
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("jwt key at %s: %w", path, err)
		}
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm()&0077 != 0 {
			slog.Warn("jwt key has loose permissions, fixing to 0600", slog.String("path", path), slog.String("mode", fmt.Sprintf("%04o", info.Mode().Perm())))
			if err2 := os.Chmod(path, 0600); err2 != nil {
				return nil, fmt.Errorf("cannot fix jwt key permissions: %w", err2)
			}
		}
		return key, nil
	}

	if dir := filepath.Dir(path); dir != "" {
		if err2 := os.MkdirAll(dir, 0700); err2 != nil {
			return nil, fmt.Errorf("cannot create jwt key directory %s: %w", dir, err2)
		}
	}

	slog.Info("generating RSA key", slog.String("path", path))
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot save jwt key to %s: %w", path, err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("encode jwt key: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close jwt key: %w", err)
	}
	return key, nil
}
