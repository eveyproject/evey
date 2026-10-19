package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// MediaItemQueryRepoAdapter wraps the existing MediaItemRepository and adds
// optimized batch queries for read paths.
type MediaItemQueryRepoAdapter struct {
	items *MediaItemRepository
	db    *sql.DB
}

func NewMediaItemQueryRepoAdapter(db *sql.DB) *MediaItemQueryRepoAdapter {
	return &MediaItemQueryRepoAdapter{
		items: NewMediaItemRepository(db),
		db:    db,
	}
}

func (r *MediaItemQueryRepoAdapter) GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.GetByID")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	item, err := r.items.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return item, nil
}

func (r *MediaItemQueryRepoAdapter) GetByFilePath(ctx context.Context, path string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.GetByFilePath")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	item, err := r.items.GetByFilePath(ctx, path)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return item, nil
}

func (r *MediaItemQueryRepoAdapter) GetByLibraryTypeTitle(ctx context.Context, libraryID shared.LibraryID, itemType, title, parentID string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.GetByLibraryTypeTitle")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	item, err := r.items.GetByLibraryTypeTitle(ctx, libraryID, itemType, title, parentID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return item, nil
}

func (r *MediaItemQueryRepoAdapter) GetByIDs(ctx context.Context, ids []shared.MediaItemID) ([]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.GetByIDs")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(ids) == 0 {
		return nil, nil
	}
	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = string(id)
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		fmt.Sprintf(`SELECT %s %s WHERE mi.id = ANY($1) ORDER BY mi.sort_title`,
			mediaItemColumns, mediaItemFromJoins),
		pq.Array(strIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var result []*library.MediaItem
	for rows.Next() {
		item, err := r.items.scanMediaItemFromRows(rows)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

func (r *MediaItemQueryRepoAdapter) List(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.List")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	items, err := r.items.List(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return items, nil
}

func (r *MediaItemQueryRepoAdapter) Count(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.Count")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	count, err := r.items.Count(ctx, filter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return count, nil
}

// GetCastByMediaItem returns cast members with person info in a single JOIN query.
func (r *MediaItemQueryRepoAdapter) GetCastByMediaItem(ctx context.Context, mediaItemID shared.MediaItemID) ([]library.CastMember, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.GetCastByMediaItem")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mp.person_id, p.name, p.thumb_path, mp.role_type, mp.character_name, mp.sort_order
		 FROM media_people mp
		 JOIN people p ON p.id = mp.person_id
		 WHERE mp.media_item_id = $1
		 ORDER BY mp.sort_order`, string(mediaItemID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	var result []library.CastMember
	for rows.Next() {
		var cm library.CastMember
		if err := rows.Scan(&cm.PersonID, &cm.PersonName, &cm.ThumbPath, &cm.RoleType, &cm.CharacterName, &cm.SortOrder); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result = append(result, cm)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// GetCastForItems fetches cast for multiple media items in one query - eliminates N+1 in gallery listing.
func (r *MediaItemQueryRepoAdapter) GetCastForItems(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]library.CastMember, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.GetCastForItems")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	if len(mediaItemIDs) == 0 {
		return map[shared.MediaItemID][]library.CastMember{}, nil
	}
	strIDs := make([]string, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		strIDs[i] = string(id)
	}
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mp.media_item_id, mp.person_id, p.name, p.thumb_path, mp.role_type, mp.character_name, mp.sort_order
		 FROM media_people mp
		 JOIN people p ON p.id = mp.person_id
		 WHERE mp.media_item_id = ANY($1)
		 ORDER BY mp.media_item_id, mp.sort_order`, pq.Array(strIDs))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()

	result := make(map[shared.MediaItemID][]library.CastMember)
	for rows.Next() {
		var mediaItemID string
		var cm library.CastMember
		if err := rows.Scan(&mediaItemID, &cm.PersonID, &cm.PersonName, &cm.ThumbPath, &cm.RoleType, &cm.CharacterName, &cm.SortOrder); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		result[shared.MediaItemID(mediaItemID)] = append(result[shared.MediaItemID(mediaItemID)], cm)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return result, nil
}

// ListGenres returns the distinct, sorted genres across every item in a
// library. genres lives as a JSONB array on media_catalog, so we unnest it
// with jsonb_array_elements_text and de-dup in SQL - the full set, unlike the
// client which only ever saw genres from the first loaded page.
func (r *MediaItemQueryRepoAdapter) ListGenres(ctx context.Context, libraryID shared.LibraryID) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.ListGenres")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT DISTINCT g
		   FROM media_items mi
		   JOIN media_catalog mc ON mc.media_item_id = mi.id
		   CROSS JOIN LATERAL jsonb_array_elements_text(mc.genres) AS g
		  WHERE mi.library_id = $1 AND g <> ''
		  ORDER BY g`, string(libraryID))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	var genres []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		genres = append(genres, g)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return genres, nil
}

// ListStreamFacets returns the distinct stream values of the items filter
// selects, in one pass over their streams. The item side is List's own WHERE
// (buildWhereClause), so the library, type and parental pre-filter are the
// grid's, never a second copy of them; the parental rule included, so a kids
// profile's facets come from the titles it may see. The join walks
// idx_media_items_library_id then idx_media_streams_media_item_id; no other
// index helps a DISTINCT over every stream of a library.
func (r *MediaItemQueryRepoAdapter) ListStreamFacets(ctx context.Context, filter library.ListMediaItemsFilter) (library.StreamFacets, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.ListStreamFacets")
	defer span.End()
	var facets library.StreamFacets
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args := r.items.buildWhereClause(filter)
	query := `WITH s AS (
		SELECT ms.stream_type, ms.codec, ms.language, COALESCE(ms.width, 0) AS width, COALESCE(ms.height, 0) AS height
		` + mediaItemFromJoins + `
		JOIN media_streams ms ON ms.media_item_id = mi.id` + where + `
	)
	SELECT 'codec', codec, 0, 0 FROM s WHERE stream_type = 'video' AND codec <> '' GROUP BY codec
	UNION ALL
	SELECT 'size', '', width, height FROM s WHERE stream_type = 'video' AND height > 0 GROUP BY width, height
	UNION ALL
	SELECT stream_type, language, 0, 0 FROM s
	 WHERE stream_type IN ('audio', 'subtitle') AND language NOT IN ('', 'und')
	 GROUP BY stream_type, language
	ORDER BY 1, 2, 3, 4`
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return facets, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, value string
		var width, height int
		if err := rows.Scan(&kind, &value, &width, &height); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return library.StreamFacets{}, err
		}
		switch kind {
		case "codec":
			facets.VideoCodecs = append(facets.VideoCodecs, value)
		case "size":
			facets.VideoSizes = append(facets.VideoSizes, library.VideoSize{Width: width, Height: height})
		case string(library.StreamTypeAudio):
			facets.AudioLanguages = append(facets.AudioLanguages, value)
		case string(library.StreamTypeSubtitle):
			facets.SubtitleLanguages = append(facets.SubtitleLanguages, value)
		}
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return library.StreamFacets{}, err
	}
	return facets, nil
}

// CountPerLibrary answers Count for several libraries at once: the same FROM
// and WHERE as Count, grouped by library, so a library list carries its
// counters without one round trip per card.
func (r *MediaItemQueryRepoAdapter) CountPerLibrary(ctx context.Context, filter library.ListMediaItemsFilter) (map[shared.LibraryID]int, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.CountPerLibrary")
	defer span.End()
	counts := make(map[shared.LibraryID]int, len(filter.LibraryIDs))
	if len(filter.LibraryIDs) == 0 {
		return counts, nil
	}
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	where, args := r.items.buildWhereClause(filter)
	rows, err := txFromCtx(ctx, r.db).QueryContext(ctx,
		`SELECT mi.library_id, COUNT(*) `+mediaItemFromJoins+where+` GROUP BY mi.library_id`, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var libraryID string
		var n int
		if err := rows.Scan(&libraryID, &n); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		counts[shared.LibraryID(libraryID)] = n
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return counts, nil
}

func (r *MediaItemQueryRepoAdapter) SeriesAggregate(ctx context.Context, seriesID shared.MediaItemID) (library.SeriesAggregate, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.libraryquery.SeriesAggregate")
	defer span.End()
	agg, err := r.items.SeriesAggregate(ctx, seriesID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return library.SeriesAggregate{}, err
	}
	return agg, nil
}

// NextEpisodeOf returns the next episode in the same series as the given
// episode. Walks: same season + index_number > current, else first episode
// of the next season. Nil + no error when the current episode is the
// series finale.
//
// Used by the predictive warm-up path - pre-spawn the next episode's
// transcode at ~85 % progress so the autoplay handoff is instant.
func (r *MediaItemQueryRepoAdapter) NextEpisodeOf(ctx context.Context, episodeID shared.MediaItemID) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "postgres.media.NextEpisodeOf")
	defer span.End()
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	// CTE: resolve current episode → season → series, then pick the
	// next episode (same season first, else first ep of the next season).
	const q = `
		WITH cur AS (
			SELECT mi.id, mi.parent_id AS season_id, mi.index_number AS episode_idx,
			       sea.parent_id AS series_id, sea.index_number AS season_idx
			FROM media_items mi
			INNER JOIN media_items sea ON sea.id = mi.parent_id
			WHERE mi.id = $1 AND mi.type = 'episode' AND mi.index_number IS NOT NULL
		),
		same_season AS (
			SELECT mi.id, 0 AS rank, mi.index_number AS idx
			FROM media_items mi, cur
			WHERE mi.parent_id = cur.season_id
			  AND mi.type = 'episode'
			  AND mi.index_number IS NOT NULL
			  AND mi.index_number > cur.episode_idx
			ORDER BY mi.index_number ASC
			LIMIT 1
		),
		next_season AS (
			SELECT mi.id, 1 AS rank, mi.index_number AS idx
			FROM media_items sea
			INNER JOIN media_items mi ON mi.parent_id = sea.id AND mi.type = 'episode'
			INNER JOIN cur ON sea.parent_id = cur.series_id
			WHERE sea.type = 'season'
			  AND sea.index_number IS NOT NULL
			  AND sea.index_number > cur.season_idx
			  AND mi.index_number IS NOT NULL
			ORDER BY sea.index_number ASC, mi.index_number ASC
			LIMIT 1
		)
		SELECT id FROM (SELECT id, rank FROM same_season UNION ALL SELECT id, rank FROM next_season) c
		ORDER BY rank ASC LIMIT 1`
	var nextID string
	if err := txFromCtx(ctx, r.db).QueryRowContext(ctx, q, string(episodeID)).Scan(&nextID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	return r.GetByID(ctx, shared.MediaItemID(nextID))
}

var _ library.MediaItemQueryRepo = (*MediaItemQueryRepoAdapter)(nil)
