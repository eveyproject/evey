package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type MusicTrackRepository struct {
	db *sql.DB
}

func NewMusicTrackRepository(db *sql.DB) *MusicTrackRepository {
	return &MusicTrackRepository{db: db}
}

func (r *MusicTrackRepository) Upsert(ctx context.Context, t music.Track) (string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.Upsert")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	span.SetAttributes(attribute.String("music_track.id", t.ID))
	// Derive album identity from the parent directory if the caller did
	// not set it. The repo is the last line of defense: we never want a
	// row without an album_folder, because GetAlbums groups on it.
	if t.AlbumFolder == "" && t.FilePath != "" {
		t.AlbumFolder = filepath.Dir(t.FilePath)
	}
	hasCover := t.CoverArtURL != ""
	var persistedID string
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `
		INSERT INTO music_tracks (
			id, library_id, title, artist, album, album_artist,
			year, track_number, disk_number, duration, file_path,
			file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
			genre, album_folder, cover_art_url, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
			CASE WHEN $21::bool THEN '/api/v1/music/tracks/' || $1 || '/art' ELSE '' END,
			NOW(),NOW())
		ON CONFLICT (file_path) DO UPDATE SET
			library_id=EXCLUDED.library_id, title=EXCLUDED.title, artist=EXCLUDED.artist,
			album=EXCLUDED.album, album_artist=EXCLUDED.album_artist, year=EXCLUDED.year,
			track_number=EXCLUDED.track_number, disk_number=EXCLUDED.disk_number,
			duration=EXCLUDED.duration, file_size=EXCLUDED.file_size,
			file_hash=EXCLUDED.file_hash, mod_time=EXCLUDED.mod_time,
			mime_type=EXCLUDED.mime_type, bitrate=EXCLUDED.bitrate,
			sample_rate=EXCLUDED.sample_rate, channels=EXCLUDED.channels,
			genre=EXCLUDED.genre,
			album_folder=EXCLUDED.album_folder,
			cover_art_url=CASE
				WHEN $21::bool THEN '/api/v1/music/tracks/' || music_tracks.id || '/art'
				WHEN music_tracks.cover_art_url LIKE '/api/v1/music/album-art/%' THEN music_tracks.cover_art_url
				ELSE '' END,
			updated_at=NOW()
		RETURNING id`,
		t.ID, t.LibraryID, t.Title, t.Artist, t.Album, t.AlbumArtist,
		t.Year, t.TrackNumber, t.DiskNumber, t.Duration, t.FilePath,
		t.FileSize, t.FileHash, t.ModTime, t.MimeType, t.Bitrate, t.SampleRate, t.Channels,
		t.Genre, t.AlbumFolder, hasCover,
	).Scan(&persistedID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	span.SetAttributes(attribute.String("music_track.persisted_id", persistedID))
	return persistedID, nil
}

func (r *MusicTrackRepository) GetByID(ctx context.Context, id string) (*music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("music_track.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	t := &music.Track{}
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE id=$1`, id).Scan(
		&t.ID, &t.LibraryID, &t.Title, &t.Artist, &t.Album, &t.AlbumArtist,
		&t.Year, &t.TrackNumber, &t.DiskNumber, &t.Duration, &t.FilePath,
		&t.FileSize, &t.FileHash, &t.ModTime, &t.MimeType, &t.Bitrate, &t.SampleRate, &t.Channels,
		&t.Genre, &t.AlbumFolder, &t.CoverArtURL, &t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return t, nil
}

func (r *MusicTrackRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("music_track.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM music_tracks WHERE id=$1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// CountByLibrary counts a library's tracks without reading them. Same reason as
// photo: the list query carries a COUNT(*) OVER() window, so using it as a
// counter reads and sorts the whole library.
func (r *MusicTrackRepository) CountByLibrary(ctx context.Context, libraryID shared.LibraryID) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.music.CountByLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var n int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM music_tracks WHERE library_id = $1`, libraryID).Scan(&n)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return n, nil
}

