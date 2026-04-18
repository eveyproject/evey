package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lib/pq"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

const mediaItemColumns = `id, library_id, parent_id, type, title, scan_title, sort_title, original_title, overview, tagline,
	 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating,
	 official_rating, certifications_by_country, premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at`

const mediaItemColumnsPrefixed = `media_items.id, media_items.library_id, media_items.parent_id, media_items.type,
	 media_items.title, media_items.scan_title, media_items.sort_title, media_items.original_title, media_items.overview, media_items.tagline,
	 media_items.file_path, media_items.file_size, media_items.file_fingerprint, media_items.mod_time,
	 media_items.duration_ms, media_items.bitrate, media_items.container_format, media_items.index_number,
	 media_items.community_rating, media_items.official_rating, media_items.certifications_by_country, media_items.premiere_date, media_items.year,
	 media_items.provider_ids, media_items.genres, media_items.studios, media_items.tags, media_items.trailers,
	 media_items.ratings, media_items.added_at, media_items.updated_at`

type MediaItemRepository struct {
	db *sql.DB
}

func NewMediaItemRepository(db *sql.DB) *MediaItemRepository {
	return &MediaItemRepository{db: db}
}

func (r *MediaItemRepository) Create(ctx context.Context, item *library.MediaItem) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	providerJSON, err := json.Marshal(item.ProviderIDs)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create marshal provider_ids: %w", err)
	}
	genres := item.Genres
	if genres == nil {
		genres = []string{}
	}
	genresJSON, err := json.Marshal(genres)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create marshal genres: %w", err)
	}
	studiosJSON, err := json.Marshal(item.Studios)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create marshal studios: %w", err)
	}
	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create marshal tags: %w", err)
	}
	trailers := item.Trailers
	if trailers == nil {
		trailers = []library.Trailer{}
	}
	trailersJSON, err := json.Marshal(trailers)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create marshal trailers: %w", err)
	}
	ratings := item.Ratings
	if ratings == nil {
		ratings = map[string]float64{}
	}
	ratingsJSON, err := json.Marshal(ratings)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create marshal ratings: %w", err)
	}
	certifications := item.CertificationsByCountry
	if certifications == nil {
		certifications = map[string]string{}
	}
	certificationsJSON, err := json.Marshal(certifications)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create marshal certifications: %w", err)
	}

	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}
	var filePath *string
	if item.FilePath != "" {
		filePath = &item.FilePath
	}
	var fingerprint *string
	if item.FileFingerprint != "" {
		fingerprint = &item.FileFingerprint
	}

	scanTitle := item.ScanTitle
	if scanTitle == "" {
		scanTitle = item.Title
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO media_items (id, library_id, parent_id, type, title, scan_title, sort_title, original_title, overview, tagline,
		 file_path, file_size, file_fingerprint, mod_time, duration_ms, bitrate, container_format, index_number, community_rating, official_rating,
		 certifications_by_country, premiere_date, year, provider_ids, genres, studios, tags, trailers, ratings, added_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31)`,
		item.ID, item.LibraryID, parentID, item.Type, item.Title, scanTitle, item.SortTitle, item.OriginalTitle,
		item.Overview, item.Tagline, filePath, item.FileSize, fingerprint, item.ModTime, item.DurationMs, item.Bitrate,
		item.ContainerFormat, item.IndexNumber, item.CommunityRating, item.OfficialRating,
		certificationsJSON, item.PremiereDate, item.Year, providerJSON, genresJSON, studiosJSON, tagsJSON, trailersJSON, ratingsJSON,
		item.AddedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Create: %w", err)
	}
	r.bumpSeriesLatestChildAddedAt(ctx, item)
	return nil
}

// bumpSeriesLatestChildAddedAt keeps media_items.latest_child_added_at in
// sync on the grandparent series when an episode is created or touched.
// Best-effort — failure is logged but not propagated (the column is a
// denormalized sort cache, missing it at most degrades the "Recently
// added" ordering until the next full scan).
func (r *MediaItemRepository) bumpSeriesLatestChildAddedAt(ctx context.Context, item *library.MediaItem) {
	if item == nil || item.Type != library.Episode || item.ParentID == "" {
		return
	}
	when := item.AddedAt
	if item.UpdatedAt.After(when) {
		when = item.UpdatedAt
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE media_items
		SET latest_child_added_at = $1
		WHERE type = 'series'
		  AND id = (SELECT parent_id FROM media_items WHERE id = $2 AND type = 'season')
		  AND (latest_child_added_at IS NULL OR latest_child_added_at < $1)
	`, when, item.ParentID)
	if err != nil {
		slog.Warn("bump series latest_child_added_at", slog.String("episode_id", string(item.ID)), slog.Any("err", err))
	}
}

