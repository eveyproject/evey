package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/shared"
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
	{
		_, uerr := repo.Upsert(ctx, track)
		require.NoError(t, uerr)
	}

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
	{
		_, uerr := repo.Upsert(ctx, track)
		require.NoError(t, uerr)
	}
	require.NoError(t, repo.Delete(ctx, "mt2"))

	got, err := repo.GetByID(ctx, "mt2")
	assert.ErrorIs(t, err, shared.ErrNotFound)
	assert.Nil(t, got)
}

// Upsert must return the PERSISTED id: a re-scan of the same file (ON CONFLICT
// by file_path) keeps the existing row's id, so a caller that minted a fresh
// id must get the existing one back, not its throwaway. The music scanner
// relies on this to publish events that reference a row that actually exists.
func TestMusicTrackRepo_UpsertReturnsExistingIDOnConflict(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-conflict-lib")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	first := newTestTrack("mt-keep", lib.ID, "Song", "Art", "Alb")
	gotID, err := repo.Upsert(ctx, first)
	require.NoError(t, err)
	require.Equal(t, "mt-keep", gotID, "fresh insert returns the inserted id")

	// Same file_path, different (throwaway) id - the conflict must keep mt-keep.
	rescan := newTestTrack("mt-throwaway", lib.ID, "Song (retag)", "Art", "Alb")
	rescan.FilePath = first.FilePath
	gotID, err = repo.Upsert(ctx, rescan)
	require.NoError(t, err)
	require.Equal(t, "mt-keep", gotID, "conflict on file_path returns the existing id, not the throwaway")

	// And the throwaway id must not have created a second row.
	_, err = repo.GetByID(ctx, "mt-throwaway")
	require.ErrorIs(t, err, shared.ErrNotFound)
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

	{
		_, uerr := repo.Upsert(ctx, newTestTrack("mt3a", lib.ID, "Song A", "Art", "Alb"))
		require.NoError(t, uerr)
	}
	t2 := newTestTrack("mt3b", lib.ID, "Song B", "Art", "Alb")
	t2.FilePath = "/music/mt3b.flac"
	{
		_, uerr := repo.Upsert(ctx, t2)
		require.NoError(t, uerr)
	}

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

	{
		_, uerr := repo.Upsert(ctx, newTestTrack("mt4a", lib.ID, "Track 1", "Band", "MyAlbum"))
		require.NoError(t, uerr)
	}
	t2 := newTestTrack("mt4b", lib.ID, "Track 2", "Band", "MyAlbum")
	t2.FilePath = "/music/mt4b.flac"
	t2.TrackNumber = 2
	{
		_, uerr := repo.Upsert(ctx, t2)
		require.NoError(t, uerr)
	}

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

	{
		_, uerr := repo.Upsert(ctx, newTestTrack("mt5a", lib.ID, "S1", "SoloArtist", "A1"))
		require.NoError(t, uerr)
	}
	t2 := newTestTrack("mt5b", lib.ID, "S2", "SoloArtist", "A2")
	t2.FilePath = "/music/mt5b.flac"
	{
		_, uerr := repo.Upsert(ctx, t2)
		require.NoError(t, uerr)
	}

	tracks, total, err := repo.ListByArtist(ctx, "SoloArtist", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, tracks, 2)
}

