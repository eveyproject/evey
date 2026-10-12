package main

import (
	"context"
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// resolveFlag resolution order: server_config DB key wins → env fallback, with
// both the dotted key and the raw env-var name accepted as DB keys.
func TestResolveFlag(t *testing.T) {
	ctx := context.Background()
	const dotted = "transcoding.keyframe_backfill"
	const env = "EVEY_KEYFRAME_BACKFILL"

	t.Run("db dotted key wins over env", func(t *testing.T) {
		t.Setenv(env, "off")
		repo := fakes.NewFakeServerConfigRepository()
		_ = repo.Set(ctx, dotted, "on")
		if got := resolveFlag(ctx, repo, dotted, env); got != "on" {
			t.Fatalf("want on (db wins over env), got %q", got)
		}
	})

	t.Run("raw env-name accepted as db key alias", func(t *testing.T) {
		t.Setenv(env, "")
		repo := fakes.NewFakeServerConfigRepository()
		_ = repo.Set(ctx, env, "on") // admin typed the raw EVEY_ name in the panel
		if got := resolveFlag(ctx, repo, dotted, env); got != "on" {
			t.Fatalf("want on (env-name alias), got %q", got)
		}
	})

	t.Run("env fallback when db has no row", func(t *testing.T) {
		t.Setenv(env, "on")
		repo := fakes.NewFakeServerConfigRepository()
		if got := resolveFlag(ctx, repo, dotted, env); got != "on" {
			t.Fatalf("want on (env fallback), got %q", got)
		}
	})

	t.Run("nil repo falls back to env", func(t *testing.T) {
		t.Setenv(env, "on")
		if got := resolveFlag(ctx, nil, dotted, env); got != "on" {
			t.Fatalf("want on (nil repo → env), got %q", got)
		}
	})

	t.Run("repo error falls back to env (never fails closed on a db blip)", func(t *testing.T) {
		t.Setenv(env, "on")
		repo := fakes.NewFakeServerConfigRepository()
		repo.Err = errors.New("db down")
		if got := resolveFlag(ctx, repo, dotted, env); got != "on" {
			t.Fatalf("want on (repo err → env), got %q", got)
		}
	})

	t.Run("unset everywhere is empty", func(t *testing.T) {
		t.Setenv(env, "")
		repo := fakes.NewFakeServerConfigRepository()
		if got := resolveFlag(ctx, repo, dotted, env); got != "" {
			t.Fatalf("want empty, got %q", got)
		}
	})
}
