package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/music"
)

type MusicTrackRepository struct {
	db *sql.DB
}

func NewMusicTrackRepository(db *sql.DB) *MusicTrackRepository {
	return &MusicTrackRepository{db: db}
}

func (r *MusicTrackRepository) Upsert(ctx context.Context, t music.Track) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO music_tracks (
			id, library_id, title, artist, album, album_artist,
			year, track_number, disk_number, duration, file_path,
			file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
			genre, cover_art_url, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,NOW(),NOW())
		ON CONFLICT (file_path) DO UPDATE SET
			library_id=EXCLUDED.library_id, title=EXCLUDED.title, artist=EXCLUDED.artist,
			album=EXCLUDED.album, album_artist=EXCLUDED.album_artist, year=EXCLUDED.year,
			track_number=EXCLUDED.track_number, disk_number=EXCLUDED.disk_number,
			duration=EXCLUDED.duration, file_size=EXCLUDED.file_size,
			file_hash=EXCLUDED.file_hash, mod_time=EXCLUDED.mod_time,
			mime_type=EXCLUDED.mime_type, bitrate=EXCLUDED.bitrate,
			sample_rate=EXCLUDED.sample_rate, channels=EXCLUDED.channels,
			genre=EXCLUDED.genre, cover_art_url=EXCLUDED.cover_art_url,
			updated_at=NOW()`,
		t.ID, t.LibraryID, t.Title, t.Artist, t.Album, t.AlbumArtist,
		t.Year, t.TrackNumber, t.DiskNumber, t.Duration, t.FilePath,
		t.FileSize, t.FileHash, t.ModTime, t.MimeType, t.Bitrate, t.SampleRate, t.Channels,
		t.Genre, t.CoverArtURL,
	)
	return err
}

func (r *MusicTrackRepository) GetByID(ctx context.Context, id string) (*music.Track, error) {
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
	_, err := r.db.ExecContext(ctx, `DELETE FROM music_tracks WHERE id=$1`, id)
	return err
}

func (r *MusicTrackRepository) ListByLibrary(ctx context.Context, libraryID string, limit, offset int, search string) ([]music.Track, int, error) {
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
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE album=$1 AND album_artist=$2
		ORDER BY disk_number, track_number`, album, artist)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (r *MusicTrackRepository) ListByArtist(ctx context.Context, artist string, limit, offset int) ([]music.Track, int, error) {
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

func (r *MusicTrackRepository) GetAlbums(ctx context.Context, libraryID string) ([]music.Album, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT album, COALESCE(NULLIF(album_artist,''), artist),
		       MIN(year), COUNT(*), SUM(duration), MIN(cover_art_url)
		FROM music_tracks WHERE library_id=$1
		GROUP BY album, COALESCE(NULLIF(album_artist,''), artist)
		ORDER BY album
		LIMIT 10000`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var albums []music.Album
	for rows.Next() {
		var a music.Album
		if err := rows.Scan(&a.Title, &a.Artist, &a.Year, &a.TrackCount, &a.Duration, &a.CoverArtURL); err != nil {
			return nil, err
		}
		a.ID = fmt.Sprintf("%s|%s", a.Title, a.Artist)
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

func (r *MusicTrackRepository) GetArtists(ctx context.Context, libraryID string) ([]music.Artist, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(album_artist,''), artist), COUNT(*), COUNT(DISTINCT album)
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
		if err := rows.Scan(&a.Name, &a.TrackCount, &a.AlbumCount); err != nil {
			return nil, err
		}
		a.ID = a.Name
		artists = append(artists, a)
	}
	return artists, rows.Err()
}

func (r *MusicTrackRepository) ListAllByLibrary(ctx context.Context, libraryID string) ([]music.Track, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, artist, album, album_artist,
		       year, track_number, disk_number, duration, file_path,
		       file_size, file_hash, mod_time, mime_type, bitrate, sample_rate, channels,
		       genre, cover_art_url, created_at, updated_at
		FROM music_tracks WHERE library_id=$1
		LIMIT 10000`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTracks(rows)
}

func (r *MusicTrackRepository) UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error {
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