// The keyset ORDER BY is (album_artist, album, disk_number, track_number, id)
// ascending. mtcur1/mtcur2 deliberately tie on every column but id - id is
// the only thing that makes the tuple unique. A limit of 2 forces the page
// boundary to land inside that tie.
func TestMusicTrackRepo_ListByLibraryCursor_PageBoundary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-cursor-lib")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	tracks := []music.Track{
		newTestTrack("mtcur1", lib.ID, "T1", "A", "Alb1"),
		newTestTrack("mtcur2", lib.ID, "T2", "A", "Alb1"), // ties mtcur1 on (album_artist, album, disk, track)
		newTestTrack("mtcur3", lib.ID, "T3", "A", "Alb1"),
		newTestTrack("mtcur4", lib.ID, "T4", "A", "Alb2"),
		newTestTrack("mtcur5", lib.ID, "T5", "B", "Alb1"),
	}
	tracks[2].TrackNumber = 2 // mtcur3 sorts after the mtcur1/mtcur2 tie

	ids := make([]string, 0, len(tracks))
	for _, tr := range tracks {
		tr.FilePath = "/music/" + tr.ID + ".flac"
		_, err := repo.Upsert(ctx, tr)
		require.NoError(t, err)
		ids = append(ids, tr.ID)
	}

	var collected []string
	var cursor *music.TrackCursor
	// A page count cap, not a break-on-empty-page loop: a seek predicate bug
	// that fails to exclude an already-returned row turns this into an
	// infinite loop against a live DB connection instead of a fast failure.
	for range len(tracks) + 2 {
		page, err := repo.ListByLibraryCursor(ctx, lib.ID, "", cursor, 2)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		for _, tr := range page {
			collected = append(collected, tr.ID)
		}
		last := page[len(page)-1]
		cursor = &music.TrackCursor{
			AlbumArtist: last.AlbumArtist, Album: last.Album,
			DiskNumber: last.DiskNumber, TrackNumber: last.TrackNumber, ID: last.ID,
		}
		if len(page) < 2 {
			break
		}
	}

	assert.ElementsMatch(t, ids, collected, "keyset pagination must return every row exactly once")
	assert.Len(t, collected, len(ids), "no row should be skipped or repeated at a page boundary")

	want := []string{"mtcur1", "mtcur2", "mtcur3", "mtcur4", "mtcur5"}
	assert.Equal(t, want, collected)

	offsetTracks, total, err := repo.ListByLibrary(ctx, lib.ID, 100, 0, "")
	require.NoError(t, err)
	assert.Equal(t, len(ids), total)
	var offsetIDs []string
	for _, tr := range offsetTracks {
		offsetIDs = append(offsetIDs, tr.ID)
	}
	assert.Equal(t, want, offsetIDs, "keyset and offset queries must agree on ordering")
}

// The keyset ORDER BY is (album, disk_number, track_number, id) ascending.
// mtart1/mtart2 tie on every column but id.
func TestMusicTrackRepo_ListByArtistCursor_PageBoundary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-cursor-artist-lib")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	all := []music.Track{
		newTestTrack("mtart1", lib.ID, "T1", "SharedArtist", "Alb1"),
		newTestTrack("mtart2", lib.ID, "T2", "SharedArtist", "Alb1"), // ties mtart1 on (album, disk, track)
		newTestTrack("mtart-other", lib.ID, "T3", "DifferentArtist", "Alb1"),
	}
	for _, tr := range all {
		tr.FilePath = "/music/" + tr.ID + ".flac"
		_, err := repo.Upsert(ctx, tr)
		require.NoError(t, err)
	}

	var collected []string
	var cursor *music.ArtistTrackCursor
	for range len(all) + 2 { // page count cap, see ListByLibraryCursor's test for why
		page, err := repo.ListByArtistCursor(ctx, "SharedArtist", cursor, 1)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		for _, tr := range page {
			collected = append(collected, tr.ID)
		}
		last := page[len(page)-1]
		cursor = &music.ArtistTrackCursor{Album: last.Album, DiskNumber: last.DiskNumber, TrackNumber: last.TrackNumber, ID: last.ID}
	}

	assert.ElementsMatch(t, []string{"mtart1", "mtart2"}, collected)
	assert.NotContains(t, collected, "mtart-other")
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

	{
		_, uerr := repo.Upsert(ctx, newTestTrack("mt6", lib.ID, "Searchable Song", "Art", "Alb"))
		require.NoError(t, uerr)
	}

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

	// Both tracks live in the same folder - one album, regardless of tags.
	t1 := newTestTrack("mt7a", lib.ID, "T1", "TheArtist", "TheAlbum")
	t1.FilePath = "/music/the-album/mt7a.flac"
	{
		_, uerr := repo.Upsert(ctx, t1)
		require.NoError(t, uerr)
	}
	t2 := newTestTrack("mt7b", lib.ID, "T2", "TheArtist", "TheAlbum")
	t2.FilePath = "/music/the-album/mt7b.flac"
	{
		_, uerr := repo.Upsert(ctx, t2)
		require.NoError(t, uerr)
	}

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	assert.Len(t, albums, 1)
	assert.Equal(t, "TheAlbum", albums[0].Title)
	assert.Equal(t, 2, albums[0].TrackCount)
	// ID is the opaque, folder-derived public id (not the raw folder path).
	assert.Equal(t, music.AlbumID("/music/the-album"), albums[0].ID)
}

