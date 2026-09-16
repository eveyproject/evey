package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
)

// The search outbox: writes the engine refused while down, replayed oldest
// first, last op wins per item (an upsert then a delete during one outage
// must replay as the delete).
func TestPendingSearchWriteRepo_LastOpWinsOldestFirst(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewPendingSearchWriteRepository(db)

	got, err := repo.List(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, got)

	require.NoError(t, repo.Enqueue(ctx, "mi-1", library.SearchOpUpsert))
	require.NoError(t, repo.Enqueue(ctx, "mi-2", library.SearchOpUpsert))
	// mi-1 is re-enqueued LAST with a delete: it takes the delete and moves
	// to the back of the queue.
	require.NoError(t, repo.Enqueue(ctx, "mi-1", library.SearchOpDelete))

	got, err = repo.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, got, 2, "one row per item, whatever the number of failures")
	assert.Equal(t, library.PendingSearchWrite{MediaItemID: "mi-2", Op: library.SearchOpUpsert}, got[0])
	assert.Equal(t, library.PendingSearchWrite{MediaItemID: "mi-1", Op: library.SearchOpDelete}, got[1])

	page, err := repo.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, "mi-2", page[0].MediaItemID, "the limit keeps the oldest")

	require.NoError(t, repo.Delete(ctx, "mi-2"))
	got, err = repo.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mi-1", got[0].MediaItemID)
	require.NoError(t, repo.Delete(ctx, "mi-2"), "draining an already drained item is a no-op")
}