func (r *MediaItemRepository) GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s FROM media_items WHERE id = $1", mediaItemColumns), string(id)))
}

func (r *MediaItemRepository) GetByFilePath(ctx context.Context, path string) (*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s FROM media_items WHERE file_path = $1", mediaItemColumns), path))
}

func (r *MediaItemRepository) GetByFileFingerprint(ctx context.Context, fingerprint string) (*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s FROM media_items WHERE file_fingerprint = $1", mediaItemColumns), fingerprint))
}

func (r *MediaItemRepository) GetByLibraryTypeTitle(ctx context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.scanMediaItem(r.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT %s FROM media_items WHERE library_id = $1 AND type = $2 AND title = $3 AND ($4 = '' OR parent_id = $4) LIMIT 1", mediaItemColumns), string(libraryID), itemType, title, parentID))
}

func (r *MediaItemRepository) buildWhereClause(f library.ListMediaItemsFilter) (string, []any) {
	var conditions []string
	var args []any
	argIdx := 1
	if f.LibraryID != nil {
		conditions = append(conditions, fmt.Sprintf("library_id = $%d", argIdx))
		args = append(args, *f.LibraryID)
		argIdx++
	}
	if len(f.LibraryIDs) > 0 {
		placeholders := make([]string, len(f.LibraryIDs))
		for i, id := range f.LibraryIDs {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, id)
			argIdx++
		}
		conditions = append(conditions, "library_id IN ("+strings.Join(placeholders, ",")+")")
	}
	if f.Type != nil {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, *f.Type)
		argIdx++
	}
	if f.ParentID != nil {
		conditions = append(conditions, fmt.Sprintf("parent_id = $%d", argIdx))
		args = append(args, *f.ParentID)
		argIdx++
	}
	if f.Year != nil {
		conditions = append(conditions, fmt.Sprintf("year = $%d", argIdx))
		args = append(args, *f.Year)
		argIdx++
	}
	if f.YearFrom != 0 {
		conditions = append(conditions, fmt.Sprintf("year >= $%d", argIdx))
		args = append(args, f.YearFrom)
		argIdx++
	}
	if f.YearTo != 0 {
		conditions = append(conditions, fmt.Sprintf("year <= $%d", argIdx))
		args = append(args, f.YearTo)
		argIdx++
	}
	if f.MinRating != 0 {
		conditions = append(conditions, fmt.Sprintf("community_rating >= $%d", argIdx))
		args = append(args, f.MinRating)
		argIdx++
	}
	for _, g := range f.Genres {
		conditions = append(conditions, fmt.Sprintf("genres @> $%d::jsonb", argIdx))
		gJSON, err := json.Marshal([]string{g})
		if err != nil {
			slog.Warn("marshal genre filter", slog.Any("err", err))
			continue
		}
		args = append(args, string(gJSON))
		argIdx++
	}
	if len(f.AllowedRatings) > 0 {
		placeholders := make([]string, len(f.AllowedRatings))
		for i, r := range f.AllowedRatings {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, r)
			argIdx++
		}
		conditions = append(conditions, "(official_rating = '' OR official_rating IS NULL OR official_rating = ANY(ARRAY["+strings.Join(placeholders, ",")+"]))")
	}
	if f.PersonID != nil {
		conditions = append(conditions, fmt.Sprintf("media_items.id IN (SELECT media_item_id FROM media_people WHERE person_id = $%d)", argIdx))
		args = append(args, *f.PersonID)
		argIdx++
	}
	if f.AfterID != "" {
		conditions = append(conditions, fmt.Sprintf("media_items.id > $%d", argIdx))
		args = append(args, f.AfterID)
		argIdx++ //nolint:ineffassign // kept so new conditions below stay placeholder-correct
	}
	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	return where, args
}

func (r *MediaItemRepository) Count(ctx context.Context, f library.ListMediaItemsFilter) (int, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args := r.buildWhereClause(f)
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items"+where, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("MediaItemRepo.Count: %w", err)
	}
	return count, nil
}