func (r *MusicTrackRepository) ListByLibrary(ctx context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]music.Track, int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListByLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 50
	}
	// Escape LIKE meta-characters in the search term; treat empty as "no filter".
	escaped := likeEscape(search)
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM music_tracks
		WHERE library_id=$1
		  AND ($2 = '' OR (
		        title ILIKE '%' || $2 || '%' ESCAPE '\' OR
		        artist ILIKE '%' || $2 || '%' ESCAPE '\' OR
		        album ILIKE '%' || $2 || '%' ESCAPE '\'
		      ))
		ORDER BY album_artist, album, disk_number, track_number, id
		LIMIT $3 OFFSET $4`, libraryID, escaped, limit, offset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}
	defer rows.Close()
	tracks, total, err := scanTracksWithTotal(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, total, err
}

// ListByLibraryCursor is the keyset form of ListByLibrary: no COUNT(*) OVER()
// window, so Postgres can use its top-N heapsort for ORDER BY ... LIMIT
// instead of materializing the whole matching set before handing back one
// page - the window function forces that materialization even for page 1.
func (r *MusicTrackRepository) ListByLibraryCursor(ctx context.Context, libraryID shared.LibraryID, search string, cursor *music.TrackCursor, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListByLibraryCursor")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 50
	}
	escaped := likeEscape(search)
	query := `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks
		WHERE library_id=$1
		  AND ($2 = '' OR (
		        title ILIKE '%' || $2 || '%' ESCAPE '\' OR
		        artist ILIKE '%' || $2 || '%' ESCAPE '\' OR
		        album ILIKE '%' || $2 || '%' ESCAPE '\'
		      ))`
	args := []any{libraryID, escaped}
	i := 3
	if cursor != nil {
		query += fmt.Sprintf(" AND (album_artist, album, disk_number, track_number, id) > ($%d, $%d, $%d, $%d, $%d)", i, i+1, i+2, i+3, i+4)
		args = append(args, cursor.AlbumArtist, cursor.Album, cursor.DiskNumber, cursor.TrackNumber, cursor.ID)
		i += 5
	}
	query += fmt.Sprintf(" ORDER BY album_artist, album, disk_number, track_number, id LIMIT $%d", i)
	args = append(args, limit)

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	tracks, err := scanTracks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, err
}

func (r *MusicTrackRepository) ListByAlbum(ctx context.Context, album, artist string) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListByAlbum")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE album=$1 AND album_artist=$2
		ORDER BY disk_number, track_number
		LIMIT 1000`, album, artist)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	tracks, err := scanTracks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, err
}

func (r *MusicTrackRepository) ListByLibraryAndAlbum(ctx context.Context, libraryID shared.LibraryID, album string) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListByLibraryAndAlbum")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE library_id=$1 AND album=$2
		ORDER BY disk_number, track_number
		LIMIT 1000`, libraryID, album)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	tracks, err := scanTracks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, err
}

// ListByAlbumFolder returns tracks of a specific album identified by
// its on-disk folder. This is the folder-based replacement for
// ListByLibraryAndAlbum: it is robust to featurings / compilations that
// carry inconsistent album_artist tags across tracks of the same folder.
func (r *MusicTrackRepository) ListByAlbumFolder(ctx context.Context, libraryID shared.LibraryID, albumFolder string) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListByAlbumFolder")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE library_id=$1 AND album_folder=$2
		ORDER BY disk_number, track_number
		LIMIT 1000`, libraryID, albumFolder)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	tracks, err := scanTracks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, err
}

func (r *MusicTrackRepository) ListByArtist(ctx context.Context, artist string, limit, offset int) ([]music.Track, int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListByArtist")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 50
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM music_tracks WHERE album_artist=$1 OR artist=$1
		ORDER BY album, disk_number, track_number, id
		LIMIT $2 OFFSET $3`, artist, limit, offset)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}
	defer rows.Close()
	tracks, total, err := scanTracksWithTotal(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, total, err
}

// ListByArtistCursor is the keyset form of ListByArtist. See ListByLibraryCursor.
func (r *MusicTrackRepository) ListByArtistCursor(ctx context.Context, artist string, cursor *music.ArtistTrackCursor, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListByArtistCursor")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE (album_artist=$1 OR artist=$1)`
	args := []any{artist}
	i := 2
	if cursor != nil {
		query += fmt.Sprintf(" AND (album, disk_number, track_number, id) > ($%d, $%d, $%d, $%d)", i, i+1, i+2, i+3)
		args = append(args, cursor.Album, cursor.DiskNumber, cursor.TrackNumber, cursor.ID)
		i += 4
	}
	query += fmt.Sprintf(" ORDER BY album, disk_number, track_number, id LIMIT $%d", i)
	args = append(args, limit)

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	tracks, err := scanTracks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, err
}

