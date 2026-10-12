package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMusicPlayRepo_GenreAffinityForProfile(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	trackRepo := postgres.NewMusicTrackRepository(db)
	playRepo := postgres.NewMusicPlayRepository(db)
	ctx := context.Background()

	u := newTestUser("gaf-u")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("gaf-p", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("gaf-lib")
	require.NoError(t, libRepo.Create(ctx, lib))

	mk := func(id, genre string) {
		tr := newTestTrack(id, lib.ID, "T"+id, "A", "B")
		tr.Genre = genre
		{
			_, uerr := trackRepo.Upsert(ctx, tr)
			require.NoError(t, uerr)
		}
	}
	mk("ga-rockA", "Rock")
	mk("ga-rockB", "Rock") // never played → contributes nothing
	mk("ga-jazzC", "Jazz")
	mk("ga-noGenre", "") // empty genre → excluded

	for range 3 {
		require.NoError(t, playRepo.IncrementPlay(ctx, string(p.ID), "ga-rockA"))
	}
	for range 5 {
		require.NoError(t, playRepo.IncrementPlay(ctx, string(p.ID), "ga-jazzC"))
	}

	aff, err := playRepo.GenreAffinityForProfile(ctx, string(p.ID))
	require.NoError(t, err)
	assert.InDelta(t, 3, aff["rock"], 0.001, "rock = 3 plays (lowercased, only the played track counts)")
	assert.InDelta(t, 5, aff["jazz"], 0.001)
	_, hasEmpty := aff[""]
	assert.False(t, hasEmpty, "empty-genre tracks must be excluded")

	// A different profile has no affinity.
	other, err := playRepo.GenreAffinityForProfile(ctx, "prf-nonexistent")
	require.NoError(t, err)
	assert.Empty(t, other)
}

func TestMusicPlayRepo_IncrementAndCounts(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	libRepo := postgres.NewLibraryRepository(db)
	trackRepo := postgres.NewMusicTrackRepository(db)
	playRepo := postgres.NewMusicPlayRepository(db)
	ctx := context.Background()

	u := newTestUser("mpu1")
	require.NoError(t, userRepo.Create(ctx, u))
	p := newTestProfile("mpp1", u.ID)
	require.NoError(t, profileRepo.Create(ctx, p))
	lib := newTestLibrary("mplib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	tr := newTestTrack("mptr1", lib.ID, "Song", "Artist", "Album")
	{
		_, uerr := trackRepo.Upsert(ctx, tr)
		require.NoError(t, uerr)
	}

	// No plays yet.
	counts, err := playRepo.PlayCountsForProfile(ctx, string(p.ID))
	require.NoError(t, err)
	assert.Equal(t, 0, counts[tr.ID])

	require.NoError(t, playRepo.IncrementPlay(ctx, string(p.ID), tr.ID))
	require.NoError(t, playRepo.IncrementPlay(ctx, string(p.ID), tr.ID))

	counts, err = playRepo.PlayCountsForProfile(ctx, string(p.ID))
	require.NoError(t, err)
	assert.Equal(t, 2, counts[tr.ID], "two listens should accumulate to play_count 2")

	// A different profile sees nothing.
	other, err := playRepo.PlayCountsForProfile(ctx, "prf-nonexistent")
	require.NoError(t, err)
	assert.Empty(t, other)
}
