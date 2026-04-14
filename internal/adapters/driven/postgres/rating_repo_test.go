package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/rating"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRatingRepo_Upsert_Get(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewRatingRepository(db)
	ctx := context.Background()

	u := newTestUser("rat-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "rat-p1", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("rat-lib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("rat-mi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	val, err := rating.NewRatingValue(4)
	require.NoError(t, err)

	rt := rating.Rating{
		MediaItemID: string(item.ID),
		ProfileID:   string(p.ID),
		Value:       val,
	}
	require.NoError(t, repo.Upsert(ctx, rt))

	got, err := repo.Get(ctx, string(item.ID), string(p.ID))
	require.NoError(t, err)
	assert.Equal(t, 4, got.Value.Int())
}

func TestRatingRepo_Get_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewRatingRepository(db)
	ctx := context.Background()

	_, err := repo.Get(ctx, "nonexistent", "noprofile")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestRatingRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewRatingRepository(db)
	ctx := context.Background()

	u := newTestUser("rat-u2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "rat-p2", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("rat-lib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("rat-mi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	val, _ := rating.NewRatingValue(3)
	require.NoError(t, repo.Upsert(ctx, rating.Rating{MediaItemID: string(item.ID), ProfileID: string(p.ID), Value: val}))
	require.NoError(t, repo.Delete(ctx, string(item.ID), string(p.ID)))

	_, err := repo.Get(ctx, string(item.ID), string(p.ID))
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestRatingRepo_GetAggregates(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewRatingRepository(db)
	ctx := context.Background()

	u := newTestUser("rat-u3")
	require.NoError(t, userRepo.Create(ctx, u))
	p1 := &user.Profile{ID: "rat-p3a", UserID: shared.UserID(u.ID), Name: "P1", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	p2 := &user.Profile{ID: "rat-p3b", UserID: shared.UserID(u.ID), Name: "P2", Preferences: map[string]string{}, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p1))
	require.NoError(t, profileRepo.Create(ctx, p2))
	lib := newTestLibrary("rat-lib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("rat-mi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	val2, _ := rating.NewRatingValue(2)
	val4, _ := rating.NewRatingValue(4)
	require.NoError(t, repo.Upsert(ctx, rating.Rating{MediaItemID: string(item.ID), ProfileID: string(p1.ID), Value: val2}))
	require.NoError(t, repo.Upsert(ctx, rating.Rating{MediaItemID: string(item.ID), ProfileID: string(p2.ID), Value: val4}))

	aggs, err := repo.GetAggregates(ctx, []string{string(item.ID)})
	require.NoError(t, err)
	agg, ok := aggs[string(item.ID)]
	require.True(t, ok)
	assert.Equal(t, 2, agg.Count)
	assert.InDelta(t, 3.0, agg.Average, 0.1)
}

func TestRatingRepo_GetAggregates_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewRatingRepository(db)
	ctx := context.Background()

	aggs, err := repo.GetAggregates(ctx, []string{})
	require.NoError(t, err)
	assert.Empty(t, aggs)
}