// Régression prod 2026-07-25 : la pochette d'album était `MIN(cover_art_url)`,
// donc un choix LEXICOGRAPHIQUE. Or "/api/v1/music/album-art/…" < "/api/v1/music/
// tracks/…" ('a' < 't'), si bien qu'une seule ligne portant une pochette iTunes
// écrasait les pochettes EMBARQUÉES correctes de tout le dossier. Sur la
// bibliothèque du user, l'album "Orchard Essentials" de Neon Harbor embarquait la
// bonne pochette dans ses MP3 et affichait quand même celle de Mixed Signals,
// devinée par iTunes. L'artwork embarqué dans le fichier est la source
// autoritaire : il doit gagner.
func TestMusicTrackRepo_GetAlbums_EmbeddedArtBeatsFetchedArt(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib-art")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	// Une piste porte une pochette embarquée (CoverArtURL non vide à l'upsert),
	// l'autre non. Seul SetAlbumCoverArtURL sait écrire une URL album-art, donc
	// on reproduit le geste exact du fetcher iTunes.
	embedded := newTestTrack("mt-art-a", lib.ID, "At The Close", "Neon Harbor", "Orchard Essentials")
	embedded.FilePath = "/music/essentials/01.mp3"
	embedded.CoverArtURL = "/api/v1/music/tracks/placeholder/art" // non vide = le fichier a une pochette
	persistedA, err := repo.Upsert(ctx, embedded)
	require.NoError(t, err)

	noArt := newTestTrack("mt-art-b", lib.ID, "Some Other Track", "Neon Harbor", "Orchard Essentials")
	noArt.FilePath = "/music/essentials/02.mp3"
	noArt.CoverArtURL = ""
	_, err = repo.Upsert(ctx, noArt)
	require.NoError(t, err)

	require.NoError(t, repo.SetAlbumCoverArtURL(ctx, lib.ID, "/music/essentials",
		"/api/v1/music/album-art/deadbeefdeadbeef"))

	// 1. Le fetcher ne doit pas avoir écrasé la pochette embarquée.
	trA, err := repo.GetByID(ctx, persistedA)
	require.NoError(t, err)
	require.NotNil(t, trA)
	assert.Equal(t, "/api/v1/music/tracks/"+persistedA+"/art", trA.CoverArtURL,
		"SetAlbumCoverArtURL ne doit jamais écraser une pochette venue du fichier")

	// 2. Et l'album doit exposer la pochette embarquée, pas celle devinée.
	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, albums, 1)
	assert.Equal(t, "/api/v1/music/tracks/"+persistedA+"/art", albums[0].CoverArtURL,
		"la pochette embarquée doit gagner contre une pochette récupérée en externe")
}

// One track without a year tag (stored as 0) must not hide the album's year:
// MIN(year) picked the 0 and every client showed no year at all.
func TestMusicTrackRepo_GetAlbums_UntaggedYearDoesNotHideTheAlbumYear(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()
	lib := newTestLibrary("mt-lib-year")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	tagged := newTestTrack("mt-year-a", lib.ID, "Tagged", "Neon Harbor", "Harbor Lights")
	tagged.FilePath = "/music/lights/01.mp3"
	tagged.Year = 2011
	_, err := repo.Upsert(ctx, tagged)
	require.NoError(t, err)
	untagged := newTestTrack("mt-year-b", lib.ID, "Untagged", "Neon Harbor", "Harbor Lights")
	untagged.FilePath = "/music/lights/02.mp3"
	untagged.Year = 0
	_, err = repo.Upsert(ctx, untagged)
	require.NoError(t, err)

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, albums, 1)
	assert.Equal(t, 2011, albums[0].Year)

	// Nobody tagged: the album still scans, with no year.
	lib2 := newTestLibrary("mt-lib-noyear")
	lib2.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib2))
	bare := newTestTrack("mt-year-c", lib2.ID, "Bare", "Neon Harbor", "No Date")
	bare.FilePath = "/music/nodate/01.mp3"
	bare.Year = 0
	_, err = repo.Upsert(ctx, bare)
	require.NoError(t, err)
	albums, err = repo.GetAlbums(ctx, lib2.ID)
	require.NoError(t, err)
	require.Len(t, albums, 1)
	assert.Equal(t, 0, albums[0].Year)
}

