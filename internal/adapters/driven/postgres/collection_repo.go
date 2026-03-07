package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type CollectionRepository struct {
	db *sql.DB
}

func NewCollectionRepository(db *sql.DB) *CollectionRepository {
	return &CollectionRepository{db: db}
}

func (r *CollectionRepository) Create(ctx context.Context, col *library.Collection) error {
	rulesJSON, err := json.Marshal(col.FilterRules)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO collections (id, name, overview, type, filter_rules, sort_field, sort_order, poster_path, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		col.ID, col.Name, col.Overview, col.Type, rulesJSON, col.SortField, col.SortOrder, col.PosterPath, col.CreatedAt,
	)
	return err
}

func (r *CollectionRepository) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	return r.scanCollection(r.db.QueryRowContext(ctx,
		`SELECT id, name, overview, type, filter_rules, sort_field, sort_order, poster_path, created_at
		 FROM collections WHERE id = $1`, id))
}

func (r *CollectionRepository) List(ctx context.Context) ([]*library.Collection, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, overview, type, filter_rules, sort_field, sort_order, poster_path, created_at
		 FROM collections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.Collection
	for rows.Next() {
		col := &library.Collection{}
		var rulesJSON []byte
		if err := rows.Scan(&col.ID, &col.Name, &col.Overview, &col.Type, &rulesJSON, &col.SortField, &col.SortOrder, &col.PosterPath, &col.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(rulesJSON, &col.FilterRules)
		result = append(result, col)
	}
	return result, rows.Err()
}

func (r *CollectionRepository) Update(ctx context.Context, col *library.Collection) error {
	rulesJSON, err := json.Marshal(col.FilterRules)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE collections SET name=$1, overview=$2, type=$3, filter_rules=$4, sort_field=$5, sort_order=$6, poster_path=$7
		 WHERE id=$8`,
		col.Name, col.Overview, col.Type, rulesJSON, col.SortField, col.SortOrder, col.PosterPath, col.ID,
	)
	return err
}

func (r *CollectionRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM collections WHERE id = $1`, id)
	return err
}

func (r *CollectionRepository) AddItem(ctx context.Context, collectionID, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO collection_items (collection_id, media_item_id) VALUES ($1,$2)
		 ON CONFLICT (collection_id, media_item_id) DO NOTHING`,
		collectionID, mediaItemID,
	)
	return err
}

func (r *CollectionRepository) RemoveItem(ctx context.Context, collectionID, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM collection_items WHERE collection_id = $1 AND media_item_id = $2`,
		collectionID, mediaItemID)
	return err
}

func (r *CollectionRepository) GetItems(ctx context.Context, collectionID string) ([]*library.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT mi.id, mi.library_id, mi.parent_id, mi.type, mi.title, mi.sort_title, mi.original_title,
		 mi.overview, mi.tagline, mi.file_path, mi.file_size, mi.duration_ms, mi.bitrate, mi.container_format,
		 mi.index_number, mi.community_rating, mi.official_rating, mi.premiere_date, mi.year,
		 mi.provider_ids, mi.genres, mi.studios, mi.tags, mi.added_at, mi.updated_at
		 FROM media_items mi
		 INNER JOIN collection_items ci ON ci.media_item_id = mi.id
		 WHERE ci.collection_id = $1
		 ORDER BY mi.sort_title`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaItem
	for rows.Next() {
		item := &library.MediaItem{}
		var parentID, filePath *string
		var providerJSON, genresJSON, studiosJSON, tagsJSON []byte
		if err := rows.Scan(
			&item.ID, &item.LibraryID, &parentID, &item.Type, &item.Title, &item.SortTitle, &item.OriginalTitle,
			&item.Overview, &item.Tagline, &filePath, &item.FileSize, &item.DurationMs, &item.Bitrate,
			&item.ContainerFormat, &item.IndexNumber, &item.CommunityRating, &item.OfficialRating,
			&item.PremiereDate, &item.Year, &providerJSON, &genresJSON, &studiosJSON, &tagsJSON,
			&item.AddedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if parentID != nil {
			item.ParentID = *parentID
		}
		if filePath != nil {
			item.FilePath = *filePath
		}
		_ = json.Unmarshal(providerJSON, &item.ProviderIDs)
		_ = json.Unmarshal(genresJSON, &item.Genres)
		_ = json.Unmarshal(studiosJSON, &item.Studios)
		_ = json.Unmarshal(tagsJSON, &item.Tags)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *CollectionRepository) scanCollection(row *sql.Row) (*library.Collection, error) {
	col := &library.Collection{}
	var rulesJSON []byte
	err := row.Scan(&col.ID, &col.Name, &col.Overview, &col.Type, &rulesJSON, &col.SortField, &col.SortOrder, &col.PosterPath, &col.CreatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(rulesJSON, &col.FilterRules)
	return col, nil
}
