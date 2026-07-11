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

func TestLibraryAccessRepo_Grant_GetByUserID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("la-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Grant(ctx, shared.UserID(u.ID), shared.LibraryID(lib.ID)))

	ids, err := repo.GetByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Contains(t, ids, lib.ID)
}

func TestLibraryAccessRepo_GrantMany(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-gm-u")
	require.NoError(t, userRepo.Create(ctx, u))
	lib1 := newTestLibrary("la-gm-1")
	lib2 := newTestLibrary("la-gm-2")
	lib3 := newTestLibrary("la-gm-3")
	require.NoError(t, libRepo.Create(ctx, lib1))
	require.NoError(t, libRepo.Create(ctx, lib2))
	require.NoError(t, libRepo.Create(ctx, lib3))

	// Empty slice = no-op.
	require.NoError(t, repo.GrantMany(ctx, shared.UserID(u.ID), nil))
	ids, err := repo.GetByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Empty(t, ids)

	// Batch grant 3.
	require.NoError(t, repo.GrantMany(ctx, shared.UserID(u.ID), []shared.LibraryID{
		shared.LibraryID(lib1.ID), shared.LibraryID(lib2.ID), shared.LibraryID(lib3.ID),
	}))
	ids, err = repo.GetByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Len(t, ids, 3)

	// Idempotent: re-grant same subset.
	require.NoError(t, repo.GrantMany(ctx, shared.UserID(u.ID), []shared.LibraryID{
		shared.LibraryID(lib1.ID), shared.LibraryID(lib2.ID),
	}))
	ids, err = repo.GetByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Len(t, ids, 3, "ON CONFLICT DO NOTHING - count unchanged")
}

func TestLibraryAccessRepo_HasAccess(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-u2")
	require.NoError(t, userRepo.Create(ctx, u))
	lib1 := newTestLibrary("la-lib2a")
	lib2 := newTestLibrary("la-lib2b")
	require.NoError(t, libRepo.Create(ctx, lib1))
	require.NoError(t, libRepo.Create(ctx, lib2))

	// no restrictions = full access
	has, err := repo.HasAccess(ctx, shared.UserID(u.ID), shared.LibraryID(lib1.ID))
	require.NoError(t, err)
	assert.True(t, has)

	// grant specific library, now restricted
	require.NoError(t, repo.Grant(ctx, shared.UserID(u.ID), shared.LibraryID(lib1.ID)))

	has, err = repo.HasAccess(ctx, shared.UserID(u.ID), shared.LibraryID(lib1.ID))
	require.NoError(t, err)
	assert.True(t, has)

	has, err = repo.HasAccess(ctx, shared.UserID(u.ID), shared.LibraryID(lib2.ID))
	require.NoError(t, err)
	assert.False(t, has)
}

func TestLibraryAccessRepo_Revoke(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-u3")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("la-lib3")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Grant(ctx, shared.UserID(u.ID), shared.LibraryID(lib.ID)))
	require.NoError(t, repo.Revoke(ctx, shared.UserID(u.ID), shared.LibraryID(lib.ID)))

	ids, err := repo.GetByUserID(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestLibraryAccessRepo_ListUsersWithRestrictions(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-u4")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("la-lib4")
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Grant(ctx, shared.UserID(u.ID), shared.LibraryID(lib.ID)))

	m, err := repo.ListUsersWithRestrictions(ctx)
	require.NoError(t, err)
	assert.Contains(t, m, shared.UserID(u.ID))
	assert.Contains(t, m[shared.UserID(u.ID)], shared.LibraryID(lib.ID))
}
