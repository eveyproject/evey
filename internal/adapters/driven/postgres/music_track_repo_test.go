package postgres_test

import (
	"context"
	"fmt"
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
	assert.ErrorIs(t, err, shared.ErrNotFound)
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

	// Both tracks live in the same folder — one album, regardless of tags.
	t1 := newTestTrack("mt7a", lib.ID, "T1", "TheArtist", "TheAlbum")
	t1.FilePath = "/music/the-album/mt7a.flac"
	require.NoError(t, repo.Upsert(ctx, t1))
	t2 := newTestTrack("mt7b", lib.ID, "T2", "TheArtist", "TheAlbum")
	t2.FilePath = "/music/the-album/mt7b.flac"
	require.NoError(t, repo.Upsert(ctx, t2))

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, albums, 1)
	assert.Equal(t, "TheAlbum", albums[0].Title)
	assert.Equal(t, 2, albums[0].TrackCount)
	assert.Equal(t, "/music/the-album", albums[0].ID)
}

// Regression for the LINKIN PARK / Jay-Z featuring bug: 24 tracks share
// album_artist="Linkin Park", 1 track overrides it to "Jay-Z". Before
// the folder-based fix, MIN(album_artist) picked "Jay-Z" (J < L) on
// every call. With mode() on (album_folder), the majority wins.
func TestMusicTrackRepo_GetAlbums_MajorityArtistWinsOnFeaturing(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib-linkin")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	folder := "/media4/LINKIN PARK - Essentials"
	// 24 tracks, album_artist = "Linkin Park"
	for i := 0; i < 24; i++ {
		tr := newTestTrack(fmt.Sprintf("lp-%02d", i), lib.ID, fmt.Sprintf("Song %02d", i), "Linkin Park", "Apple Essentials")
		tr.AlbumArtist = "Linkin Park"
		tr.FilePath = fmt.Sprintf("%s/%02d.flac", folder, i)
		require.NoError(t, repo.Upsert(ctx, tr))
	}
	// 1 featuring track, album_artist = "Jay-Z" (would lose to MIN() on
	// album_artist alphabetical ordering without the folder fix).
	feat := newTestTrack("lp-17-feat", lib.ID, "Numb/Encore (feat. Jay-Z)", "Jay-Z", "Apple Essentials")
	feat.AlbumArtist = "Jay-Z"
	feat.FilePath = folder + "/17-feat.flac"
	require.NoError(t, repo.Upsert(ctx, feat))

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, albums, 1, "folder should collapse to 1 album")
	assert.Equal(t, 25, albums[0].TrackCount)
	assert.Equal(t, "Linkin Park", albums[0].Artist, "majority album_artist should win")
	assert.Equal(t, "Apple Essentials", albums[0].Title)
	assert.Equal(t, folder, albums[0].ID)
}

// Two folders that share the same album tag ("Apple Essentials") must
// stay distinct albums. This is the mirror of the previous test: the
// tag-based grouping would have collapsed them; folder-based keeps them.
func TestMusicTrackRepo_GetAlbums_SameTagDifferentFoldersStaySplit(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib-apple")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	folder1 := "/media/Artist A - Essentials"
	folder2 := "/media/Artist B - Essentials"
	for i := 0; i < 5; i++ {
		t1 := newTestTrack(fmt.Sprintf("a-%d", i), lib.ID, fmt.Sprintf("Track %d", i), "Artist A", "Apple Essentials")
		t1.FilePath = fmt.Sprintf("%s/%d.flac", folder1, i)
		require.NoError(t, repo.Upsert(ctx, t1))
		t2 := newTestTrack(fmt.Sprintf("b-%d", i), lib.ID, fmt.Sprintf("Track %d", i), "Artist B", "Apple Essentials")
		t2.FilePath = fmt.Sprintf("%s/%d.flac", folder2, i)
		require.NoError(t, repo.Upsert(ctx, t2))
	}

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, albums, 2, "same album tag in different folders = 2 albums")
	ids := map[string]int{}
	for _, a := range albums {
		ids[a.ID] = a.TrackCount
	}
	assert.Equal(t, 5, ids[folder1])
	assert.Equal(t, 5, ids[folder2])
}

// Exercises the new folder-keyed track listing used by the UI to show
// "all tracks of this album".
func TestMusicTrackRepo_ListByAlbumFolder(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib-folder")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	folder := "/music/folder-album"
	t1 := newTestTrack("fa-1", lib.ID, "T1", "X", "AlbumTag")
	t1.FilePath = folder + "/01.flac"
	t1.TrackNumber = 1
	require.NoError(t, repo.Upsert(ctx, t1))
	t2 := newTestTrack("fa-2", lib.ID, "T2", "Y", "AlbumTag")
	t2.FilePath = folder + "/02.flac"
	t2.TrackNumber = 2
	require.NoError(t, repo.Upsert(ctx, t2))
	// Out-of-folder track with same album tag must not leak in.
	other := newTestTrack("fa-3", lib.ID, "T3", "X", "AlbumTag")
	other.FilePath = "/music/other-folder/03.flac"
	require.NoError(t, repo.Upsert(ctx, other))

	tracks, err := repo.ListByAlbumFolder(ctx, lib.ID, folder)
	require.NoError(t, err)
	require.Len(t, tracks, 2)
	// Verify disk/track ordering.
	assert.Equal(t, "T1", tracks[0].Title)
	assert.Equal(t, "T2", tracks[1].Title)
	// album_folder should round-trip.
	assert.Equal(t, folder, tracks[0].AlbumFolder)
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

func TestMusicTrackRepo_SampleByLibrary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("smp-lib")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))
	for _, id := range []string{"smp1", "smp2", "smp3", "smp4", "smp5"} {
		require.NoError(t, repo.Upsert(ctx, newTestTrack(id, lib.ID, "T"+id, "A", "B")))
	}

	// Capped below the row count → exactly limit rows.
	got, err := repo.SampleByLibrary(ctx, lib.ID, 3)
	require.NoError(t, err)
	assert.Len(t, got, 3, "sample must be capped at limit")

	// Limit above the count → all rows.
	all, err := repo.SampleByLibrary(ctx, lib.ID, 100)
	require.NoError(t, err)
	assert.Len(t, all, 5)

	// Zero limit → empty, no query.
	none, err := repo.SampleByLibrary(ctx, lib.ID, 0)
	require.NoError(t, err)
	assert.Empty(t, none)
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
