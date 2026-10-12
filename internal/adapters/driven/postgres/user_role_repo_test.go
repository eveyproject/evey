package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRoleRepo_AssignRole_GetRolesByUserID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	roleRepo := postgres.NewRoleRepository(db)
	urRepo := postgres.NewUserRoleRepository(db)
	ctx := context.Background()

	u := newTestUser("ur-u1")
	require.NoError(t, userRepo.Create(ctx, u))

	role := &user.Role{ID: "ur-r1", Name: "admin-role", Permissions: []string{"*"}}
	require.NoError(t, roleRepo.Create(ctx, role))

	require.NoError(t, urRepo.AssignRole(ctx, shared.UserID(u.ID), role.ID))

	roles, err := urRepo.GetRolesByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Len(t, roles, 1)
	assert.Equal(t, "admin-role", roles[0].Name)
}

func TestUserRoleRepo_RemoveRole(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	roleRepo := postgres.NewRoleRepository(db)
	urRepo := postgres.NewUserRoleRepository(db)
	ctx := context.Background()

	u := newTestUser("ur-u2")
	require.NoError(t, userRepo.Create(ctx, u))

	role := &user.Role{ID: "ur-r2", Name: "temp-role", Permissions: []string{}}
	require.NoError(t, roleRepo.Create(ctx, role))

	require.NoError(t, urRepo.AssignRole(ctx, shared.UserID(u.ID), role.ID))
	require.NoError(t, urRepo.RemoveRole(ctx, shared.UserID(u.ID), role.ID))

	roles, err := urRepo.GetRolesByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Empty(t, roles)
}

func TestUserRoleRepo_AssignRole_Idempotent(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	roleRepo := postgres.NewRoleRepository(db)
	urRepo := postgres.NewUserRoleRepository(db)
	ctx := context.Background()

	u := newTestUser("ur-u3")
	require.NoError(t, userRepo.Create(ctx, u))

	role := &user.Role{ID: "ur-r3", Name: "dup-role", Permissions: []string{}}
	require.NoError(t, roleRepo.Create(ctx, role))

	require.NoError(t, urRepo.AssignRole(ctx, shared.UserID(u.ID), role.ID))
	require.NoError(t, urRepo.AssignRole(ctx, shared.UserID(u.ID), role.ID))

	roles, err := urRepo.GetRolesByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Len(t, roles, 1)
}
