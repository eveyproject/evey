package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
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
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.items.GetByID(ctx, id)
}

func (r *MediaItemQueryRepoAdapter) GetByIDs(ctx context.Context, ids []shared.MediaItemID) ([]*library.MediaItem, error) {
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
		return nil, err
	}
	defer rows.Close()
	var result []*library.MediaItem
	for rows.Next() {
		item, err := r.items.scanMediaItemFromRows(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *MediaItemQueryRepoAdapter) List(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.items.List(ctx, filter)
}

func (r *MediaItemQueryRepoAdapter) Count(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	return r.items.Count(ctx, filter)
}

// GetCastByMediaItem returns cast members with person info in a single JOIN query.
func (r *MediaItemQueryRepoAdapter) GetCastByMediaItem(ctx context.Context, mediaItemID shared.MediaItemID) ([]library.CastMember, error) {
	ctx, cancel := withTimeout(ctx, defaultQueryTimeout)
	defer cancel()
	rows, err := r.db.QueryContext(ctx,
		`SELECT mp.person_id, p.name, p.thumb_path, mp.role_type, mp.character_name, mp.sort_order
		 FROM media_people mp
		 JOIN people p ON p.id = mp.person_id
		 WHERE mp.media_item_id = $1
		 ORDER BY mp.sort_order`, string(mediaItemID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []library.CastMember
	for rows.Next() {
		var cm library.CastMember
		if err := rows.Scan(&cm.PersonID, &cm.PersonName, &cm.ThumbPath, &cm.RoleType, &cm.CharacterName, &cm.SortOrder); err != nil {
			return nil, err
		}
		result = append(result, cm)
	}
	return result, rows.Err()
}

// GetCastForItems fetches cast for multiple media items in one query — eliminates N+1 in gallery listing.
func (r *MediaItemQueryRepoAdapter) GetCastForItems(ctx context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]library.CastMember, error) {
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
		return nil, err
	}
	defer rows.Close()

	result := make(map[shared.MediaItemID][]library.CastMember)
	for rows.Next() {
		var mediaItemID string
		var cm library.CastMember
		if err := rows.Scan(&mediaItemID, &cm.PersonID, &cm.PersonName, &cm.ThumbPath, &cm.RoleType, &cm.CharacterName, &cm.SortOrder); err != nil {
			return nil, err
		}
		result[shared.MediaItemID(mediaItemID)] = append(result[shared.MediaItemID(mediaItemID)], cm)
	}
	return result, rows.Err()
}

func (r *MediaItemQueryRepoAdapter) SeriesAggregate(ctx context.Context, seriesID shared.MediaItemID) (library.SeriesAggregate, error) {
	return r.items.SeriesAggregate(ctx, seriesID)
}

var _ library.MediaItemQueryRepo = (*MediaItemQueryRepoAdapter)(nil)
