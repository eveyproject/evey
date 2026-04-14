package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerConfigRepo_Set_Get(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewServerConfigRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "test.key", "test-value"))

	got, err := repo.Get(ctx, "test.key")
	require.NoError(t, err)
	assert.Equal(t, "test.key", got.Key)
	assert.Equal(t, "test-value", got.Value)
	assert.False(t, got.UpdatedAt.IsZero())
}

func TestServerConfigRepo_Set_Upsert(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewServerConfigRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "upsert.key", "original"))
	require.NoError(t, repo.Set(ctx, "upsert.key", "updated"))

	got, err := repo.Get(ctx, "upsert.key")
	require.NoError(t, err)
	assert.Equal(t, "updated", got.Value)
}

func TestServerConfigRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewServerConfigRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "delete.key", "value"))
	require.NoError(t, repo.Delete(ctx, "delete.key"))

	_, err := repo.Get(ctx, "delete.key")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestServerConfigRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewServerConfigRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Set(ctx, "list.c", "val-c"))
	require.NoError(t, repo.Set(ctx, "list.a", "val-a"))
	require.NoError(t, repo.Set(ctx, "list.b", "val-b"))

	list, err := repo.List(ctx)
	require.NoError(t, err)

	var keys []string
	for _, e := range list {
		if e.Key == "list.a" || e.Key == "list.b" || e.Key == "list.c" {
			keys = append(keys, e.Key)
		}
	}
	require.Len(t, keys, 3)
	assert.Equal(t, []string{"list.a", "list.b", "list.c"}, keys)
}
