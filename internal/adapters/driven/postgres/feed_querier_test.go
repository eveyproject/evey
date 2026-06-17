package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Recommended must return only items sharing at least one of the profile's
// genres (the GIN ?| pre-filter must be behaviour-equivalent to the old
// per-row score>=1 filter).
func TestFeedQuerier_Recommended_GenrePrefilter(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	fq := postgres.NewFeedQuerier(db)
	ctx := context.Background()

	lib := newTestLibrary("recolib")
	require.NoError(t, libRepo.Create(ctx, lib))
	mk := func(id string, genres []string) {
		it := newTestMediaItem(id, lib.ID)
		it.Catalog.Genres = genres
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	mk("rec-a", []string{"Action", "Drama"})
	mk("rec-b", []string{"Comedy"})
	mk("rec-c", []string{"Action"})

	items, err := fq.Recommended(ctx, "prf-x", []string{"Action"}, 10)
	require.NoError(t, err)
	got := map[string]bool{}
	for _, it := range items {
		got[string(it.MediaItemID)] = true
	}
	assert.True(t, got["rec-a"], "rec-a shares Action")
	assert.True(t, got["rec-c"], "rec-c shares Action")
	assert.False(t, got["rec-b"], "rec-b shares no genre and must be excluded")
}

// SimilarItems must return only items sharing a genre with the target, ranked
// by the number of shared genres, excluding the target itself.
func TestFeedQuerier_SimilarItems_GenrePrefilter(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	fq := postgres.NewFeedQuerier(db)
	ctx := context.Background()

	lib := newTestLibrary("simlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	mk := func(id string, genres []string) {
		it := newTestMediaItem(id, lib.ID)
		it.Catalog.Genres = genres
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	mk("sim-t", []string{"Action", "Drama"}) // target
	mk("sim-a", []string{"Action"})          // shares 1
	mk("sim-b", []string{"Comedy"})          // shares 0
	mk("sim-c", []string{"Action", "Drama"}) // shares 2

	items, err := fq.SimilarItems(ctx, "sim-t", "prf-x", 10)
	require.NoError(t, err)

	scores := map[string]int{}
	for _, it := range items {
		scores[string(it.MediaItemID)] = it.Score
	}
	_, hasTarget := scores["sim-t"]
	assert.False(t, hasTarget, "the target itself must be excluded")
	_, hasB := scores["sim-b"]
	assert.False(t, hasB, "an item sharing no genre must be excluded")
	assert.Equal(t, 1, scores["sim-a"], "sim-a shares 1 genre")
	assert.Equal(t, 2, scores["sim-c"], "sim-c shares 2 genres")
	// Results must be ordered by shared-genre score descending.
	require.GreaterOrEqual(t, len(items), 2)
	for i := 1; i < len(items); i++ {
		assert.GreaterOrEqual(t, items[i-1].Score, items[i].Score, "items must be score-ordered")
	}
}
