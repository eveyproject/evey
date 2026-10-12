package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// A key migration 000154 seeds as a default must not silence an operator's
// own key: env beats the built-in default and beats an empty row, an admin's
// stored key beats everything, the built-in is the last resort.
func TestResolveAPIKeyWithBuiltin(t *testing.T) {
	ctx := context.Background()
	const dbKey = "api.tmdb_key"
	const env = "EVEY_TMDB_API_KEY"

	t.Run("admin key in db beats env and builtin", func(t *testing.T) {
		t.Setenv(env, "from-env")
		repo := fakes.NewFakeServerConfigRepository()
		_ = repo.Set(ctx, dbKey, "admin-key")
		if got := resolveAPIKeyWithBuiltin(ctx, repo, dbKey, env, builtinTMDBKey); got != "admin-key" {
			t.Fatalf("want admin-key, got %q", got)
		}
	})

	t.Run("env beats the seeded builtin and replaces it in db", func(t *testing.T) {
		t.Setenv(env, "from-env")
		repo := fakes.NewFakeServerConfigRepository()
		_ = repo.Set(ctx, dbKey, builtinTMDBKey) // what the migration leaves
		if got := resolveAPIKeyWithBuiltin(ctx, repo, dbKey, env, builtinTMDBKey); got != "from-env" {
			t.Fatalf("want from-env, got %q", got)
		}
		if e, err := repo.Get(ctx, dbKey); err != nil || e.Value != "from-env" {
			t.Fatalf("env key must be seeded over the builtin, got %+v %v", e, err)
		}
	})

	t.Run("builtin serves when nothing else is set", func(t *testing.T) {
		t.Setenv(env, "")
		repo := fakes.NewFakeServerConfigRepository()
		_ = repo.Set(ctx, dbKey, builtinTMDBKey)
		if got := resolveAPIKeyWithBuiltin(ctx, repo, dbKey, env, builtinTMDBKey); got != builtinTMDBKey {
			t.Fatalf("want the builtin, got %q", got)
		}
	})

	t.Run("no builtin behaves like resolveAPIKey", func(t *testing.T) {
		t.Setenv(env, "from-env")
		repo := fakes.NewFakeServerConfigRepository()
		_ = repo.Set(ctx, dbKey, "")
		if got := resolveAPIKey(ctx, repo, dbKey, env); got != "from-env" {
			t.Fatalf("want from-env over an empty row, got %q", got)
		}
	})
}

// The Go constant and the migration must carry the same token, or the server
// would treat the shipped default as an admin's own key (and env seeding of a
// real key would go inert).
func TestBuiltinTMDBKeyMatchesMigration(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "adapters", "driven", "postgres", "migrations", "000154_api_keys_defaults.up.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if !strings.Contains(string(body), "('api.tmdb_key', '"+builtinTMDBKey+"', NOW())") {
		t.Fatal("migration 000154 does not seed api.tmdb_key with builtinTMDBKey - keep the two identical")
	}
	for _, k := range []string{"api.omdb_key", "api.opensubtitles_key", "api.tidb_key"} {
		if !strings.Contains(string(body), "('"+k+"', '', NOW())") {
			t.Errorf("migration 000154 must create %s as an empty row so the panel lists it", k)
		}
	}
}
