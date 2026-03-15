package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"

	"github.com/gravity-zero/evey/internal/domain/library"
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

func (r *MediaItemQueryRepoAdapter) GetByID(ctx context.Context, id string) (*library.MediaItem, error) {
	return r.items.GetByID(ctx, id)
}

func (r *MediaItemQueryRepoAdapter) List(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return r.items.List(ctx, filter)
}

func (r *MediaItemQueryRepoAdapter) Count(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	return r.items.Count(ctx, filter)
}

// GetCastByMediaItem returns cast members with person info in a single JOIN query.
func (r *MediaItemQueryRepoAdapter) GetCastByMediaItem(ctx context.Context, mediaItemID string) ([]library.CastMember, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT mp.person_id, p.name, p.thumb_path, mp.role_type, mp.character_name, mp.sort_order
		 FROM media_people mp
		 JOIN people p ON p.id = mp.person_id
		 WHERE mp.media_item_id = $1
		 ORDER BY mp.sort_order`, mediaItemID)
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
func (r *MediaItemQueryRepoAdapter) GetCastForItems(ctx context.Context, mediaItemIDs []string) (map[string][]library.CastMember, error) {
	if len(mediaItemIDs) == 0 {
		return map[string][]library.CastMember{}, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT mp.media_item_id, mp.person_id, p.name, p.thumb_path, mp.role_type, mp.character_name, mp.sort_order
		 FROM media_people mp
		 JOIN people p ON p.id = mp.person_id
		 WHERE mp.media_item_id = ANY($1)
		 ORDER BY mp.media_item_id, mp.sort_order`, pq.Array(mediaItemIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]library.CastMember)
	for rows.Next() {
		var mediaItemID string
		var cm library.CastMember
		if err := rows.Scan(&mediaItemID, &cm.PersonID, &cm.PersonName, &cm.ThumbPath, &cm.RoleType, &cm.CharacterName, &cm.SortOrder); err != nil {
			return nil, err
		}
		result[mediaItemID] = append(result[mediaItemID], cm)
	}
	return result, rows.Err()
}
