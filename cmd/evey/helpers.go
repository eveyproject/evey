package main

import (
	"context"
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

	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appsubtitle "github.com/gravity-zero/evey/internal/app/subtitle"
	domainshared "github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
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
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		return nil, fmt.Errorf("encode jwt key: %w", err)
	}
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

type libMediaGetter struct {
	q *applibrary.QueryService
}

func (g *libMediaGetter) GetMediaItem(ctx context.Context, id string) (*appsubtitle.MediaItemInfo, error) {
	item, err := g.q.GetMediaItem(ctx, id)
	if err != nil {
		return nil, err
	}
	return &appsubtitle.MediaItemInfo{
		ID:       item.ID,
		FilePath: item.FilePath,
		Title:    item.Title,
	}, nil
}

// verify interface at compile time
var _ appsubtitle.MediaItemGetter = (*libMediaGetter)(nil)

// isWithinDir reports whether path is strictly inside dir (both cleaned).
func isWithinDir(dir, path string) bool {
	cleanDir := filepath.Clean(dir) + string(filepath.Separator)
	return strings.HasPrefix(filepath.Clean(path), cleanDir)
}

// safeJoin joins base and rel, then verifies the result is strictly inside base.
// Uses filepath.Abs to resolve any .. components before comparison.
func safeJoin(base, rel string) (string, error) {
	joined := filepath.Join(base, rel)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	if abs != baseAbs && !strings.HasPrefix(abs, baseAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal detected")
	}
	return abs, nil
}

// extractBearerToken reads the Bearer token from Authorization header or ?token query param.
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return h[7:]
	}
	return r.URL.Query().Get("token")
}

// requireAdmin is a middleware that enforces authenticated admin access.
func requireAdmin(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := tokenGen.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !claims.IsAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// userMaxStreamsAdapter implements UserMaxStreamsGetter using the user repository.
type userMaxStreamsAdapter struct {
	repo interface {
		GetByID(ctx context.Context, id string) (*domainuser.User, error)
	}
}

func (a *userMaxStreamsAdapter) GetMaxStreams(ctx context.Context, userID string) *int {
	u, err := a.repo.GetByID(ctx, userID)
	if err != nil {
		return nil
	}
	return u.MaxStreams
}

// serverConfigStore wraps ServerConfigRepository to implement remoteAccessPersister.
type serverConfigStore struct {
	repo domainshared.ServerConfigRepository
}

func (s *serverConfigStore) Get(ctx context.Context, key string) (string, error) {
	e, err := s.repo.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return e.Value, nil
}

func (s *serverConfigStore) Set(ctx context.Context, key, value string) error {
	return s.repo.Set(ctx, key, value)
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

