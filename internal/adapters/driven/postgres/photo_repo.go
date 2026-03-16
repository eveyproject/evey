package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/gravity-zero/evey/internal/domain/photo"
)

type PhotoRepository struct {
	db *sql.DB
}

func NewPhotoRepository(db *sql.DB) *PhotoRepository {
	return &PhotoRepository{db: db}
}

func (r *PhotoRepository) Upsert(ctx context.Context, p photo.Photo) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO photos (id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		                    taken_at, latitude, longitude, camera_model, album, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (file_path) DO UPDATE SET
		    library_id   = EXCLUDED.library_id,
		    title        = EXCLUDED.title,
		    file_size    = EXCLUDED.file_size,
		    file_hash    = EXCLUDED.file_hash,
		    mod_time     = EXCLUDED.mod_time,
		    mime_type    = EXCLUDED.mime_type,
		    width        = EXCLUDED.width,
		    height       = EXCLUDED.height,
		    taken_at     = EXCLUDED.taken_at,
		    latitude     = EXCLUDED.latitude,
		    longitude    = EXCLUDED.longitude,
		    camera_model = EXCLUDED.camera_model,
		    album        = EXCLUDED.album,
		    updated_at   = EXCLUDED.updated_at`,
		p.ID, p.LibraryID, p.Title, p.FilePath, p.FileSize, p.FileHash, p.ModTime, p.MimeType,
		p.Width, p.Height, p.TakenAt, p.Latitude, p.Longitude,
		p.CameraModel, p.Album, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (r *PhotoRepository) GetByID(ctx context.Context, id string) (*photo.Photo, error) {
	p := &photo.Photo{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, created_at, updated_at
		FROM photos WHERE id = $1`, id,
	).Scan(
		&p.ID, &p.LibraryID, &p.Title, &p.FilePath, &p.FileSize, &p.FileHash, &p.ModTime, &p.MimeType,
		&p.Width, &p.Height, &p.TakenAt, &p.Latitude, &p.Longitude,
		&p.CameraModel, &p.Album, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *PhotoRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM photos WHERE id = $1`, id)
	return err
}

func (r *PhotoRepository) ListByLibrary(ctx context.Context, libraryID string, limit, offset int) ([]photo.Photo, int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM photos WHERE library_id = $1
		ORDER BY taken_at DESC NULLS LAST, created_at DESC
		LIMIT $2 OFFSET $3`, libraryID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	return scanPhotosWithTotal(rows)
}

func (r *PhotoRepository) ListByAlbum(ctx context.Context, libraryID, album string, limit, offset int) ([]photo.Photo, int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM photos WHERE library_id = $1 AND album = $2
		ORDER BY taken_at DESC NULLS LAST, created_at DESC
		LIMIT $3 OFFSET $4`, libraryID, album, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	return scanPhotosWithTotal(rows)
}

func (r *PhotoRepository) GetAlbums(ctx context.Context, libraryID string) ([]photo.Album, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT album, library_id, COUNT(*) as photo_count, MIN(id) as cover_photo_id
		FROM photos WHERE library_id = $1 AND album != ''
		GROUP BY album, library_id ORDER BY album`, libraryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var albums []photo.Album
	for rows.Next() {
		var a photo.Album
		if err := rows.Scan(&a.Name, &a.LibraryID, &a.PhotoCount, &a.CoverPhotoID); err != nil {
			return nil, err
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

func (r *PhotoRepository) Search(ctx context.Context, libraryID, query string, limit int) ([]photo.Photo, error) {
	pattern := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, created_at, updated_at
		FROM photos WHERE library_id = $1 AND (title ILIKE $2 OR album ILIKE $2)
		ORDER BY taken_at DESC NULLS LAST
		LIMIT $3`, libraryID, pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPhotos(rows)
}

func (r *PhotoRepository) ListAllByLibrary(ctx context.Context, libraryID string) ([]photo.Photo, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, created_at, updated_at
		FROM photos WHERE library_id = $1`, libraryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPhotos(rows)
}

func (r *PhotoRepository) UpdateFilePath(ctx context.Context, id, newPath string, modTime time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE photos SET file_path = $1, mod_time = $2, updated_at = NOW() WHERE id = $3`,
		newPath, modTime, id)
	return err
}

func scanPhotos(rows *sql.Rows) ([]photo.Photo, error) {
	var photos []photo.Photo
	for rows.Next() {
		var p photo.Photo
		if err := rows.Scan(
			&p.ID, &p.LibraryID, &p.Title, &p.FilePath, &p.FileSize, &p.FileHash, &p.ModTime, &p.MimeType,
			&p.Width, &p.Height, &p.TakenAt, &p.Latitude, &p.Longitude,
			&p.CameraModel, &p.Album, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

func scanPhotosWithTotal(rows *sql.Rows) ([]photo.Photo, int, error) {
	var photos []photo.Photo
	var total int
	for rows.Next() {
		var p photo.Photo
		if err := rows.Scan(
			&p.ID, &p.LibraryID, &p.Title, &p.FilePath, &p.FileSize, &p.FileHash, &p.ModTime, &p.MimeType,
			&p.Width, &p.Height, &p.TakenAt, &p.Latitude, &p.Longitude,
			&p.CameraModel, &p.Album, &p.CreatedAt, &p.UpdatedAt,
			&total,
		); err != nil {
			return nil, 0, err
		}
		photos = append(photos, p)
	}
	return photos, total, rows.Err()
}