// likeEscape escapes LIKE/ILIKE meta-characters so caller-supplied input is
// matched literally. Uses the default '\' escape character.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *MusicTrackRepository) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.Search")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 20
	}
	// Escape % and _ so user input cannot construct pathological ILIKE patterns.
	like := "%" + likeEscape(query) + "%"
	// An empty libraryID searches across every library (the global search bar's
	// SQL fallback when Meilisearch is down). The caller is responsible for
	// filtering hits down to the libraries the user may access.
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks
		WHERE ($1 = '' OR library_id = $1) AND (title ILIKE $2 ESCAPE '\' OR artist ILIKE $2 ESCAPE '\' OR album ILIKE $2 ESCAPE '\')
		ORDER BY title LIMIT $3`, string(libraryID), like, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	tracks, err := scanTracks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, err
}

// GetAlbums groups tracks by their parent folder (album_folder). Each
// display field on the returned Album is the most frequent value among
// the tracks of that folder - ID3 tags within a folder can disagree
// (a featuring changes album_artist on one track, an Apple-Music-style
// import writes "Orchard Essentials" on the album tag, etc). The
// folder wins as identity; the majority wins for display.
func (r *MusicTrackRepository) GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]music.Album, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.GetAlbums")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT album_folder,
		       COALESCE(mode() WITHIN GROUP (ORDER BY NULLIF(album,'')), '') AS display_album,
		       COALESCE(
		           mode() WITHIN GROUP (ORDER BY NULLIF(album_artist,'')),
		           mode() WITHIN GROUP (ORDER BY NULLIF(artist,'')),
		           ''
		       ) AS display_artist,
		       -- year is NOT NULL DEFAULT 0: one untagged track must not hide the
		       -- album's year.
		       COALESCE(MIN(NULLIF(year,0)), 0) AS year,
		       COUNT(*)  AS track_count,
		       SUM(duration) AS duration,
		       -- Préférence EXPLICITE pour la pochette embarquée dans le fichier
		       -- (/music/tracks/<id>/art) sur celle récupérée en externe
		       -- (/music/album-art/<hash>). Un simple MIN() tranchait par ordre
		       -- alphabétique, et 'a' < 't' : une seule ligne iTunes suffisait à
		       -- masquer les pochettes embarquées correctes de tout le dossier.
		       -- L'artwork du fichier est la source autoritaire.
		       COALESCE(
		           MIN(NULLIF(cover_art_url,'')) FILTER (WHERE cover_art_url LIKE '/api/v1/music/tracks/%'),
		           MIN(NULLIF(cover_art_url,''))
		       ) AS cover_art_url
		FROM music_tracks
		WHERE library_id=$1
		GROUP BY album_folder
		ORDER BY display_album, album_folder
		LIMIT 10000`, libraryID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var albums []music.Album
	for rows.Next() {
		var a music.Album
		var folder string
		var cover sql.NullString
		if err := rows.Scan(&folder, &a.Title, &a.Artist, &a.Year, &a.TrackCount, &a.Duration, &cover); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		a.CoverArtURL = cover.String
		// An all-untagged folder (no album tag on any track) used to render a
		// BLANK album header in every client (prod 2026-08-26, an untagged
		// audio-documentary mp3). The folder is already this album's identity -
		// its name is the best available display, never the empty string.
		if a.Title == "" {
			a.Title = filepath.Base(folder)
		}
		// Album identity is the folder path, not a (title, artist) tuple. Expose
		// it to clients as a short opaque id (not the raw path) so URLs don't
		// leak the filesystem layout and homonym albums don't collide.
		a.ID = music.AlbumID(folder)
		albums = append(albums, a)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return albums, nil
}

func (r *MusicTrackRepository) GetArtists(ctx context.Context, libraryID shared.LibraryID) ([]music.Artist, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.GetArtists")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		-- Album identity is the on-disk folder, not the album tag (GetAlbums groups
		-- on album_folder): two folders sharing one album tag are two albums, so
		-- count distinct folders to match the artist's actual album count.
		-- Same cover preference as the albums: the file's own art over an
		-- external one (a plain MIN picked '/album-art/' over '/tracks/').
		SELECT COALESCE(NULLIF(album_artist,''), artist), COUNT(*), COUNT(DISTINCT album_folder),
		       COALESCE(
		           MIN(NULLIF(cover_art_url,'')) FILTER (WHERE cover_art_url LIKE '/api/v1/music/tracks/%'),
		           MIN(NULLIF(cover_art_url,''))
		       )
		FROM music_tracks WHERE library_id=$1
		GROUP BY 1 ORDER BY 1
		LIMIT 10000`, libraryID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var artists []music.Artist
	for rows.Next() {
		var a music.Artist
		var cover sql.NullString
		if err := rows.Scan(&a.Name, &a.TrackCount, &a.AlbumCount, &cover); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		a.CoverArtURL = cover.String
		a.ID = a.Name
		artists = append(artists, a)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return artists, nil
}

// ListAllByLibrary returns every track in a library, used by the scanner
// to build dedup indexes. No LIMIT for the same reason as PhotoRepo
// (see that method): an arbitrary cap makes libs > cap churn upsert
// every scan tick because the rest look "missing from byPath".
// Not for UI pagination (see ListByAlbum / search endpoints).
func (r *MusicTrackRepository) ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListAllByLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE library_id=$1`, libraryID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	tracks, err := scanTracks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, err
}

