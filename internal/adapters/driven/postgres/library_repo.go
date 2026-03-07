package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type LibraryRepository struct {
	db *sql.DB
}

func NewLibraryRepository(db *sql.DB) *LibraryRepository {
	return &LibraryRepository{db: db}
}

func (r *LibraryRepository) Create(_ context.Context, _ *library.Library) error {
	return errors.New("TODO: implement SQL insert")
}

func (r *LibraryRepository) GetByID(_ context.Context, _ string) (*library.Library, error) {
	return nil, errors.New("TODO: implement SQL select")
}

func (r *LibraryRepository) List(_ context.Context) ([]*library.Library, error) {
	return nil, errors.New("TODO: implement SQL select all")
}

func (r *LibraryRepository) Update(_ context.Context, _ *library.Library) error {
	return errors.New("TODO: implement SQL update")
}

func (r *LibraryRepository) Delete(_ context.Context, _ string) error {
	return errors.New("TODO: implement SQL delete")
}
