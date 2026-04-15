package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestInviteLink(id, creatorID, token string) *user.InviteLink {
	return &user.InviteLink{
		ID:        id,
		CreatedBy: shared.UserID(creatorID),
		Token:     token,
		MaxUses:   10,
		UsedCount: 0,
		IsAdmin:   false,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
}

func TestInviteLinkRepo_Create_GetByToken(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewInviteLinkRepository(db)
	ctx := context.Background()

	u := newTestUser("inv-u1")
	require.NoError(t, userRepo.Create(ctx, u))

	link := newTestInviteLink("inv1", string(u.ID), "tok-abc")
	require.NoError(t, repo.Create(ctx, link))

	got, err := repo.GetByToken(ctx, "tok-abc")
	require.NoError(t, err)
	assert.Equal(t, link.ID, got.ID)
	assert.Equal(t, link.Token, got.Token)
	assert.Equal(t, link.MaxUses, got.MaxUses)
}

func TestInviteLinkRepo_ListByCreator(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewInviteLinkRepository(db)
	ctx := context.Background()

	u := newTestUser("inv-u2")
	require.NoError(t, userRepo.Create(ctx, u))

	require.NoError(t, repo.Create(ctx, newTestInviteLink("inv2a", string(u.ID), "tok-1")))
	require.NoError(t, repo.Create(ctx, newTestInviteLink("inv2b", string(u.ID), "tok-2")))

	list, err := repo.ListByCreator(ctx, string(u.ID))
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestInviteLinkRepo_IncrementUsedCount(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewInviteLinkRepository(db)
	ctx := context.Background()

	u := newTestUser("inv-u3")
	require.NoError(t, userRepo.Create(ctx, u))

	link := newTestInviteLink("inv3", string(u.ID), "tok-inc")
	require.NoError(t, repo.Create(ctx, link))
	require.NoError(t, repo.IncrementUsedCount(ctx, link.ID))

	got, err := repo.GetByToken(ctx, "tok-inc")
	require.NoError(t, err)
	assert.Equal(t, 1, got.UsedCount)
}

func TestInviteLinkRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewInviteLinkRepository(db)
	ctx := context.Background()

	u := newTestUser("inv-u4")
	require.NoError(t, userRepo.Create(ctx, u))

	link := newTestInviteLink("inv4", string(u.ID), "tok-del")
	require.NoError(t, repo.Create(ctx, link))
	require.NoError(t, repo.Delete(ctx, link.ID))

	_, err := repo.GetByToken(ctx, "tok-del")
	assert.Error(t, err)
}
