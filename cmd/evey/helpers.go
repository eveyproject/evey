package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeFilenameChars = regexp.MustCompile(`[^\w\s.\-()]`)

func jsonEncodeResponse(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"message":"encode error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func loadOrGenerateRSAKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("jwt key at %s is corrupted (invalid PEM)", path)
		}
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("jwt key at %s: %w", path, err)
		}
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0077 != 0 {
			slog.Warn("jwt key has loose permissions, fixing to 0600", slog.String("path", path), slog.String("mode", fmt.Sprintf("%04o", info.Mode().Perm())))
			if err := os.Chmod(path, 0600); err != nil {
				return nil, fmt.Errorf("cannot fix jwt key permissions: %w", err)
			}
		}
		return key, nil
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("cannot create jwt key directory %s: %w", dir, err)
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
	defer f.Close()
	pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return key, nil
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\"", "")
	name = unsafeFilenameChars.ReplaceAllString(name, "_")
	if name == "" {
		name = "download"
	}
	return name
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request, baseURL string) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	allowed := strings.TrimRight(baseURL, "/")
	if strings.TrimRight(origin, "/") == allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}
