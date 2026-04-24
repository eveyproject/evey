package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
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
	rows, err := r.db.QueryContext(ctx,
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
	rows, err := r.db.QueryContext(ctx,
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

// GetCastForItems fetches cast for multiple media items in one query — eliminates N+1 in gallery listing.
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
	rows, err := r.db.QueryContext(ctx,
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

var _ library.MediaItemQueryRepo = (*MediaItemQueryRepoAdapter)(nil)
