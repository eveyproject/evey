package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTrack(id string, libID shared.LibraryID, title, artist, album string) music.Track {
	return music.Track{
		ID:          id,
		LibraryID:   shared.LibraryID(libID),
		Title:       title,
		Artist:      artist,
		Album:       album,
		AlbumArtist: artist,
		Year:        2024,
		TrackNumber: 1,
		DiskNumber:  1,
		Duration:    240.5,
		FilePath:    "/music/" + id + ".flac",
		FileSize:    5000000,
		FileHash:    "hash-" + id,
		ModTime:     time.Now().UTC().Truncate(time.Millisecond),
		MimeType:    "audio/flac",
		Bitrate:     320,
		SampleRate:  44100,
		Channels:    2,
		Genre:       "Rock",
		CoverArtURL: "/art/" + id,
	}
}

func TestMusicTrackRepo_Upsert_GetByID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib1")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	track := newTestTrack("mt1", lib.ID, "Song One", "Artist A", "Album X")
	require.NoError(t, repo.Upsert(ctx, track))

	got, err := repo.GetByID(ctx, "mt1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Song One", got.Title)
	assert.Equal(t, "Artist A", got.Artist)
}

func TestMusicTrackRepo_Delete(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib2")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	track := newTestTrack("mt2", lib.ID, "Song Del", "Art", "Alb")
	require.NoError(t, repo.Upsert(ctx, track))
	require.NoError(t, repo.Delete(ctx, "mt2"))

	got, err := repo.GetByID(ctx, "mt2")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestMusicTrackRepo_ListByLibrary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib3")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestTrack("mt3a", lib.ID, "Song A", "Art", "Alb")))
	t2 := newTestTrack("mt3b", lib.ID, "Song B", "Art", "Alb")
	t2.FilePath = "/music/mt3b.flac"
	require.NoError(t, repo.Upsert(ctx, t2))

	tracks, total, err := repo.ListByLibrary(ctx, lib.ID, 10, 0, "")
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, tracks, 2)
}

func TestMusicTrackRepo_ListByAlbum(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib4")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestTrack("mt4a", lib.ID, "Track 1", "Band", "MyAlbum")))
	t2 := newTestTrack("mt4b", lib.ID, "Track 2", "Band", "MyAlbum")
	t2.FilePath = "/music/mt4b.flac"
	t2.TrackNumber = 2
	require.NoError(t, repo.Upsert(ctx, t2))

	tracks, err := repo.ListByAlbum(ctx, "MyAlbum", "Band")
	require.NoError(t, err)
	assert.Len(t, tracks, 2)
}

func TestMusicTrackRepo_ListByArtist(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib5")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestTrack("mt5a", lib.ID, "S1", "SoloArtist", "A1")))
	t2 := newTestTrack("mt5b", lib.ID, "S2", "SoloArtist", "A2")
	t2.FilePath = "/music/mt5b.flac"
	require.NoError(t, repo.Upsert(ctx, t2))

	tracks, total, err := repo.ListByArtist(ctx, "SoloArtist", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, tracks, 2)
}

func TestMusicTrackRepo_Search(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib6")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestTrack("mt6", lib.ID, "Searchable Song", "Art", "Alb")))

	tracks, err := repo.Search(ctx, lib.ID, "Searchable", 10)
	require.NoError(t, err)
	assert.Len(t, tracks, 1)
}

func TestMusicTrackRepo_GetAlbums(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib7")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestTrack("mt7a", lib.ID, "T1", "TheArtist", "TheAlbum")))
	t2 := newTestTrack("mt7b", lib.ID, "T2", "TheArtist", "TheAlbum")
	t2.FilePath = "/music/mt7b.flac"
	require.NoError(t, repo.Upsert(ctx, t2))

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, albums, 1)
	assert.Equal(t, "TheAlbum", albums[0].Title)
	assert.Equal(t, 2, albums[0].TrackCount)
}

func TestMusicTrackRepo_GetArtists(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib8")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestTrack("mt8", lib.ID, "T1", "UniqueArtist", "A")))

	artists, err := repo.GetArtists(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, artists, 1)
	assert.Equal(t, "UniqueArtist", artists[0].Name)
}

func TestMusicTrackRepo_ListAllByLibrary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib9")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestTrack("mt9a", lib.ID, "All1", "A", "B")))
	t2 := newTestTrack("mt9b", lib.ID, "All2", "A", "B")
	t2.FilePath = "/music/mt9b.flac"
	require.NoError(t, repo.Upsert(ctx, t2))

	tracks, err := repo.ListAllByLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, tracks, 2)
}

func TestMusicTrackRepo_UpdateFilePath(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib10")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	require.NoError(t, repo.Upsert(ctx, newTestTrack("mt10", lib.ID, "MovedSong", "A", "B")))

	newMod := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.UpdateFilePath(ctx, "mt10", "/music/new_path.flac", newMod))

	got, err := repo.GetByID(ctx, "mt10")
	require.NoError(t, err)
	assert.Equal(t, "/music/new_path.flac", got.FilePath)
}
