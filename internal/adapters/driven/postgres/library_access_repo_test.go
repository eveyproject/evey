package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
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

// An invite link stores its library ids as JSONB - no FK. Delete one of those
// libraries and every redemption of the invite used to die on the FK violation
// inside GrantMany, taking the whole registration transaction with it. Stale
// ids must be skipped; surviving ones must still be granted.
func TestLibraryAccessRepo_GrantMany_SkipsDeletedLibraries(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	accessRepo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("gm-u")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("gm-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	err := accessRepo.GrantMany(ctx, u.ID, []shared.LibraryID{lib.ID, "lib-deleted-long-ago"})
	require.NoError(t, err, "a stale invite library id must not fail the grant")

	ok, err := accessRepo.HasAccess(ctx, u.ID, lib.ID)
	require.NoError(t, err)
	assert.True(t, ok, "the surviving library is still granted")
}

// ReplaceAccess is the admin's "set this user's libraries" in one atomic
// swap: the old set goes, exactly the new set comes, an empty set means
// "no restriction row at all" - and a library that no longer exists must
// fail the whole swap, never leave the user half-granted.
func TestLibraryAccessRepo_ReplaceAccess_AtomicSwap(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("ra-u")
	require.NoError(t, userRepo.Create(ctx, u))
	libA, libB, libC := newTestLibrary("ra-a"), newTestLibrary("ra-b"), newTestLibrary("ra-c")
	for _, l := range []*library.Library{libA, libB, libC} {
		require.NoError(t, libRepo.Create(ctx, l))
	}
	require.NoError(t, repo.GrantMany(ctx, u.ID, []shared.LibraryID{libA.ID, libB.ID}))

	require.NoError(t, repo.ReplaceAccess(ctx, u.ID, []shared.LibraryID{libB.ID, libC.ID, libC.ID}))
	ids, err := repo.GetByUserID(ctx, u.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []shared.LibraryID{libB.ID, libC.ID}, ids, "old grant gone, new set exactly, duplicate collapsed")

	// A vanished library id violates the FK: the swap rolls back as a whole.
	err = repo.ReplaceAccess(ctx, u.ID, []shared.LibraryID{libA.ID, "lib-gone"})
	require.Error(t, err)
	ids, err = repo.GetByUserID(ctx, u.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []shared.LibraryID{libB.ID, libC.ID}, ids, "a failed swap leaves the previous grants intact")

	require.NoError(t, repo.ReplaceAccess(ctx, u.ID, nil))
	ids, err = repo.GetByUserID(ctx, u.ID)
	require.NoError(t, err)
	assert.Empty(t, ids, "an empty set lifts every restriction")

	require.NoError(t, repo.GrantMany(ctx, u.ID, []shared.LibraryID{libA.ID, libB.ID}))
	require.NoError(t, repo.DeleteByUserID(ctx, u.ID))
	ids, err = repo.GetByUserID(ctx, u.ID)
	require.NoError(t, err)
	assert.Empty(t, ids, "DeleteByUserID revokes everything in one statement")
	require.NoError(t, repo.DeleteByUserID(ctx, u.ID), "revoking nothing is fine")
}
