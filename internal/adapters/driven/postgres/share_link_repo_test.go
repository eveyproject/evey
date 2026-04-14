package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShareLinkRepo_Create_GetByToken(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewShareLinkRepo(db)
	ctx := context.Background()

	u := newTestUser("sl-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("sl-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("sl-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	link := &shared.ShareLink{
		ID:          "sl1",
		Token:       "share-tok-1",
		MediaItemID: string(item.ID),
		CreatedBy:   string(u.ID),
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
		ViewCount:   0,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))

	got, err := repo.GetByToken(ctx, "share-tok-1")
	require.NoError(t, err)
	assert.Equal(t, "sl1", got.ID)
	assert.Equal(t, string(item.ID), got.MediaItemID)
}

func TestShareLinkRepo_ListByUser(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewShareLinkRepo(db)
	ctx := context.Background()

	u := newTestUser("sl-u2")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("sl-lib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("sl-mi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	for i, tok := range []string{"sl2a-tok", "sl2b-tok"} {
		link := &shared.ShareLink{
			ID:          tok,
			Token:       tok,
			MediaItemID: string(item.ID),
			CreatedBy:   string(u.ID),
			ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second).UTC().Truncate(time.Millisecond),
		}
		require.NoError(t, repo.Create(ctx, link))
	}

	list, err := repo.ListByUser(ctx, string(u.ID))
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestShareLinkRepo_ListAll(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewShareLinkRepo(db)
	ctx := context.Background()

	u := newTestUser("sl-u3")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("sl-lib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("sl-mi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	link := &shared.ShareLink{
		ID:          "sl3",
		Token:       "sl3-tok",
		MediaItemID: string(item.ID),
		CreatedBy:   string(u.ID),
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))

	list, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 1)
}

func TestShareLinkRepo_IncrementViewCount(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewShareLinkRepo(db)
	ctx := context.Background()

	u := newTestUser("sl-u4")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("sl-lib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("sl-mi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	link := &shared.ShareLink{
		ID:          "sl4",
		Token:       "sl4-tok",
		MediaItemID: string(item.ID),
		CreatedBy:   string(u.ID),
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))
	require.NoError(t, repo.IncrementViewCount(ctx, "sl4-tok"))

	got, err := repo.GetByToken(ctx, "sl4-tok")
	require.NoError(t, err)
	assert.Equal(t, 1, got.ViewCount)
}

func TestShareLinkRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewShareLinkRepo(db)
	ctx := context.Background()

	u := newTestUser("sl-u5")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("sl-lib5")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("sl-mi5", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	link := &shared.ShareLink{
		ID:          "sl5",
		Token:       "sl5-tok",
		MediaItemID: string(item.ID),
		CreatedBy:   string(u.ID),
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))
	require.NoError(t, repo.Delete(ctx, "sl5"))

	_, err := repo.GetByToken(ctx, "sl5-tok")
	assert.Error(t, err)
}

func TestShareLinkRepo_DeleteExpired(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewShareLinkRepo(db)
	ctx := context.Background()

	u := newTestUser("sl-u6")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("sl-lib6")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("sl-mi6", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	link := &shared.ShareLink{
		ID:          "sl6",
		Token:       "sl6-tok",
		MediaItemID: string(item.ID),
		CreatedBy:   string(u.ID),
		ExpiresAt:   time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))

	count, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