// Sans aucune pochette embarquée, l'artwork externe reste servi : le correctif
// ci-dessus est une PRÉFÉRENCE, pas un rejet de l'artwork iTunes.
func TestMusicTrackRepo_GetAlbums_FetchedArtStillServedWithoutEmbedded(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib-art2")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	tr := newTestTrack("mt-art-c", lib.ID, "Some Song", "Skyrider", "Song Of The Valley")
	tr.FilePath = "/music/crime/01.flac"
	tr.CoverArtURL = "" // aucune pochette dans le fichier
	_, err := repo.Upsert(ctx, tr)
	require.NoError(t, err)

	require.NoError(t, repo.SetAlbumCoverArtURL(ctx, lib.ID, "/music/crime",
		"/api/v1/music/album-art/cafecafecafecafe"))

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, albums, 1)
	assert.Equal(t, "/api/v1/music/album-art/cafecafecafecafe", albums[0].CoverArtURL)
}

// TestMusicTrackRepo_GetArtists_AlbumCountByFolder proves the artist album count
// matches the folder-based album identity GetAlbums uses: one album tag spread
// over two folders is two albums, not one (the old COUNT(DISTINCT album) reported
// one).
func TestMusicTrackRepo_GetArtists_AlbumCountByFolder(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib-art")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	// Same album tag "The Wall" in two folders → two albums for the artist.
	t1 := newTestTrack("art1", lib.ID, "T1", "Pink Floyd", "The Wall")
	t1.FilePath = "/music/wall-d1/art1.flac"
	{
		_, uerr := repo.Upsert(ctx, t1)
		require.NoError(t, uerr)
	}
	t2 := newTestTrack("art2", lib.ID, "T2", "Pink Floyd", "The Wall")
	t2.FilePath = "/music/wall-d2/art2.flac"
	{
		_, uerr := repo.Upsert(ctx, t2)
		require.NoError(t, uerr)
	}

	artists, err := repo.GetArtists(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, artists, 1)
	assert.Equal(t, "Pink Floyd", artists[0].Name)
	assert.Equal(t, 2, artists[0].TrackCount)
	assert.Equal(t, 2, artists[0].AlbumCount, "two folders sharing one album tag must count as two albums")
}

// Regression for the NEON HARBOR / MC Zed featuring bug: 24 tracks share
// album_artist="Neon Harbor", 1 track overrides it to "MC Zed". Before
// the folder-based fix, MIN(album_artist) picked "MC Zed" (J < L) on
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

	folder := "/media4/NEON HARBOR - Essentials"
	// 24 tracks, album_artist = "Neon Harbor"
	for i := 0; i < 24; i++ {
		tr := newTestTrack(fmt.Sprintf("lp-%02d", i), lib.ID, fmt.Sprintf("Song %02d", i), "Neon Harbor", "Orchard Essentials")
		tr.AlbumArtist = "Neon Harbor"
		tr.FilePath = fmt.Sprintf("%s/%02d.flac", folder, i)
		{
			_, uerr := repo.Upsert(ctx, tr)
			require.NoError(t, uerr)
		}
	}
	// 1 featuring track, album_artist = "MC Zed" (would lose to MIN() on
	// album_artist alphabetical ordering without the folder fix).
	feat := newTestTrack("lp-17-feat", lib.ID, "Some Track (feat. MC Zed)", "MC Zed", "Orchard Essentials")
	feat.AlbumArtist = "MC Zed"
	feat.FilePath = folder + "/17-feat.flac"
	{
		_, uerr := repo.Upsert(ctx, feat)
		require.NoError(t, uerr)
	}

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, albums, 1, "folder should collapse to 1 album")
	assert.Equal(t, 25, albums[0].TrackCount)
	assert.Equal(t, "Neon Harbor", albums[0].Artist, "majority album_artist should win")
	assert.Equal(t, "Orchard Essentials", albums[0].Title)
	assert.Equal(t, music.AlbumID(folder), albums[0].ID)
}