func (r *MediaItemRepository) List(ctx context.Context, f library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args := r.buildWhereClause(f)
	query := fmt.Sprintf("SELECT %s FROM media_items", mediaItemColumns) + where

	sortField := "sort_title"
	if f.SortField != "" {
		if col, ok := SortColumnMap[f.SortField]; ok {
			sortField = col
		}
	}
	sortOrder := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortOrder = "DESC"
	}
	// Keyset pagination forces an ID-based order so the WHERE id > $N
	// cursor is monotonic. Incompatible with added_at / sort_title orders.
	if f.AfterID != "" {
		query += " ORDER BY media_items.id ASC"
	} else if sortField == "added_at" {
		// Use the denormalized latest_child_added_at column on series rows
		// so "Recently added" sorts by latest episode without a LATERAL
		// join (cf backlog #4, maintained by bumpSeriesLatestChildAddedAt).
		query += fmt.Sprintf(` ORDER BY CASE WHEN media_items.type = 'series'
		 THEN COALESCE(media_items.latest_child_added_at, media_items.added_at)
		 ELSE media_items.added_at END %s`, sortOrder)
	} else {
		query += fmt.Sprintf(" ORDER BY %s %s", sortField, sortOrder)
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.List: %w", err)
	}
	defer rows.Close()

	var result []*library.MediaItem
	for rows.Next() {
		item, err := r.scanMediaItemFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.List scan: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.List rows: %w", err)
	}
	return result, nil
}

func (r *MediaItemRepository) Update(ctx context.Context, item *library.MediaItem) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	providerJSON, err := json.Marshal(item.ProviderIDs)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Update marshal provider_ids: %w", err)
	}
	genres := item.Genres
	if genres == nil {
		genres = []string{}
	}
	genresJSON, err := json.Marshal(genres)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Update marshal genres: %w", err)
	}
	studiosJSON, err := json.Marshal(item.Studios)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Update marshal studios: %w", err)
	}
	tagsJSON, err := json.Marshal(item.Tags)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Update marshal tags: %w", err)
	}
	trailers := item.Trailers
	if trailers == nil {
		trailers = []library.Trailer{}
	}
	trailersJSON, err := json.Marshal(trailers)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Update marshal trailers: %w", err)
	}
	ratings := item.Ratings
	if ratings == nil {
		ratings = map[string]float64{}
	}
	ratingsJSON, err := json.Marshal(ratings)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Update marshal ratings: %w", err)
	}
	certifications := item.CertificationsByCountry
	if certifications == nil {
		certifications = map[string]string{}
	}
	certificationsJSON, err := json.Marshal(certifications)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Update marshal certifications: %w", err)
	}

	var parentID *string
	if item.ParentID != "" {
		parentID = &item.ParentID
	}
	var filePath *string
	if item.FilePath != "" {
		filePath = &item.FilePath
	}
	var fingerprint *string
	if item.FileFingerprint != "" {
		fingerprint = &item.FileFingerprint
	}

	res, err := r.db.ExecContext(ctx,
		`UPDATE media_items SET library_id=$1, parent_id=$2, type=$3, title=$4, sort_title=$5, original_title=$6,
		 overview=$7, tagline=$8, file_path=$9, file_size=$10, file_fingerprint=$11, mod_time=$12, duration_ms=$13, bitrate=$14, container_format=$15,
		 index_number=$16, community_rating=$17, official_rating=$18, certifications_by_country=$19, premiere_date=$20, year=$21,
		 provider_ids=$22, genres=$23, studios=$24, tags=$25, trailers=$26, ratings=$27, updated_at=$28 WHERE id=$29`,
		item.LibraryID, parentID, item.Type, item.Title, item.SortTitle, item.OriginalTitle,
		item.Overview, item.Tagline, filePath, item.FileSize, fingerprint, item.ModTime, item.DurationMs, item.Bitrate,
		item.ContainerFormat, item.IndexNumber, item.CommunityRating, item.OfficialRating,
		certificationsJSON, item.PremiereDate, item.Year, providerJSON, genresJSON, studiosJSON, tagsJSON, trailersJSON, ratingsJSON,
		item.UpdatedAt, item.ID,
	)
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Update: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	r.bumpSeriesLatestChildAddedAt(ctx, item)
	return nil
}

