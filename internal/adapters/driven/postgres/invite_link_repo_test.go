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
	assert.Nil(t, got.MaxProfiles, "no cap stays NULL")

	// The profile cap round-trips (migration 000170).
	capped := newTestInviteLink("inv1-cap", string(u.ID), "tok-cap")
	two := 2
	capped.MaxProfiles = &two
	require.NoError(t, repo.Create(ctx, capped))
	gotCapped, err := repo.GetByToken(ctx, "tok-cap")
	require.NoError(t, err)
	require.NotNil(t, gotCapped.MaxProfiles)
	assert.Equal(t, 2, *gotCapped.MaxProfiles)
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

// DecrementUsedCount rolls back a claim (used by rollback-on-create-failure
// path). Guarded against going below 0.
func TestInviteLinkRepo_DecrementUsedCount(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewInviteLinkRepository(db)
	ctx := context.Background()

	u := newTestUser("inv-u-dec")
	require.NoError(t, userRepo.Create(ctx, u))

	link := &user.InviteLink{
		ID:        "inv-dec",
		CreatedBy: shared.UserID(u.ID),
		Token:     "tok-dec",
		MaxUses:   5,
		UsedCount: 3,
	}
	require.NoError(t, repo.Create(ctx, link))

	require.NoError(t, repo.DecrementUsedCount(ctx, link.ID))
	got, err := repo.GetByToken(ctx, "tok-dec")
	require.NoError(t, err)
	assert.Equal(t, 2, got.UsedCount)

	// Floor at 0 - decrement below 0 is a no-op.
	link2 := &user.InviteLink{
		ID:        "inv-dec-0",
		CreatedBy: shared.UserID(u.ID),
		Token:     "tok-dec-0",
		MaxUses:   1,
		UsedCount: 0,
	}
	require.NoError(t, repo.Create(ctx, link2))
	require.NoError(t, repo.DecrementUsedCount(ctx, link2.ID)) // no error, no-op
	got2, err := repo.GetByToken(ctx, "tok-dec-0")
	require.NoError(t, err)
	assert.Equal(t, 0, got2.UsedCount, "must not go negative")
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

func TestInviteLinkRepo_Delete_NotFound(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewInviteLinkRepository(db)
	err := repo.Delete(context.Background(), "invite-missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}
