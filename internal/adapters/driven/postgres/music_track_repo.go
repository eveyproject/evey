package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
)

type MusicTrackRepository struct {
	db *sql.DB
}

func NewMusicTrackRepository(db *sql.DB) *MusicTrackRepository {
	return &MusicTrackRepository{db: db}
}

func (r *MusicTrackRepository) Upsert(ctx context.Context, t music.Track) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	hasCover := t.CoverArtURL != ""
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO music_tracks (
			id, library_id, title, artist, album, album_artist,
			year, track_number, disk_number, duration, file_path,
			file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
			genre, cover_art_url, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
			CASE WHEN $20::bool THEN '/api/v1/music/tracks/' || $1 || '/art' ELSE '' END,
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
			cover_art_url=CASE
				WHEN $20::bool THEN '/api/v1/music/tracks/' || music_tracks.id || '/art'
				WHEN music_tracks.cover_art_url LIKE '/api/v1/music/album-art/%' THEN music_tracks.cover_art_url
				ELSE '' END,
			updated_at=NOW()`,
		t.ID, t.LibraryID, t.Title, t.Artist, t.Album, t.AlbumArtist,
		t.Year, t.TrackNumber, t.DiskNumber, t.Duration, t.FilePath,
		t.FileSize, t.FileHash, t.ModTime, t.MimeType, t.Bitrate, t.SampleRate, t.Channels,
		t.Genre, hasCover,
	)
	return err
}

func (r *MusicTrackRepository) GetByID(ctx context.Context, id string) (*music.Track, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	t := &music.Track{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE id=$1`, id).Scan(
		&t.ID, &t.LibraryID, &t.Title, &t.Artist, &t.Album, &t.AlbumArtist,
		&t.Year, &t.TrackNumber, &t.DiskNumber, &t.Duration, &t.FilePath,
		&t.FileSize, &t.FileHash, &t.ModTime, &t.MimeType, &t.Bitrate, &t.SampleRate, &t.Channels,
		&t.Genre, &t.CoverArtURL, &t.CreatedAt, &t.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (r *MusicTrackRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM music_tracks WHERE id=$1`, id)
	return err
}

func (r *MusicTrackRepository) ListByLibrary(ctx context.Context, libraryID shared.LibraryID, limit, offset int, search string) ([]music.Track, int, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM music_tracks
		WHERE library_id=$1
		  AND ($2 = '' OR (title ILIKE '%' || $2 || '%' OR artist ILIKE '%' || $2 || '%' OR album ILIKE '%' || $2 || '%'))
		ORDER BY album_artist, album, disk_number, track_number
		LIMIT $3 OFFSET $4`, libraryID, search, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	return scanTracksWithTotal(rows)
}

func (r *MusicTrackRepository) ListByAlbum(ctx context.Context, album, artist string) ([]music.Track, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE album=$1 AND album_artist=$2
		ORDER BY disk_number, track_number
		LIMIT 1000`, album, artist)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (r *MusicTrackRepository) ListByLibraryAndAlbum(ctx context.Context, libraryID shared.LibraryID, album string) ([]music.Track, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE library_id=$1 AND album=$2
		ORDER BY disk_number, track_number
		LIMIT 1000`, libraryID, album)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (r *MusicTrackRepository) ListByArtist(ctx context.Context, artist string, limit, offset int) ([]music.Track, int, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM music_tracks WHERE album_artist=$1 OR artist=$1
		ORDER BY album, disk_number, track_number
		LIMIT $2 OFFSET $3`, artist, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	return scanTracksWithTotal(rows)
}

func (r *MusicTrackRepository) Search(ctx context.Context, query string, limit int) ([]music.Track, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 20
	}
	like := fmt.Sprintf("%%%s%%", query)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at
		FROM music_tracks
		WHERE title ILIKE $1 OR artist ILIKE $1 OR album ILIKE $1
		ORDER BY title LIMIT $2`, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (r *MusicTrackRepository) GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]music.Album, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// Group by album only: tracks with a "feat." in artist (and empty
	// album_artist) must not spawn a phantom album for the same title.
	// Prefer any non-empty album_artist as the display artist.
	rows, err := r.db.QueryContext(ctx, `
		SELECT album,
		       COALESCE(MIN(NULLIF(album_artist,'')), MIN(artist)),
		       MIN(year), COUNT(*), SUM(duration), MIN(NULLIF(cover_art_url,''))
		FROM music_tracks WHERE library_id=$1
		GROUP BY album
		ORDER BY album
		LIMIT 10000`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var albums []music.Album
	for rows.Next() {
		var a music.Album
		var cover sql.NullString
		if err := rows.Scan(&a.Title, &a.Artist, &a.Year, &a.TrackCount, &a.Duration, &cover); err != nil {
			return nil, err
		}
		a.CoverArtURL = cover.String
		a.ID = fmt.Sprintf("%s|%s", a.Title, a.Artist)
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

func (r *MusicTrackRepository) GetArtists(ctx context.Context, libraryID shared.LibraryID) ([]music.Artist, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(album_artist,''), artist), COUNT(*), COUNT(DISTINCT album), MIN(NULLIF(cover_art_url,''))
		FROM music_tracks WHERE library_id=$1
		GROUP BY 1 ORDER BY 1
		LIMIT 10000`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artists []music.Artist
	for rows.Next() {
		var a music.Artist
		var cover sql.NullString
		if err := rows.Scan(&a.Name, &a.TrackCount, &a.AlbumCount, &cover); err != nil {
			return nil, err
		}
		a.CoverArtURL = cover.String
		a.ID = a.Name
		artists = append(artists, a)
	}
	return artists, rows.Err()
}

func (r *MusicTrackRepository) ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]music.Track, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE library_id=$1
		ORDER BY album_artist, album, disk_number, track_number
		LIMIT 10000`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (r *MusicTrackRepository) ListAlbumsMissingArt(ctx context.Context, libraryID shared.LibraryID) ([]music.AlbumKey, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx, `
		SELECT album, COALESCE(NULLIF(album_artist,''), artist) AS artist
		FROM music_tracks
		WHERE library_id=$1 AND album <> '' AND COALESCE(NULLIF(album_artist,''), artist) <> ''
		GROUP BY album, COALESCE(NULLIF(album_artist,''), artist)
		HAVING BOOL_AND(COALESCE(cover_art_url,'') = '')
		LIMIT 5000`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []music.AlbumKey
	for rows.Next() {
		var k music.AlbumKey
		if err := rows.Scan(&k.Album, &k.Artist); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *MusicTrackRepository) SetAlbumCoverArtURL(ctx context.Context, libraryID shared.LibraryID, album, albumArtist, url string) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `
		UPDATE music_tracks
		SET cover_art_url=$4, updated_at=NOW()
		WHERE library_id=$1 AND album=$2 AND COALESCE(NULLIF(album_artist,''), artist)=$3`,
		libraryID, album, albumArtist, url)
	return err
}

func (r *MusicTrackRepository) UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx,
		`UPDATE music_tracks SET file_path = $1, mod_time = $2, updated_at = NOW() WHERE id = $3`,
		newPath, modTime, id)
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
			&t.Genre, &t.CoverArtURL, &t.CreatedAt, &t.UpdatedAt,
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
			&t.Genre, &t.CoverArtURL, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

var _ music.TrackRepository = (*MusicTrackRepository)(nil)
