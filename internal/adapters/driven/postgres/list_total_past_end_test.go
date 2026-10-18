package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
)

// The photo and music offset lists carried their total as COUNT(*) OVER() on
// the returned rows. A page past the last row has no row to carry it, so the
// total read 0 - a client paging a filtered or shrunk set was told the library
// was empty. The window also made Postgres read and sort every matching row
// whatever the LIMIT. The total is now its own count.
func TestOffsetLists_TotalSurvivesAPagePastTheEnd(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)

	photoLib := newTestLibrary("tot-ph")
	photoLib.Type = "photo"
	require.NoError(t, libRepo.Create(ctx, photoLib))
	photos := postgres.NewPhotoRepository(db)
	for _, id := range []string{"tp1", "tp2", "tp3"} {
		require.NoError(t, photos.Upsert(ctx, newTestPhoto(id, photoLib.ID, "trip")))
	}

	musicLib := newTestLibrary("tot-mu")
	musicLib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, musicLib))
	tracks := postgres.NewMusicTrackRepository(db)
	for _, id := range []string{"tm1", "tm2", "tm3"} {
		_, err := tracks.Upsert(ctx, newTestTrack(id, musicLib.ID, "Song "+id, "Artist", "Album"))
		require.NoError(t, err)
	}

	for _, offset := range []int{0, 2, 10} {
		p, total, err := photos.ListByLibrary(ctx, photoLib.ID, 2, offset)
		require.NoError(t, err)
		assert.Equal(t, 3, total, "photos by library, offset %d", offset)
		assert.LessOrEqual(t, len(p), 2)

		_, total, err = photos.ListByAlbum(ctx, photoLib.ID, "trip", 2, offset)
		require.NoError(t, err)
		assert.Equal(t, 3, total, "photos by album, offset %d", offset)

		_, total, err = tracks.ListByLibrary(ctx, musicLib.ID, 2, offset, "")
		require.NoError(t, err)
		assert.Equal(t, 3, total, "tracks by library, offset %d", offset)

		_, total, err = tracks.ListByLibrary(ctx, musicLib.ID, 2, offset, "Song tm2")
		require.NoError(t, err)
		assert.Equal(t, 1, total, "tracks by library with a search, offset %d", offset)

		_, total, err = tracks.ListByArtist(ctx, "Artist", 2, offset)
		require.NoError(t, err)
		assert.Equal(t, 3, total, "tracks by artist, offset %d", offset)
	}
}