// SampleByLibrary returns up to limit RANDOM tracks of the library - a bounded
// candidate pool for the playlist generator so a huge library isn't loaded whole
// into memory just to pick a few dozen tracks. Genre affinity is computed
// separately from the profile's full history, so the sample only needs to be a
// large-enough reservoir to rank.
func (r *MusicTrackRepository) SampleByLibrary(ctx context.Context, libraryID shared.LibraryID, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.SampleByLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)), attribute.Int("limit", limit))
	if limit <= 0 {
		return nil, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, album_folder, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE library_id=$1
		ORDER BY RANDOM() LIMIT $2`, libraryID, limit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	tracks, err := scanTracks(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return tracks, err
}

// ListAlbumsMissingArt returns one AlbumKey per folder that has no
// cover art on any of its tracks. The Album / Artist display strings
// come from the folder's majority tags, so downstream fetchers (which
// still key on human-readable artist+album) get the best-guess values.
// ListAlbumFolders returns the distinct album_folder values in a library. Used
// to resolve a short opaque album id (music.AlbumID) back to its folder without
// a reverse hash: the caller hashes each folder and matches. The set is small
// (one per album) and bounded.
func (r *MusicTrackRepository) ListAlbumFolders(ctx context.Context, libraryID shared.LibraryID) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListAlbumFolders")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT DISTINCT album_folder
		FROM music_tracks
		WHERE library_id=$1 AND album_folder <> ''
		LIMIT 10000`, libraryID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var folder string
		if err := rows.Scan(&folder); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		out = append(out, folder)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}

func (r *MusicTrackRepository) ListAllAlbums(ctx context.Context, libraryID shared.LibraryID) ([]music.AlbumKey, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.ListAllAlbums")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Every album folder we can represent as a fetchable (artist, album). The
	// cover_art_url state is deliberately NOT filtered here: the service gates
	// on whether the cached image exists on disk, so an album whose URL is set
	// but whose file was wiped is re-fetched instead of staying broken.
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT album_folder,
		       COALESCE(mode() WITHIN GROUP (ORDER BY NULLIF(album,'')), '') AS display_album,
		       COALESCE(
		           mode() WITHIN GROUP (ORDER BY NULLIF(album_artist,'')),
		           mode() WITHIN GROUP (ORDER BY NULLIF(artist,'')),
		           ''
		       ) AS display_artist,
		       -- Une seule piste à pochette embarquée suffit à pourvoir l'album :
		       -- le service saute alors le fetch externe au lieu d'écraser une
		       -- pochette qui vient du fichier lui-même.
		       BOOL_OR(cover_art_url LIKE '/api/v1/music/tracks/%') AS has_embedded_art
		FROM music_tracks
		WHERE library_id=$1
		GROUP BY album_folder
		HAVING COALESCE(mode() WITHIN GROUP (ORDER BY NULLIF(album,'')), '') <> ''
		   AND COALESCE(
		           mode() WITHIN GROUP (ORDER BY NULLIF(album_artist,'')),
		           mode() WITHIN GROUP (ORDER BY NULLIF(artist,'')),
		           ''
		       ) <> ''
		LIMIT 5000`, libraryID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var out []music.AlbumKey
	for rows.Next() {
		var k music.AlbumKey
		// BOOL_OR renvoie NULL sur un groupe entièrement NULL : NullBool absorbe.
		var hasEmbedded sql.NullBool
		if err := rows.Scan(&k.AlbumFolder, &k.Album, &k.Artist, &hasEmbedded); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		k.HasEmbeddedArt = hasEmbedded.Valid && hasEmbedded.Bool
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return out, nil
}

// SetTrackCoverArtURL fixe la pochette d'une seule piste, sans passer par
// l'upsert (qui exige les métadonnées complètes du fichier).
func (r *MusicTrackRepository) SetTrackCoverArtURL(ctx context.Context, trackID, url string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.SetTrackCoverArtURL")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		UPDATE music_tracks SET cover_art_url=$2, updated_at=NOW() WHERE id=$1`,
		trackID, url)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// SetAlbumCoverArtURL applies a cover art URL to every track sharing the
