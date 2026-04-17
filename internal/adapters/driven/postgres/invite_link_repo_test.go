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

// Atomicity: a saturated invite (UsedCount = MaxUses) rejects further claims.
// Regression guard for the TOCTOU fix (fake mirrors this semantics).
func TestInviteLinkRepo_IncrementUsedCount_Saturated(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewInviteLinkRepository(db)
	ctx := context.Background()

	u := newTestUser("inv-u-sat")
	require.NoError(t, userRepo.Create(ctx, u))

	// Single-use invite already at its cap.
	link := &user.InviteLink{
		ID:        "inv-sat",
		CreatedBy: shared.UserID(u.ID),
		Token:     "tok-sat",
		MaxUses:   1,
		UsedCount: 1,
	}
	require.NoError(t, repo.Create(ctx, link))

	// Conditional UPDATE must not affect any row → should surface as an error.
	err := repo.IncrementUsedCount(ctx, link.ID)
	require.Error(t, err, "expected claim rejection for saturated invite")

	got, err := repo.GetByToken(ctx, "tok-sat")
	require.NoError(t, err)
	assert.Equal(t, 1, got.UsedCount, "UsedCount must remain unchanged")
}

// MaxUses = 0 means unlimited: claim always succeeds.
func TestInviteLinkRepo_IncrementUsedCount_Unlimited(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewInviteLinkRepository(db)
	ctx := context.Background()

	u := newTestUser("inv-u-unl")
	require.NoError(t, userRepo.Create(ctx, u))

	link := &user.InviteLink{
		ID:        "inv-unl",
		CreatedBy: shared.UserID(u.ID),
		Token:     "tok-unl",
		MaxUses:   0, // unlimited
		UsedCount: 1000,
	}
	require.NoError(t, repo.Create(ctx, link))

	require.NoError(t, repo.IncrementUsedCount(ctx, link.ID))
	got, err := repo.GetByToken(ctx, "tok-unl")
	require.NoError(t, err)
	assert.Equal(t, 1001, got.UsedCount)
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
