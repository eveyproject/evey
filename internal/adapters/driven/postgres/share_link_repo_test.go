package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
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
		MediaItemID: item.ID,
		CreatedBy:   u.ID,
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
		ViewCount:   0,
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))

	got, err := repo.GetByToken(ctx, "share-tok-1")
	require.NoError(t, err)
	assert.Equal(t, "sl1", got.ID)
	assert.Equal(t, item.ID, got.MediaItemID)
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
			MediaItemID: item.ID,
			CreatedBy:   u.ID,
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
		MediaItemID: item.ID,
		CreatedBy:   u.ID,
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))

	list, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(list), 1)
}

func TestShareLinkRepo_ConsumeView(t *testing.T) {
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
		MediaItemID: item.ID,
		CreatedBy:   u.ID,
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))
	spent, err := repo.ConsumeView(ctx, "sl4-tok")
	require.NoError(t, err)
	assert.Equal(t, 1, spent.ViewCount)

	got, err := repo.GetByToken(ctx, "sl4-tok")
	require.NoError(t, err)
	assert.Equal(t, 1, got.ViewCount)

	_, err = repo.ConsumeView(ctx, "unknown-tok")
	require.ErrorIs(t, err, shared.ErrNotFound)
}

// An expired or exhausted link refuses the spend and keeps its counter.
func TestShareLinkRepo_ConsumeView_RefusesWithoutSpending(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewShareLinkRepo(db)
	ctx := context.Background()

	u := newTestUser("sl-u4b")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("sl-lib4b")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("sl-mi4b", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	one := 1
	now := time.Now().UTC().Truncate(time.Millisecond)
	exhausted := &shared.ShareLink{ID: "sl4b", Token: "sl4b-tok", MediaItemID: item.ID, CreatedBy: u.ID,
		ExpiresAt: now.Add(time.Hour), MaxViews: &one, ViewCount: 1, CreatedAt: now}
	expired := &shared.ShareLink{ID: "sl4c", Token: "sl4c-tok", MediaItemID: item.ID, CreatedBy: u.ID,
		ExpiresAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Hour)}
	require.NoError(t, repo.Create(ctx, exhausted))
	require.NoError(t, repo.Create(ctx, expired))

	for _, l := range []*shared.ShareLink{exhausted, expired} {
		_, err := repo.ConsumeView(ctx, l.Token)
		require.ErrorIs(t, err, shared.ErrNotFound)
		got, err := repo.GetByToken(ctx, l.Token)
		require.NoError(t, err)
		assert.Equal(t, l.ViewCount, got.ViewCount, l.Token)
	}
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
		MediaItemID: item.ID,
		CreatedBy:   u.ID,
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))
	require.NoError(t, repo.Delete(ctx, "sl5"))

	_, err := repo.GetByToken(ctx, "sl5-tok")
	assert.Error(t, err)
}

func TestShareLinkRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewShareLinkRepo(db)
	err := repo.Delete(context.Background(), "sl-missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
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
		MediaItemID: item.ID,
		CreatedBy:   u.ID,
		// An expired link as it really occurs: created legitimately (expiry
		// after creation, so it passes Validate) and simply overtaken by time.
		ExpiresAt: time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Millisecond),
		CreatedAt: time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, repo.Create(ctx, link))

	count, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// GetByID is the revoke path's lookup: the owner check needs created_by and
// the row must come back whole; an unknown id is an error, not a nil link.
func TestShareLinkRepo_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	u := newTestUser("sl-gid")
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	lib := newTestLibrary("sl-gidlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	item := newTestMediaItem("sl-gidmi", lib.ID)
	require.NoError(t, postgres.NewMediaItemRepository(db).Create(ctx, item))
	repo := postgres.NewShareLinkRepo(db)
	maxViews := 3
	require.NoError(t, repo.Create(ctx, &shared.ShareLink{
		ID: "sl-gid-1", Token: "share-tok-gid", MediaItemID: item.ID, CreatedBy: u.ID, MaxViews: &maxViews,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond), CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}))

	got, err := repo.GetByID(ctx, "sl-gid-1")
	require.NoError(t, err)
	assert.Equal(t, "share-tok-gid", got.Token)
	assert.Equal(t, u.ID, got.CreatedBy)
	assert.Equal(t, item.ID, got.MediaItemID)
	require.NotNil(t, got.MaxViews)
	assert.Equal(t, 3, *got.MaxViews)

	_, err = repo.GetByID(ctx, "sl-ghost")
	require.Error(t, err)
}
