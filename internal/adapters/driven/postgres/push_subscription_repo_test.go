package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushSubscriptionRepo_Upsert_GetByProfile(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	repo := postgres.NewPushSubscriptionRepository(db)
	ctx := context.Background()

	u := newTestUser("ps-u1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "ps-p1", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))

	require.NoError(t, repo.Upsert(ctx, u.ID, p.ID, "https://push.example.com/sub1", "p256dh-key", "auth-key"))

	subs, err := repo.GetByProfile(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, subs, 1)
	assert.Equal(t, "https://push.example.com/sub1", subs[0].Endpoint)
}

func TestPushSubscriptionRepo_ListByUser(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	repo := postgres.NewPushSubscriptionRepository(db)
	ctx := context.Background()

	u := newTestUser("ps-u2")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "ps-p2", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))

	require.NoError(t, repo.Upsert(ctx, u.ID, p.ID, "https://push.example.com/sub2a", "k1", "a1"))
	require.NoError(t, repo.Upsert(ctx, u.ID, p.ID, "https://push.example.com/sub2b", "k2", "a2"))

	subs, err := repo.ListByUser(ctx, u.ID)
	require.NoError(t, err)
	assert.Len(t, subs, 2)
}

func TestPushSubscriptionRepo_DeleteByEndpoint(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	repo := postgres.NewPushSubscriptionRepository(db)
	ctx := context.Background()

	u := newTestUser("ps-u3")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "ps-p3", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))

	endpoint := "https://push.example.com/sub3"
	require.NoError(t, repo.Upsert(ctx, u.ID, p.ID, endpoint, "k", "a"))
	require.NoError(t, repo.DeleteByEndpoint(ctx, u.ID, endpoint))

	subs, err := repo.GetByProfile(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, subs)
}

func TestPushSubscriptionRepo_DeleteByUser(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	repo := postgres.NewPushSubscriptionRepository(db)
	ctx := context.Background()

	u := newTestUser("ps-u4")
	require.NoError(t, userRepo.Create(ctx, u))
	p := &user.Profile{ID: "ps-p4", UserID: shared.UserID(u.ID), Name: "Default", Preferences: map[string]string{}, IsDefault: true, CreatedAt: time.Now().UTC()}
	require.NoError(t, profileRepo.Create(ctx, p))

	require.NoError(t, repo.Upsert(ctx, u.ID, p.ID, "https://push.example.com/sub4", "k", "a"))
	require.NoError(t, repo.DeleteByUser(ctx, u.ID))

	subs, err := repo.ListByUser(ctx, u.ID)
	require.NoError(t, err)
	assert.Empty(t, subs)
}