// same album_folder. Using the folder as the key keeps the semantics
// aligned with GetAlbums / ListAlbumsMissingArt and avoids mis-stamping
// tracks when tag values disagree within the folder.
//
// Les pistes portant une pochette EMBARQUÉE sont exclues : elles sont la source
// autoritaire. L'UPDATE écrasait tout le dossier, y compris ces pistes - et
// comme un scan ne ré-upsert PAS un fichier inchangé, la pochette embarquée ne
// revenait jamais. C'est ce qui a durablement collé la pochette de Mixed Signals
// sur "Orchard Essentials" en prod (07-25).
func (r *MusicTrackRepository) SetAlbumCoverArtURL(ctx context.Context, libraryID shared.LibraryID, albumFolder, url string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.SetAlbumCoverArtURL")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		UPDATE music_tracks
		SET cover_art_url=$3, updated_at=NOW()
		WHERE library_id=$1 AND album_folder=$2
		  AND cover_art_url NOT LIKE '/api/v1/music/tracks/%'`,
		libraryID, albumFolder, url)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (r *MusicTrackRepository) UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.musictrack.UpdateFilePath")
	defer span.End()
	span.SetAttributes(attribute.String("music_track.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// A moved file changes its parent folder; re-derive album_folder so
	// the album grouping stays coherent without a full re-scan.
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE music_tracks SET file_path = $1, mod_time = $2, album_folder = $4, updated_at = NOW() WHERE id = $3`,
		newPath, modTime, id, filepath.Dir(newPath))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func scanTracksWithTotal(rows *sql.Rows) ([]music.Track, int, error) {
	var tracks []music.Track
	var total int
	for rows.Next() {
		var t music.Track
		if err := rows.Scan(
			&t.ID, &t.LibraryID, &t.Title, &t.Artist, &t.Album, &t.AlbumArtist,
			&t.Year, &t.TrackNumber, &t.DiskNumber, &t.Duration, &t.FilePath,
			&t.FileSize, &t.FileHash, &t.ModTime, &t.MimeType, &t.Bitrate, &t.SampleRate, &t.Channels,
			&t.Genre, &t.AlbumFolder, &t.CoverArtURL, &t.CreatedAt, &t.UpdatedAt,
			&total,
		); err != nil {
			return nil, 0, err
		}
		tracks = append(tracks, t)
	}
	return tracks, total, rows.Err()
}

func scanTracks(rows *sql.Rows) ([]music.Track, error) {
	var tracks []music.Track
	for rows.Next() {
		var t music.Track
		if err := rows.Scan(
			&t.ID, &t.LibraryID, &t.Title, &t.Artist, &t.Album, &t.AlbumArtist,
			&t.Year, &t.TrackNumber, &t.DiskNumber, &t.Duration, &t.FilePath,
			&t.FileSize, &t.FileHash, &t.ModTime, &t.MimeType, &t.Bitrate, &t.SampleRate, &t.Channels,
			&t.Genre, &t.AlbumFolder, &t.CoverArtURL, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

var _ music.TrackRepository = (*MusicTrackRepository)(nil)
