package postgres

import (
	"context"
	"database/sql"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type MediaPersonRepository struct {
	db *sql.DB
}

func NewMediaPersonRepository(db *sql.DB) *MediaPersonRepository {
	return &MediaPersonRepository{db: db}
}

func (r *MediaPersonRepository) Create(ctx context.Context, mp *library.MediaPerson) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO media_people (media_item_id, person_id, role_type, character_name, sort_order)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (media_item_id, person_id, role_type) DO NOTHING`,
		mp.MediaItemID, mp.PersonID, mp.RoleType, mp.CharacterName, mp.SortOrder,
	)
	return err
}

func (r *MediaPersonRepository) DeleteByMediaItemID(ctx context.Context, mediaItemID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM media_people WHERE media_item_id = $1`, mediaItemID)
	return err
}

func (r *MediaPersonRepository) GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*library.MediaPerson, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT media_item_id, person_id, role_type, character_name, sort_order
		 FROM media_people WHERE media_item_id = $1 ORDER BY sort_order`, mediaItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*library.MediaPerson
	for rows.Next() {
		mp := &library.MediaPerson{}
		if err := rows.Scan(&mp.MediaItemID, &mp.PersonID, &mp.RoleType, &mp.CharacterName, &mp.SortOrder); err != nil {
			return nil, err
		}
		result = append(result, mp)
	}
	return result, rows.Err()
}

var _ library.MediaPersonRepository = (*MediaPersonRepository)(nil)
