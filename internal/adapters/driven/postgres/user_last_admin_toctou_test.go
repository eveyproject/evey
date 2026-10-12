package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/require"
)

// Regression guard: two concurrent guarded removals of DISTINCT admins must
// not both pass a stale enabled-admin count and leave zero admins (a total
// lockout). The tx-scoped advisory lock serialises check+mutate, so exactly
// one wins and the other gets ErrLastEnabledAdmin.
func TestUser_LastAdminGuard_TOCTOU(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	a := newTestUser("la-a")
	a.IsAdmin = true
	b := newTestUser("la-b")
	b.IsAdmin = true
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))

	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results <- repo.DeleteIfNotLastAdmin(ctx, a.ID) }()
	go func() { defer wg.Done(); results <- repo.DisableIfNotLastAdmin(ctx, b.ID) }()
	wg.Wait()
	close(results)

	ok, refused := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, user.ErrLastEnabledAdmin):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, ok, "exactly one removal must win")
	require.Equal(t, 1, refused, "the loser must be refused as last-enabled-admin")

	n, err := repo.CountEnabledAdmins(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n, "one enabled admin must always remain")
}

// The guard never fires for a non-admin or for a DISABLED admin (removing one
// cannot lower the enabled-admin count), and it deletes/disables normally when
// other enabled admins remain.
func TestUser_LastAdminGuard_Semantics(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	admin := newTestUser("sem-admin")
	admin.IsAdmin = true
	disabledAdmin := newTestUser("sem-disabled")
	disabledAdmin.IsAdmin = true
	disabledAdmin.Enabled = false
	regular := newTestUser("sem-user")
	for _, u := range []*user.User{admin, disabledAdmin, regular} {
		require.NoError(t, repo.Create(ctx, u))
	}

	// The sole enabled admin cannot be deleted nor disabled.
	require.ErrorIs(t, repo.DeleteIfNotLastAdmin(ctx, admin.ID), user.ErrLastEnabledAdmin)
	require.ErrorIs(t, repo.DisableIfNotLastAdmin(ctx, admin.ID), user.ErrLastEnabledAdmin)

	// A disabled admin is freely removable even when only one enabled admin exists.
	require.NoError(t, repo.DeleteIfNotLastAdmin(ctx, disabledAdmin.ID))

	// A regular user passes straight through the guard.
	require.NoError(t, repo.DisableIfNotLastAdmin(ctx, regular.ID))
	got, err := repo.GetByID(ctx, regular.ID)
	require.NoError(t, err)
	require.False(t, got.Enabled)
}
