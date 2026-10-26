package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// A user restricted to libraries that no longer hold a grant row (library
// deleted, last grant revoked, invite ids gone stale) must see nothing, not
// everything: the restriction is a flag on the user, not "rows exist".

func TestLibraryAccessRepo_DeletedOnlyLibraryDoesNotWidenAccess(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-r-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	only := newTestLibrary("la-r-only")
	other := newTestLibrary("la-r-other")
	require.NoError(t, libRepo.Create(ctx, only))
	require.NoError(t, libRepo.Create(ctx, other))
	require.NoError(t, repo.Grant(ctx, shared.UserID(u.ID), shared.LibraryID(only.ID)))

	require.NoError(t, libRepo.Delete(ctx, shared.LibraryID(only.ID)))

	has, err := repo.HasAccess(ctx, shared.UserID(u.ID), shared.LibraryID(other.ID))
	require.NoError(t, err)
	assert.False(t, has, "the cascade removed the grant, not the restriction")
	restricted, err := repo.IsRestricted(ctx, shared.UserID(u.ID))
	require.NoError(t, err)
	assert.True(t, restricted)
}

func TestLibraryAccessRepo_RevokingTheLastGrantDeniesAll(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-r-u2")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("la-r-lib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	require.NoError(t, repo.Grant(ctx, shared.UserID(u.ID), shared.LibraryID(lib.ID)))
	require.NoError(t, repo.Revoke(ctx, shared.UserID(u.ID), shared.LibraryID(lib.ID)))

	has, err := repo.HasAccess(ctx, shared.UserID(u.ID), shared.LibraryID(lib.ID))
	require.NoError(t, err)
	assert.False(t, has)
}

func TestLibraryAccessRepo_StaleInviteLibrariesDenyAll(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-r-u3")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("la-r-lib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	require.NoError(t, repo.GrantMany(ctx, shared.UserID(u.ID), []shared.LibraryID{"la-r-gone"}))

	has, err := repo.HasAccess(ctx, shared.UserID(u.ID), shared.LibraryID(lib.ID))
	require.NoError(t, err)
	assert.False(t, has)
}

// RevokeAll and an empty replace are the admin's "unrestricted" action: they
// clear the flag with the grants.
func TestLibraryAccessRepo_RevokeAllAndEmptyReplaceLiftTheRestriction(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewLibraryAccessRepository(db)
	ctx := context.Background()

	u := newTestUser("la-r-u4")
	require.NoError(t, userRepo.Create(ctx, u))
	a := newTestLibrary("la-r-a")
	b := newTestLibrary("la-r-b")
	require.NoError(t, libRepo.Create(ctx, a))
	require.NoError(t, libRepo.Create(ctx, b))
	uid := shared.UserID(u.ID)

	require.NoError(t, repo.ReplaceAccess(ctx, uid, []shared.LibraryID{shared.LibraryID(a.ID)}))
	has, err := repo.HasAccess(ctx, uid, shared.LibraryID(b.ID))
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, repo.DeleteByUserID(ctx, uid))
	has, err = repo.HasAccess(ctx, uid, shared.LibraryID(b.ID))
	require.NoError(t, err)
	assert.True(t, has)
	restricted, err := repo.IsRestricted(ctx, uid)
	require.NoError(t, err)
	assert.False(t, restricted)

	require.NoError(t, repo.ReplaceAccess(ctx, uid, []shared.LibraryID{shared.LibraryID(a.ID)}))
	require.NoError(t, repo.ReplaceAccess(ctx, uid, nil))
	has, err = repo.HasAccess(ctx, uid, shared.LibraryID(b.ID))
	require.NoError(t, err)
	assert.True(t, has)
}
