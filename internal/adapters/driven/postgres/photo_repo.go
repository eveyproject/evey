package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type PhotoRepository struct {
	db *sql.DB
}

func NewPhotoRepository(db *sql.DB) *PhotoRepository {
	return &PhotoRepository{db: db}
}

func (r *PhotoRepository) Upsert(ctx context.Context, p photo.Photo) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.Upsert")
	defer span.End()
	span.SetAttributes(attribute.String("photo.id", p.ID))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	// blurhash is nullable in the DB but a plain `string` on the domain
	// (empty = absent). NullIfEmpty preserves the existing NULL when the
	// scanner skipped the encode, instead of pushing an empty string that
	// would be hard to distinguish from "never computed".
	var blurhash sql.NullString
	if p.Blurhash != "" {
		blurhash = sql.NullString{String: p.Blurhash, Valid: true}
	}
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `
		INSERT INTO photos (id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		                    taken_at, latitude, longitude, camera_model, album, blurhash, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
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
		    blurhash     = COALESCE(EXCLUDED.blurhash, photos.blurhash),
		    updated_at   = EXCLUDED.updated_at`,
		p.ID, p.LibraryID, p.Title, p.FilePath, p.FileSize, p.FileHash, p.ModTime, p.MimeType,
		p.Width, p.Height, p.TakenAt, p.Latitude, p.Longitude,
		p.CameraModel, p.Album, blurhash, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (r *PhotoRepository) GetByID(ctx context.Context, id string) (*photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.GetByID")
	defer span.End()
	span.SetAttributes(attribute.String("photo.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	p := &photo.Photo{}
	var blurhash sql.NullString
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, blurhash, created_at, updated_at
		FROM photos WHERE id = $1`, id,
	).Scan(
		&p.ID, &p.LibraryID, &p.Title, &p.FilePath, &p.FileSize, &p.FileHash, &p.ModTime, &p.MimeType,
		&p.Width, &p.Height, &p.TakenAt, &p.Latitude, &p.Longitude,
		&p.CameraModel, &p.Album, &blurhash, &p.CreatedAt, &p.UpdatedAt,
	)
	p.Blurhash = blurhash.String
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return p, nil
}

func (r *PhotoRepository) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.Delete")
	defer span.End()
	span.SetAttributes(attribute.String("photo.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM photos WHERE id = $1`, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		span.RecordError(shared.ErrNotFound)
		span.SetStatus(codes.Error, shared.ErrNotFound.Error())
		return shared.ErrNotFound
	}
	return nil
}

func (r *PhotoRepository) DeleteByIDs(ctx context.Context, ids []string) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.DeleteByIDs")
	defer span.End()
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx, `DELETE FROM photos WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("PhotoRepo.DeleteByIDs: %w", err)
	}
	return nil
}

// CountByLibrary counts a library's photos without reading them. The stats
// endpoint loops over every library, and the previous form (ListByLibrary with
// limit=1, rows discarded) paid a full read + sort per library because of the
// COUNT(*) OVER() window in the list query.
func (r *PhotoRepository) CountByLibrary(ctx context.Context, libraryID shared.LibraryID) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.CountByLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var n int
	err := txFromCtx(ctx, r.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM photos WHERE library_id = $1`, libraryID).Scan(&n)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return n, nil
}

func (r *PhotoRepository) ListByLibrary(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]photo.Photo, int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.ListByLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, blurhash, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM photos WHERE library_id = $1
		ORDER BY taken_at DESC NULLS LAST, created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, libraryID, limit, offset,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}
	defer rows.Close()
	photos, total, err := scanPhotosWithTotal(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return photos, total, err
}

