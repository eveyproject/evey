package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
)

// Two writers updating the same item at once (a watcher scan and an
// auto-enrich) must not collide on the positional trailer ids: in prod the
// loser failed with a duplicate key on media_trailers_pkey.
func TestMediaItemRepo_ConcurrentUpdatesKeepTheTrailers(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewMediaItemRepository(db)
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, newTestLibrary("trlib")))
	item := fullItem("tr1", "trlib", "fp-tr1")
	item.Trailers = []library.Trailer{{Site: "YouTube", Key: "a"}, {Site: "YouTube", Key: "b"}}
	require.NoError(t, repo.Create(ctx, item))

	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for range 8 {
		wg.Go(func() {
			for range 20 {
				cp := *item
				if err := repo.Update(ctx, &cp); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	got, err := repo.GetByID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got.Trailers, 2, "the item keeps exactly its own trailers")
	assert.Equal(t, "a", got.Trailers[0].Key)
	assert.Equal(t, "b", got.Trailers[1].Key)
}