// Two folders that share the same album tag ("Orchard Essentials") must
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
		t1 := newTestTrack(fmt.Sprintf("a-%d", i), lib.ID, fmt.Sprintf("Track %d", i), "Artist A", "Orchard Essentials")
		t1.FilePath = fmt.Sprintf("%s/%d.flac", folder1, i)
		{
			_, uerr := repo.Upsert(ctx, t1)
			require.NoError(t, uerr)
		}
		t2 := newTestTrack(fmt.Sprintf("b-%d", i), lib.ID, fmt.Sprintf("Track %d", i), "Artist B", "Orchard Essentials")
		t2.FilePath = fmt.Sprintf("%s/%d.flac", folder2, i)
		{
			_, uerr := repo.Upsert(ctx, t2)
			require.NoError(t, uerr)
		}
	}

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, albums, 2, "same album tag in different folders = 2 albums")
	ids := map[string]int{}
	for _, a := range albums {
		ids[a.ID] = a.TrackCount
	}
	// Keyed by the opaque public id derived from each distinct folder.
	assert.Equal(t, 5, ids[music.AlbumID(folder1)])
	assert.Equal(t, 5, ids[music.AlbumID(folder2)])
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
	{
		_, uerr := repo.Upsert(ctx, t1)
		require.NoError(t, uerr)
	}
	t2 := newTestTrack("fa-2", lib.ID, "T2", "Y", "AlbumTag")
	t2.FilePath = folder + "/02.flac"
	t2.TrackNumber = 2
	{
		_, uerr := repo.Upsert(ctx, t2)
		require.NoError(t, uerr)
	}
	// Out-of-folder track with same album tag must not leak in.
	other := newTestTrack("fa-3", lib.ID, "T3", "X", "AlbumTag")
	other.FilePath = "/music/other-folder/03.flac"
	{
		_, uerr := repo.Upsert(ctx, other)
		require.NoError(t, uerr)
	}

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

	{
		_, uerr := repo.Upsert(ctx, newTestTrack("mt8", lib.ID, "T1", "UniqueArtist", "A"))
		require.NoError(t, uerr)
	}

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
		{
			_, uerr := repo.Upsert(ctx, newTestTrack(id, lib.ID, "T"+id, "A", "B"))
			require.NoError(t, uerr)
		}
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

	{
		_, uerr := repo.Upsert(ctx, newTestTrack("mt9a", lib.ID, "All1", "A", "B"))
		require.NoError(t, uerr)
	}
	t2 := newTestTrack("mt9b", lib.ID, "All2", "A", "B")
	t2.FilePath = "/music/mt9b.flac"
	{
		_, uerr := repo.Upsert(ctx, t2)
		require.NoError(t, uerr)
	}

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

	{
		_, uerr := repo.Upsert(ctx, newTestTrack("mt10", lib.ID, "MovedSong", "A", "B"))
		require.NoError(t, uerr)
	}

	newMod := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.UpdateFilePath(ctx, "mt10", "/music/new_path.flac", newMod))

	got, err := repo.GetByID(ctx, "mt10")
	require.NoError(t, err)
	assert.Equal(t, "/music/new_path.flac", got.FilePath)
}

// Same contract as the photo counter: the stats endpoint replaced a
// list-one-row-and-read-the-total call with this, so it must agree exactly and
// stay scoped to its own library.
func TestMusicTrackRepo_CountByLibrary_AgreesWithListTotal(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-count-a")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))
	other := newTestLibrary("mt-count-b")
	other.Type = "music"
	require.NoError(t, libRepo.Create(ctx, other))

	for i, id := range []string{"mc1", "mc2", "mc3"} {
		tr := newTestTrack(id, lib.ID, "Song "+id, "Art", "Alb")
		tr.FilePath = fmt.Sprintf("/music/%s-%d.flac", id, i)
		_, uerr := repo.Upsert(ctx, tr)
		require.NoError(t, uerr)
	}
	stray := newTestTrack("mc-other", other.ID, "Elsewhere", "Art", "Alb")
	stray.FilePath = "/music/mc-other.flac"
	_, uerr := repo.Upsert(ctx, stray)
	require.NoError(t, uerr)

	_, total, err := repo.ListByLibrary(ctx, lib.ID, 1, 0, "")
	require.NoError(t, err)

	n, err := repo.CountByLibrary(ctx, lib.ID)
	require.NoError(t, err)
	assert.Equal(t, total, n, "the counter disagrees with the list total it replaced")
	assert.Equal(t, 3, n)

	nOther, err := repo.CountByLibrary(ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, nOther)
}

