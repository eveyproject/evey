package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRatingRepo_Upsert_Get(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewUserRatingRepository(db)
	ctx := context.Background()

	u := newTestUser("urat-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "urat-p1", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("urat-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("urat-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	r := &library.UserRating{ProfileID: p.ID, MediaItemID: item.ID, Value: 1}
	require.NoError(t, repo.Upsert(ctx, r))

	got, err := repo.Get(ctx, string(p.ID), string(item.ID))
	require.NoError(t, err)
	assert.Equal(t, 1, got.Value)
}

func TestUserRatingRepo_Get_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRatingRepository(db)
	ctx := context.Background()

	_, err := repo.Get(ctx, "no-profile", "no-item")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestUserRatingRepo_GetForItems(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewUserRatingRepository(db)
	ctx := context.Background()

	u := newTestUser("urat-u2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "urat-p2", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("urat-lib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("urat-mi2a", lib.ID)
	item2 := newTestMediaItem("urat-mi2b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	require.NoError(t, itemRepo.Create(ctx, item2))

	require.NoError(t, repo.Upsert(ctx, &library.UserRating{ProfileID: p.ID, MediaItemID: item1.ID, Value: 1}))
	require.NoError(t, repo.Upsert(ctx, &library.UserRating{ProfileID: p.ID, MediaItemID: item2.ID, Value: -1}))

	result, err := repo.GetForItems(ctx, string(p.ID), []string{string(item1.ID), string(item2.ID)})
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 1, result[string(item1.ID)].Value)
	assert.Equal(t, -1, result[string(item2.ID)].Value)
}

func TestUserRatingRepo_GetForItems_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRatingRepository(db)
	ctx := context.Background()

	result, err := repo.GetForItems(ctx, "any", []string{})
	require.NoError(t, err)
	assert.Empty(t, result)
}
