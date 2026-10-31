package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
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
	assert.Equal(t, "mi-2", got[0].MediaItemID)
	assert.Equal(t, library.SearchOpUpsert, got[0].Op)
	assert.Equal(t, "mi-1", got[1].MediaItemID)
	assert.Equal(t, library.SearchOpDelete, got[1].Op)

	page, err := repo.List(ctx, 1)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, "mi-2", page[0].MediaItemID, "the limit keeps the oldest")

	require.NoError(t, repo.Delete(ctx, got[0]))
	got, err = repo.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mi-1", got[0].MediaItemID)
	require.NoError(t, repo.Delete(ctx, library.PendingSearchWrite{MediaItemID: "mi-2", Op: library.SearchOpUpsert, EnqueuedAt: time.Now()}), "draining an already drained item is a no-op")
}

// A document the engine keeps refusing is counted per drain, then
// dead-lettered out of the listing; a fresh write gives it a new chance.
func TestPendingSearchWriteRepo_RecordFailureDeadLetters(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewPendingSearchWriteRepository(db)

	require.NoError(t, repo.Enqueue(ctx, "mi-bad", library.SearchOpUpsert))
	require.NoError(t, repo.Enqueue(ctx, "mi-ok", library.SearchOpUpsert))

	dead, err := repo.RecordFailure(ctx, bad(t, repo), 2)
	require.NoError(t, err)
	assert.False(t, dead, "first refusal only counts")
	got, err := repo.List(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, got, 2)

	dead, err = repo.RecordFailure(ctx, bad(t, repo), 2)
	require.NoError(t, err)
	assert.True(t, dead, "the second refusal reaches the limit")
	got, err = repo.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "mi-ok", got[0].MediaItemID, "a dead-lettered row is no longer replayed")

	require.NoError(t, repo.Enqueue(ctx, "mi-bad", library.SearchOpUpsert))
	got, err = repo.List(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, got, 2, "a new write revives the dead-lettered row")
	dead, err = repo.RecordFailure(ctx, bad(t, repo), 2)
	require.NoError(t, err)
	assert.False(t, dead, "the revived row starts counting from zero")
}

// A write re-enqueued after the drain listed the row must survive the drain's
// acknowledgement: the drain confirmed the older content, not the newer one.
func TestPendingSearchWriteRepo_ReEnqueuedRowSurvivesStaleDelete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewPendingSearchWriteRepository(db)

	require.NoError(t, repo.Enqueue(ctx, "mi-1", library.SearchOpUpsert))
	listed, err := repo.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, listed, 1)

	require.NoError(t, repo.Enqueue(ctx, "mi-1", library.SearchOpUpsert))

	require.NoError(t, repo.Delete(ctx, listed[0]))
	got, err := repo.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, got, 1, "the newer write is still pending for the next drain")

	dead, err := repo.RecordFailure(ctx, listed[0], 1)
	require.NoError(t, err)
	assert.False(t, dead, "a refusal of the older document does not count against the newer one")

	require.NoError(t, repo.Delete(ctx, got[0]))
	got, err = repo.List(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, got, "an up-to-date acknowledgement still removes the row")
}

func bad(t *testing.T, repo *postgres.PendingSearchWriteRepository) library.PendingSearchWrite {
	t.Helper()
	all, err := repo.List(context.Background(), 0)
	require.NoError(t, err)
	for _, w := range all {
		if w.MediaItemID == "mi-bad" {
			return w
		}
	}
	return library.PendingSearchWrite{MediaItemID: "mi-bad", Op: library.SearchOpUpsert, EnqueuedAt: time.Now()}
}
