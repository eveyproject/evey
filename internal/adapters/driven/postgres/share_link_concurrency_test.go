package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	appmedia "github.com/evey-project/evey/internal/app/media"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/require"
)

// Two downloads of a maxViews=1 link that genuinely overlap: a held row lock
// parks both inside Postgres until each is waiting, then lets them go at
// once. Exactly one is served, the other refused, and the counter ends at 1.
// The old check-increment-reread flow let both bump to 2 and refused both.
func TestShareService_Validate_ConcurrentMaxViewsOne(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewShareLinkRepo(db)
	svc := appmedia.NewShareService(repo)
	ctx := context.Background()

	u := newTestUser("slc-u")
	require.NoError(t, userRepo.Create(ctx, u))
	lib := newTestLibrary("slc-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("slc-mi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	one := 1
	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Create(ctx, &shared.ShareLink{
		ID: "slc", Token: "slc-tok", MediaItemID: item.ID, CreatedBy: u.ID,
		ExpiresAt: now.Add(time.Hour), MaxViews: &one, CreatedAt: now,
	}))

	gate, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = gate.Rollback() }()
	_, err = gate.ExecContext(ctx, `SELECT 1 FROM share_links WHERE token = 'slc-tok' FOR UPDATE`)
	require.NoError(t, err)

	const N = 2
	var wg sync.WaitGroup
	results := make(chan error, N)
	for range N {
		wg.Go(func() {
			_, verr := svc.Validate(ctx, "slc-tok")
			results <- verr
		})
	}

	require.Eventually(t, func() bool {
		var waiting int
		qerr := db.QueryRowContext(ctx,
			`SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting)
		return qerr == nil && waiting == N
	}, 10*time.Second, 10*time.Millisecond, "both downloads must be parked on the row lock")
	require.NoError(t, gate.Commit())

	wg.Wait()
	close(results)
	ok, refused := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, appmedia.ErrShareViewLimit):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, ok, "exactly one download is served")
	require.Equal(t, 1, refused, "the other is refused on the view limit")

	got, err := repo.GetByToken(ctx, "slc-tok")
	require.NoError(t, err)
	require.Equal(t, 1, got.ViewCount)
}
