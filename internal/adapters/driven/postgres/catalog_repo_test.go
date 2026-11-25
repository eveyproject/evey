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
	"github.com/eveyproject/evey/internal/domain/shared"
)

// The catalog repo against a real schema: what is due, what is stored, what
// the libraries own (an episode's season through its parent), and the films
// of a started saga.
func TestCatalogRepo_RoundTrip(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	libs := postgres.NewLibraryRepository(db)
	items := postgres.NewMediaItemRepository(db)
	cols := postgres.NewCollectionRepository(db)
	repo := postgres.NewCatalogRepo(db)

	require.NoError(t, libs.Create(ctx, newTestLibrary("cat1")))
	lib := shared.LibraryID("cat1")
	mk := func(id string, typ library.MediaItemType, parent string, idx *int, tmdb string) {
		it := newTestMediaItem(id, lib)
		it.Type, it.ParentID, it.IndexNumber, it.Title, it.SortTitle = typ, parent, idx, id, id
		it.Catalog.ProviderIDs = map[string]string{}
		if tmdb != "" {
			it.Catalog.ProviderIDs["tmdb"] = tmdb
		}
		require.NoError(t, items.Create(ctx, it))
	}
	mk("got", library.Series, "", nil, "1399")
	mk("got-s1", library.Season, "got", new(1), "")
	mk("got-s1e1", library.Episode, "got-s1", new(1), "")
	mk("got-s1e3", library.Episode, "got-s1", new(3), "")
	mk("got-s1ex", library.Episode, "got-s1", nil, "") // no number: never counted
	mk("local", library.Series, "", nil, "")           // no TMDB id: never due
	mk("rocky", library.MovieItem, "", nil, "1366")
	mk("rocky2", library.MovieItem, "", nil, "1367")

	due, err := repo.SeriesToCheck(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, "got", due[0].ItemID)
	assert.Equal(t, "1399", due[0].TmdbID)

	aired := time.Date(2011, 4, 17, 0, 0, 0, 0, time.UTC)
	require.NoError(t, repo.ReplaceSeriesEpisodes(ctx, "got", []library.ExpectedEpisode{
		{Season: 1, Episode: 1, Title: "Winter Is Coming", AirDate: &aired},
		{Season: 1, Episode: 2},
	}, time.Now()))
	due, err = repo.SeriesToCheck(ctx, time.Now().Add(-time.Hour), 10)
	require.NoError(t, err)
	assert.Empty(t, due, "checked within the hour: not due")

	exp, err := repo.ExpectedEpisodes(ctx, []string{"got"})
	require.NoError(t, err)
	require.Len(t, exp["got"], 2)
	require.NotNil(t, exp["got"][0].AirDate)
	assert.Nil(t, exp["got"][1].AirDate)

	owned, err := repo.OwnedEpisodes(ctx, []string{"got", "local"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []library.OwnedEpisode{{SeriesID: "got", Season: 1, Episode: 1}, {SeriesID: "got", Season: 1, Episode: 3}}, owned["got"])

	series, err := repo.ListSeries(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, series, 2)
	none, err := repo.ListSeries(ctx, []shared.LibraryID{"elsewhere"})
	require.NoError(t, err)
	assert.Empty(t, none)

	saga := &library.Collection{ID: "c-rocky", TmdbID: new(1575), Name: "Rocky"}
	require.NoError(t, cols.Upsert(ctx, saga))
	require.NoError(t, cols.LinkItem(ctx, saga.ID, "rocky", 0))
	require.NoError(t, cols.LinkItem(ctx, saga.ID, "rocky2", 1))
	sagas, ownedFilms, err := repo.SagasOwned(ctx, nil)
	require.NoError(t, err)
	require.Len(t, sagas, 1)
	assert.Equal(t, 1575, sagas[0].TmdbID)
	assert.ElementsMatch(t, []int{1366, 1367}, ownedFilms[saga.ID])

	dueC, err := repo.CollectionsToCheck(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, dueC, 1)
	require.NoError(t, repo.ReplaceCollectionParts(ctx, saga.ID, []library.CollectionPart{
		{TmdbID: 1366, Title: "Rocky", ReleaseDate: &aired},
		{TmdbID: 1371, Title: "Rocky III"},
	}, time.Now()))
	parts, err := repo.CollectionParts(ctx, []string{saga.ID})
	require.NoError(t, err)
	assert.Len(t, parts[saga.ID], 2)

	// A deleted series takes its stored catalog with it.
	_, err = db.ExecContext(ctx, `DELETE FROM media_items WHERE id = 'got'`)
	require.NoError(t, err)
	exp, err = repo.ExpectedEpisodes(ctx, []string{"got"})
	require.NoError(t, err)
	assert.Empty(t, exp)
}
