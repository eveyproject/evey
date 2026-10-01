package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	appconfig "github.com/evey-project/evey/internal/app/config"
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

// failOnKey writes through the real repository but refuses one key, the way
// a database error would hit the second of several writes.
type failOnKey struct {
	*postgres.ServerConfigRepository
	key string
}

func (f failOnKey) Set(ctx context.Context, key, value string) error {
	if key == f.key {
		return errors.New("disk full")
	}
	return f.ServerConfigRepository.Set(ctx, key, value)
}

// A multi-key admin save is all or nothing: when one write fails, the ones
// already made in the same save are rolled back (SSO sign-ups were left
// switched on without the restrictions meant to go with them).
func TestServerConfig_SetManyIsAllOrNothing(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewServerConfigRepository(db)
	svc := appconfig.NewService(failOnKey{repo, "sso.z_allowed_domains"}, appconfig.WithUnitOfWork(postgres.NewTxUnit(db)))

	err := svc.SetMany(ctx, map[string]string{"sso.a_signup_enabled": "true", "sso.z_allowed_domains": "example.com"})
	require.Error(t, err)
	_, getErr := repo.Get(ctx, "sso.a_signup_enabled")
	require.Error(t, getErr, "the write made before the failure must be rolled back")

	ok := appconfig.NewService(repo, appconfig.WithUnitOfWork(postgres.NewTxUnit(db)))
	require.NoError(t, ok.SetMany(ctx, map[string]string{"sso.a_signup_enabled": "true", "sso.z_allowed_domains": "example.com"}))
	e, err := repo.Get(ctx, "sso.z_allowed_domains")
	require.NoError(t, err)
	assert.Equal(t, "example.com", e.Value)
}
