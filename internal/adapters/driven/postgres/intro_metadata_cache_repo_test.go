package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
)

// The community intro-metadata cache: a miss is (nil, zero, nil) and not an
// error, one row per upstream source, the most recent answer wins a read,
// and an invalidation clears every source for the episode at once.
func TestIntroMetadataCacheRepo(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo := postgres.NewIntroMetadataCacheRepository(db)

	payload, at, err := repo.Get(ctx, 100, 1, 1)
	require.NoError(t, err)
	assert.Nil(t, payload)
	assert.True(t, at.IsZero(), "a miss is not an error")

	require.NoError(t, repo.Put(ctx, 100, 1, 1, "tidb", []byte(`{"start":10}`)))
	payload, at, err = repo.Get(ctx, 100, 1, 1)
	require.NoError(t, err)
	assert.JSONEq(t, `{"start":10}`, string(payload))
	assert.False(t, at.IsZero())

	// A second source for the same episode is its own row; the newest
	// answer is what a read returns.
	require.NoError(t, repo.Put(ctx, 100, 1, 1, "introdb", []byte(`{"start":12}`)))
	payload, _, err = repo.Get(ctx, 100, 1, 1)
	require.NoError(t, err)
	assert.JSONEq(t, `{"start":12}`, string(payload))

	// Re-fetching a source rewrites that source's row only.
	require.NoError(t, repo.Put(ctx, 100, 1, 1, "tidb", []byte(`{"start":11}`)))
	payload, _, err = repo.Get(ctx, 100, 1, 1)
	require.NoError(t, err)
	assert.JSONEq(t, `{"start":11}`, string(payload))

	// Neighbouring episodes are separate keys.
	_, at, err = repo.Get(ctx, 100, 1, 2)
	require.NoError(t, err)
	assert.True(t, at.IsZero())
	require.NoError(t, repo.Put(ctx, 100, 1, 2, "tidb", []byte(`{"start":0}`)))

	n, err := repo.Invalidate(ctx, 100, 1, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n, "every source for the episode goes")
	_, at, err = repo.Get(ctx, 100, 1, 1)
	require.NoError(t, err)
	assert.True(t, at.IsZero())
	payload, _, err = repo.Get(ctx, 100, 1, 2)
	require.NoError(t, err)
	assert.JSONEq(t, `{"start":0}`, string(payload), "the next episode is untouched")
	n, err = repo.Invalidate(ctx, 100, 1, 1)
	require.NoError(t, err)
	assert.Zero(t, n)
}