func (r *PhotoRepository) ListByAlbum(ctx context.Context, libraryID shared.LibraryID, album string, limit, offset int) ([]photo.Photo, int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.ListByAlbum")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, blurhash, created_at, updated_at,
		       COUNT(*) OVER() AS total
		FROM photos WHERE library_id = $1 AND album = $2
		ORDER BY taken_at DESC NULLS LAST, created_at DESC, id DESC
		LIMIT $3 OFFSET $4`, libraryID, album, limit, offset,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, 0, err
	}
	defer rows.Close()
	photos, total, err := scanPhotosWithTotal(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return photos, total, err
}

// photoCursorOrderBy is the sort key both the offset (COUNT(*) OVER()) and
// keyset paginated queries share: taken_at DESC NULLS LAST, created_at DESC,
// id DESC. Kept as the RAW columns (not COALESCE(taken_at, ...)) on purpose:
// idx_photos_library_taken_at is built on exactly this expression, and an
// EXPLAIN ANALYZE against the 7199-photo dev library showed COALESCE defeats
// that index (Seq Scan + Sort, ~4-8ms) where the raw form gets an Index Scan
// the planner can also use to satisfy the seek predicate below (~0.1-0.2ms).
const photoCursorOrderBy = `ORDER BY taken_at DESC NULLS LAST, created_at DESC, id DESC`

// photoCursorSeek builds the "give me rows strictly after the cursor" WHERE
// fragment for the keyset queries, and the args it binds starting at $argN.
// It returns "" when cursor is nil (first page).
//
// It cannot use a single row-comparison `(taken_at, created_at, id) < (...)`
// like the non-nullable cursors elsewhere in the codebase (e.g.
// WatchProgressRepo.GetByProfileCursor): taken_at is nullable and NULLS LAST
// puts every NULL row after every non-null one, but SQL row comparison
// returns NULL (excludes the row) the moment either side of a component is
// NULL - so a plain "<" would silently drop the entire NULL-taken_at group
// the first time the cursor still has a non-null TakenAt. The two branches
// below encode the NULLS LAST ordering explicitly instead.
func photoCursorSeek(cursor *photo.PhotoCursor, argN int) (sql string, args []any) {
	if cursor == nil {
		return "", nil
	}
	if cursor.TakenAt != nil {
		// Still inside (or entering) the non-null taken_at group: anything with
		// taken_at IS NULL sorts after the whole group and is always a
		// candidate for the next page; anything with an earlier taken_at, or a
		// tied taken_at broken by created_at/id, is too.
		return fmt.Sprintf(
			` AND (taken_at IS NULL OR taken_at < $%d OR (taken_at = $%d AND created_at < $%d) OR (taken_at = $%d AND created_at = $%d AND id < $%d))`,
			argN, argN, argN+1, argN, argN+1, argN+2,
		), []any{*cursor.TakenAt, cursor.CreatedAt, cursor.ID}
	}
	// Already inside the NULL group: only other NULL rows are candidates.
	return fmt.Sprintf(
		` AND (taken_at IS NULL AND (created_at < $%d OR (created_at = $%d AND id < $%d)))`,
		argN, argN, argN+1,
	), []any{cursor.CreatedAt, cursor.ID}
}

// ListByLibraryCursor is the keyset form of ListByLibrary: no COUNT(*) OVER()
// window, so Postgres can use its top-N heapsort for ORDER BY ... LIMIT
// instead of materializing (and sorting) the whole matching set before
// handing back one page - the window function forces that materialization
// even for page 1.
func (r *PhotoRepository) ListByLibraryCursor(ctx context.Context, libraryID shared.LibraryID, cursor *photo.PhotoCursor, limit int) ([]photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.ListByLibraryCursor")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	query := `SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, blurhash, created_at, updated_at
		FROM photos WHERE library_id = $1`
	args := []any{libraryID}
	seek, seekArgs := photoCursorSeek(cursor, 2)
	query += seek
	args = append(args, seekArgs...)
	query += fmt.Sprintf(" %s LIMIT $%d", photoCursorOrderBy, len(args)+1)
	args = append(args, limit)

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	photos, err := scanPhotos(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return photos, err
}

// ListByAlbumCursor is the keyset form of ListByAlbum. See ListByLibraryCursor.
func (r *PhotoRepository) ListByAlbumCursor(ctx context.Context, libraryID shared.LibraryID, album string, cursor *photo.PhotoCursor, limit int) ([]photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.ListByAlbumCursor")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	query := `SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, blurhash, created_at, updated_at
		FROM photos WHERE library_id = $1 AND album = $2`
	args := []any{libraryID, album}
	seek, seekArgs := photoCursorSeek(cursor, 3)
	query += seek
	args = append(args, seekArgs...)
	query += fmt.Sprintf(" %s LIMIT $%d", photoCursorOrderBy, len(args)+1)
	args = append(args, limit)

	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	photos, err := scanPhotos(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return photos, err
}

func (r *PhotoRepository) GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]photo.Album, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.GetAlbums")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT album, library_id, COUNT(*) as photo_count, MIN(id) as cover_photo_id
		FROM photos WHERE library_id = $1 AND album != ''
		GROUP BY album, library_id ORDER BY album
		LIMIT 10000`, libraryID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var albums []photo.Album
	for rows.Next() {
		var a photo.Album
		if err := rows.Scan(&a.Name, &a.LibraryID, &a.PhotoCount, &a.CoverPhotoID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		albums = append(albums, a)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return albums, nil
}

func (r *PhotoRepository) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.Search")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	pattern := "%" + likeEscape(query) + "%"
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, blurhash, created_at, updated_at
		FROM photos WHERE library_id = $1 AND (title ILIKE $2 ESCAPE '\' OR album ILIKE $2 ESCAPE '\')
		ORDER BY taken_at DESC NULLS LAST
		LIMIT $3`, libraryID, pattern, limit,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	photos, err := scanPhotos(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return photos, err
}

// ListAllByLibrary returns every photo in a library, used by the scanner
// to build dedup indexes (byPath / byHash). No LIMIT: the scanner needs
// the full set to detect moves + in-place updates; an arbitrary cap
// would make libs > cap look like "N new files every tick" and spin
// upsert. Memory is bounded by row count × row size - a lib with 100k
// photos holds ~50-100MB here, acceptable compared to the churn cost.
// Not to be used for UI pagination (see ListByLibrary for that).
func (r *PhotoRepository) ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.ListAllByLibrary")
	defer span.End()
	span.SetAttributes(attribute.String("library.id", string(libraryID)))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, `
		SELECT id, library_id, title, file_path, file_size, file_hash, mod_time, mime_type, width, height,
		       taken_at, latitude, longitude, camera_model, album, blurhash, created_at, updated_at
		FROM photos WHERE library_id = $1`, libraryID,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	photos, err := scanPhotos(rows)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return photos, err
}

func (r *PhotoRepository) UpdateFilePath(ctx context.Context, id, newPath, album string, modTime time.Time) error {
	ctx, span := tracing.StartSpan(ctx, "postgres.photo.UpdateFilePath")
	defer span.End()
	span.SetAttributes(attribute.String("photo.id", id))
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := txFromCtx(ctx, r.db).ExecContext(ctx,
		`UPDATE photos SET file_path = $1, album = $2, mod_time = $3, updated_at = NOW() WHERE id = $4`,
		newPath, album, modTime, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func scanPhotos(rows *sql.Rows) ([]photo.Photo, error) {
	var photos []photo.Photo
	for rows.Next() {
		var p photo.Photo
		var blurhash sql.NullString
		if err := rows.Scan(
			&p.ID, &p.LibraryID, &p.Title, &p.FilePath, &p.FileSize, &p.FileHash, &p.ModTime, &p.MimeType,
			&p.Width, &p.Height, &p.TakenAt, &p.Latitude, &p.Longitude,
			&p.CameraModel, &p.Album, &blurhash, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		p.Blurhash = blurhash.String
		photos = append(photos, p)
	}
	return photos, rows.Err()
}

func scanPhotosWithTotal(rows *sql.Rows) ([]photo.Photo, int, error) {
	var photos []photo.Photo
	var total int
	for rows.Next() {
		var p photo.Photo
		var blurhash sql.NullString
		if err := rows.Scan(
			&p.ID, &p.LibraryID, &p.Title, &p.FilePath, &p.FileSize, &p.FileHash, &p.ModTime, &p.MimeType,
			&p.Width, &p.Height, &p.TakenAt, &p.Latitude, &p.Longitude,
			&p.CameraModel, &p.Album, &blurhash, &p.CreatedAt, &p.UpdatedAt,
			&total,
		); err != nil {
			return nil, 0, err
		}
		p.Blurhash = blurhash.String
		photos = append(photos, p)
	}
	return photos, total, rows.Err()
}

var _ photo.Repository = (*PhotoRepository)(nil)
