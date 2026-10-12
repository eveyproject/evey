package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleRepo_Create_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	role := &user.Role{
		ID:          "role1",
		Name:        "editor",
		Description: "Can edit media",
		Permissions: []string{"media.edit", "media.view"},
		IsSystem:    false,
	}
	require.NoError(t, repo.Create(ctx, role))

	got, err := repo.GetByID(ctx, "role1")
	require.NoError(t, err)
	assert.Equal(t, "editor", got.Name)
	assert.Equal(t, []string{"media.edit", "media.view"}, got.Permissions)
}

func TestRoleRepo_GetByName(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	role := &user.Role{
		ID:          "role2",
		Name:        "viewer",
		Description: "Read only",
		Permissions: []string{"media.view"},
		IsSystem:    true,
	}
	require.NoError(t, repo.Create(ctx, role))

	got, err := repo.GetByName(ctx, "viewer")
	require.NoError(t, err)
	assert.Equal(t, role.ID, got.ID)
	assert.True(t, got.IsSystem)
}

func TestRoleRepo_List(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &user.Role{ID: "role3a", Name: "aaa", Permissions: []string{}}))
	require.NoError(t, repo.Create(ctx, &user.Role{ID: "role3b", Name: "bbb", Permissions: []string{}}))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 2)
}

func TestRoleRepo_GetByID_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewRoleRepository(db)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, "nonexistent")
	assert.Error(t, err)
}