func (r *MediaItemRepository) Delete(ctx context.Context, id shared.MediaItemID) error {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	res, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = $1`, string(id))
	if err != nil {
		return fmt.Errorf("MediaItemRepo.Delete: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (r *MediaItemRepository) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_items WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("MediaItemRepo.DeleteByIDs: %w", err)
	}
	return nil
}

func (r *MediaItemRepository) ListFilePaths(ctx context.Context, libraryID shared.LibraryID) (map[string]string, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, file_path FROM media_items WHERE library_id = $1 AND file_path IS NOT NULL AND file_path <> ''`,
		libraryID,
	)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.ListFilePaths: %w", err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, fmt.Errorf("MediaItemRepo.ListFilePaths scan: %w", err)
		}
		result[id] = path
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.ListFilePaths rows: %w", err)
	}
	return result, nil
}

const listByLibraryIDMaxRows = 10000

func (r *MediaItemRepository) ListByLibraryID(ctx context.Context, libraryID shared.LibraryID) ([]*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf("SELECT %s FROM media_items WHERE library_id = $1 ORDER BY added_at DESC LIMIT $2", mediaItemColumns), libraryID, listByLibraryIDMaxRows)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.ListByLibraryID: %w", err)
	}
	defer rows.Close()
	var result []*library.MediaItem
	for rows.Next() {
		item, err := r.scanMediaItemFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.ListByLibraryID scan: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.ListByLibraryID rows: %w", err)
	}
	return result, nil
}

func (r *MediaItemRepository) ListScanSummaries(ctx context.Context, libraryID shared.LibraryID) ([]library.MediaItemScanRow, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	const batchSize = 5000
	var result []library.MediaItemScanRow
	for offset := 0; ; offset += batchSize {
		batch, err := r.listScanSummariesBatch(ctx, libraryID, batchSize, offset)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.ListScanSummaries: %w", err)
		}
		result = append(result, batch...)
		if len(batch) < batchSize {
			break
		}
	}
	return result, nil
}

func (r *MediaItemRepository) listScanSummariesBatch(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]library.MediaItemScanRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, file_path, file_fingerprint, file_size, mod_time
		 FROM media_items WHERE library_id = $1 ORDER BY id LIMIT $2 OFFSET $3`,
		libraryID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.listScanSummariesBatch: %w", err)
	}
	defer rows.Close()
	var result []library.MediaItemScanRow
	for rows.Next() {
		var row library.MediaItemScanRow
		var filePath, fingerprint *string
		if err := rows.Scan(&row.ID, &filePath, &fingerprint, &row.FileSize, &row.ModTime); err != nil {
			return nil, fmt.Errorf("MediaItemRepo.listScanSummariesBatch scan: %w", err)
		}
		if filePath != nil {
			row.FilePath = *filePath
		}
		if fingerprint != nil {
			row.FileFingerprint = *fingerprint
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.listScanSummariesBatch rows: %w", err)
	}
	return result, nil
}

// SeriesAggregate walks the parent_id tree rooted at seriesID via a recursive
// CTE and sums the runtime of every episode it finds. season_count counts the
// distinct season parents actually contributing episodes (so missing seasons
// that exist only as empty folders don't inflate the number).
func (r *MediaItemRepository) SeriesAggregate(ctx context.Context, seriesID shared.MediaItemID) (library.SeriesAggregate, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	var agg library.SeriesAggregate
	err := r.db.QueryRowContext(ctx, `
		WITH RECURSIVE descendants AS (
		  SELECT id, parent_id, type, duration_ms
		  FROM media_items
		  WHERE id = $1
		  UNION ALL
		  SELECT m.id, m.parent_id, m.type, m.duration_ms
		  FROM media_items m
		  INNER JOIN descendants d ON m.parent_id = d.id
		)
		SELECT
		  COALESCE(SUM(duration_ms) FILTER (WHERE type = 'episode' AND duration_ms > 0), 0) AS total_ms,
		  COUNT(*) FILTER (WHERE type = 'episode')                                         AS episode_count,
		  COUNT(DISTINCT parent_id) FILTER (WHERE type = 'episode')                        AS season_count
		FROM descendants`, string(seriesID)).Scan(&agg.TotalDurationMs, &agg.EpisodeCount, &agg.SeasonCount)
	if err != nil {
		return library.SeriesAggregate{}, fmt.Errorf("MediaItemRepo.SeriesAggregate: %w", err)
	}
	return agg, nil
}

func (r *MediaItemRepository) ListContainerInfo(ctx context.Context, libraryID shared.LibraryID) ([]library.MediaItemContainerRow, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	const batchSize = 5000
	var result []library.MediaItemContainerRow
	for offset := 0; ; offset += batchSize {
		batch, err := r.listContainerInfoBatch(ctx, libraryID, batchSize, offset)
		if err != nil {
			return nil, fmt.Errorf("MediaItemRepo.ListContainerInfo: %w", err)
		}
		result = append(result, batch...)
		if len(batch) < batchSize {
			break
		}
	}
	return result, nil
}

func (r *MediaItemRepository) listContainerInfoBatch(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]library.MediaItemContainerRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, parent_id, type FROM media_items WHERE library_id = $1 ORDER BY id LIMIT $2 OFFSET $3`,
		libraryID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.listContainerInfoBatch: %w", err)
	}
	defer rows.Close()
	var result []library.MediaItemContainerRow
	for rows.Next() {
		var row library.MediaItemContainerRow
		var parentID *string
		if err := rows.Scan(&row.ID, &parentID, &row.Type); err != nil {
			return nil, fmt.Errorf("MediaItemRepo.listContainerInfoBatch scan: %w", err)
		}
		if parentID != nil {
			row.ParentID = *parentID
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.listContainerInfoBatch rows: %w", err)
	}
	return result, nil
}

