package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type MediaItemRepository struct {
	db *sql.DB
}

func NewMediaItemRepository(db *sql.DB) *MediaItemRepository {
	return &MediaItemRepository{db: db}
}

func (r *MediaItemRepository) Create(_ context.Context, _ *library.MediaItem) error {
	return errors.New("TODO: implement SQL insert")
}

func (r *MediaItemRepository) GetByID(_ context.Context, _ string) (*library.MediaItem, error) {
	return nil, errors.New("TODO: implement SQL select")
}

func (r *MediaItemRepository) List(_ context.Context, _ library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return nil, errors.New("TODO: implement SQL select with filters")
}

func (r *MediaItemRepository) Update(_ context.Context, _ *library.MediaItem) error {
	return errors.New("TODO: implement SQL update")
}

func (r *MediaItemRepository) Delete(_ context.Context, _ string) error {
	return errors.New("TODO: implement SQL delete")
}

func (r *MediaItemRepository) GetByFilePath(_ context.Context, _ string) (*library.MediaItem, error) {
	return nil, errors.New("TODO: implement SQL select by file_path")
}
