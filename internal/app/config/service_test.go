package config

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_Set_and_List(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)
	ctx := context.Background()

	require.NoError(t, svc.Set(ctx, "theme", "dark"))
	require.NoError(t, svc.Set(ctx, "lang", "fr"))

	entries, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	keys := map[string]string{}
	for _, e := range entries {
		keys[e.Key] = e.Value
	}
	assert.Equal(t, "dark", keys["theme"])
	assert.Equal(t, "fr", keys["lang"])
}

func TestService_Set_overwrite(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)
	ctx := context.Background()

	require.NoError(t, svc.Set(ctx, "k", "v1"))
	require.NoError(t, svc.Set(ctx, "k", "v2"))

	entries, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "v2", entries[0].Value)
}

func TestService_Delete(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)
	ctx := context.Background()

	require.NoError(t, svc.Set(ctx, "k", "v"))
	require.NoError(t, svc.Delete(ctx, "k"))

	entries, err := svc.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestService_Delete_nonexistent(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)

	err := svc.Delete(context.Background(), "nope")
	assert.NoError(t, err)
}

func TestService_List_empty(t *testing.T) {
	repo := fakes.NewFakeServerConfigRepository()
	svc := NewService(repo)

	entries, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestService_repo_error(t *testing.T) {
	repoErr := errors.New("db down")
	repo := fakes.NewFakeServerConfigRepository()
	repo.Err = repoErr
	svc := NewService(repo)
	ctx := context.Background()

	t.Run("Set", func(t *testing.T) {
		assert.ErrorIs(t, svc.Set(ctx, "k", "v"), repoErr)
	})
	t.Run("Delete", func(t *testing.T) {
		assert.ErrorIs(t, svc.Delete(ctx, "k"), repoErr)
	})
	t.Run("List", func(t *testing.T) {
		_, err := svc.List(ctx)
		assert.ErrorIs(t, err, repoErr)
	})
}