// Prod 2026-08-26 : un mp3 sans aucun tag ("Star Profile - Audio Documentary")
// donnait un album au TITRE VIDE - en-tête blanc dans les trois clients. Le
// dossier EST l'identité de l'album (c'est déjà la clé de groupage) : quand
// aucune piste du groupe ne porte de tag album, son nom est le meilleur
// affichage disponible, pas la chaîne vide.
func TestMusicTrackRepo_GetAlbums_UntaggedFolderFallsBackToFolderName(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	repo := postgres.NewMusicTrackRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mt-lib-untagged")
	lib.Type = "music"
	require.NoError(t, libRepo.Create(ctx, lib))

	tr := newTestTrack("mt-untag-1", lib.ID, "Star Profile - 01 - Audio Documentary.mp3", "", "")
	tr.AlbumArtist = ""
	tr.FilePath = "/music/audio documentary/Star Profile - 01 - Audio Documentary.mp3"
	tr.AlbumFolder = "/music/audio documentary"
	_, uerr := repo.Upsert(ctx, tr)
	require.NoError(t, uerr)

	albums, err := repo.GetAlbums(ctx, lib.ID)
	require.NoError(t, err)
	require.Len(t, albums, 1)
	assert.Equal(t, "audio documentary", albums[0].Title,
		"an all-untagged folder must display its folder name, not an empty header")
}

// The cover-art fetcher's view of a library: one candidate per album FOLDER
// (tags voted by mode, blanks ignored), a folder with no usable album or
// artist skipped, an embedded cover flagged so the fetcher leaves it alone;
// plus the per-album track listing and the fetched-URL stamp.
func TestMusicTrackRepo_AlbumFolders_AllAlbums_CoverURL(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("mfoldlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	repo := postgres.NewMusicTrackRepository(db)

	mk := func(id, folder, title, artist, album, cover string) {
		t.Helper()
		tr := newTestTrack(id, lib.ID, title, artist, album)
		tr.AlbumFolder = folder
		tr.CoverArtURL = cover
		_, err := repo.Upsert(ctx, tr)
		require.NoError(t, err)
	}
	mk("mf-a1", "/music/A", "One", "Artist A", "Album A", "")
	mk("mf-a2", "/music/A", "Two", "Artist A", "Album A", "")
	mk("mf-a3", "/music/A", "Three", "Artist A", "", "") // untagged stray: the mode ignores the blank
	mk("mf-b1", "/music/B", "Solo", "Artist B", "Album B", "/api/v1/music/tracks/mf-b1/art")
	mk("mf-c1", "/music/C", "Untagged", "", "", "")     // no album, no artist: not fetchable
	mk("mf-d1", "", "Loose", "Artist D", "Album D", "") // no folder given: derived from the file's directory

	folders, err := repo.ListAlbumFolders(ctx, lib.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"/music/A", "/music/B", "/music/C", "/music"}, folders, "every folder once, the blank one derived from the path")

	albums, err := repo.ListAllAlbums(ctx, lib.ID)
	require.NoError(t, err)
	byFolder := map[string]music.AlbumKey{}
	for _, a := range albums {
		byFolder[a.AlbumFolder] = a
	}
	require.Contains(t, byFolder, "/music/A")
	assert.Equal(t, music.AlbumKey{AlbumFolder: "/music/A", Artist: "Artist A", Album: "Album A"}, byFolder["/music/A"])
	require.Contains(t, byFolder, "/music/B")
	assert.True(t, byFolder["/music/B"].HasEmbeddedArt, "a track carrying its own cover blocks the external fetch")
	assert.NotContains(t, byFolder, "/music/C", "nothing to search with")

	tracks, err := repo.ListByLibraryAndAlbum(ctx, lib.ID, "Album A")
	require.NoError(t, err)
	assert.Len(t, tracks, 2)

	require.NoError(t, repo.SetTrackCoverArtURL(ctx, "mf-a1", "/art/fetched-a"))
	got, err := repo.GetByID(ctx, "mf-a1")
	require.NoError(t, err)
	assert.Equal(t, "/art/fetched-a", got.CoverArtURL)
	require.NoError(t, repo.SetTrackCoverArtURL(ctx, "ghost", "/x"), "an unknown track is a no-op")
}