// rowScanner abstracts sql.Row.Scan and sql.Rows.Scan so scanMediaItem
// and the iterator case share the same column-mapping logic.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanMediaItemFields fills item from a scanner. Returns sql.ErrNoRows
// verbatim when the source is sql.Row and no row was returned — callers
// typically translate to shared.ErrNotFound.
func scanMediaItemFields(src rowScanner, item *library.MediaItem) error {
	var parentID, filePath, fingerprint, scanTitle *string
	var providerJSON, certificationsJSON, genresJSON, studiosJSON, tagsJSON, trailersJSON, ratingsJSON []byte

	if err := src.Scan(
		&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &scanTitle, &item.SortTitle, &item.OriginalTitle,
		&item.Overview, &item.Tagline, &filePath, &item.FileSize, &fingerprint, &item.ModTime, &item.DurationMs, &item.Bitrate,
		&item.ContainerFormat, &item.IndexNumber, &item.CommunityRating, &item.OfficialRating, &certificationsJSON,
		&item.PremiereDate, &item.Year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON, &trailersJSON, &ratingsJSON,
		&item.AddedAt, &item.UpdatedAt,
	); err != nil {
		return err
	}
	if parentID != nil {
		item.ParentID = *parentID
	}
	if filePath != nil {
		item.FilePath = *filePath
	}
	if fingerprint != nil {
		item.FileFingerprint = *fingerprint
	}
	if scanTitle != nil {
		item.ScanTitle = *scanTitle
	}
	// Required fields: corrupt JSON is a DB integrity error, not a soft
	// warning. provider_ids is the join key to TMDB/IMDB; genres drives
	// filtering. Returning an error here stops the request instead of
	// silently serving an item with empty slices.
	if err := json.Unmarshal(providerJSON, &item.ProviderIDs); err != nil {
		return fmt.Errorf("unmarshal provider_ids: %w", err)
	}
	if err := json.Unmarshal(genresJSON, &item.Genres); err != nil {
		return fmt.Errorf("unmarshal genres: %w", err)
	}
	// Optional fields: warn-only so one bad row doesn't break a whole list.
	if len(certificationsJSON) > 0 {
		if err := json.Unmarshal(certificationsJSON, &item.CertificationsByCountry); err != nil {
			slog.Warn("json unmarshal", slog.String("field", "certifications_by_country"), slog.Any("err", err))
		}
	}
	if err := json.Unmarshal(studiosJSON, &item.Studios); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "studios"), slog.Any("err", err))
	}
	if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "tags"), slog.Any("err", err))
	}
	if err := json.Unmarshal(trailersJSON, &item.Trailers); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "trailers"), slog.Any("err", err))
	}
	if err := json.Unmarshal(ratingsJSON, &item.Ratings); err != nil {
		slog.Warn("json unmarshal", slog.String("field", "ratings"), slog.Any("err", err))
	}
	return nil
}

func (r *MediaItemRepository) scanMediaItem(row *sql.Row) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	err := scanMediaItemFields(row, item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("MediaItemRepo.scanMediaItem: %w", err)
	}
	return item, nil
}

func (r *MediaItemRepository) scanMediaItemFromRows(rows *sql.Rows) (*library.MediaItem, error) {
	item := &library.MediaItem{}
	if err := scanMediaItemFields(rows, item); err != nil {
		return nil, fmt.Errorf("MediaItemRepo.scanMediaItemFromRows: %w", err)
	}
	return item, nil
}

var _ library.MediaItemRepository = (*MediaItemRepository)(nil)
