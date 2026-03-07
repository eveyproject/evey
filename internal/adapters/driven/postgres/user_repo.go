package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/user"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(_ context.Context, _ *user.User) error {
	return errors.New("TODO: implement SQL insert")
}

func (r *UserRepository) GetByID(_ context.Context, _ string) (*user.User, error) {
	return nil, errors.New("TODO: implement SQL select")
}

func (r *UserRepository) GetByUsername(_ context.Context, _ string) (*user.User, error) {
	return nil, errors.New("TODO: implement SQL select by username")
}

func (r *UserRepository) GetByOIDCSubject(_ context.Context, _, _ string) (*user.User, error) {
	return nil, errors.New("TODO: implement SQL select by OIDC subject")
}

func (r *UserRepository) List(_ context.Context) ([]*user.User, error) {
	return nil, errors.New("TODO: implement SQL select all")
}

func (r *UserRepository) Update(_ context.Context, _ *user.User) error {
	return errors.New("TODO: implement SQL update")
}

func (r *UserRepository) Delete(_ context.Context, _ string) error {
	return errors.New("TODO: implement SQL delete")
}

func (r *UserRepository) CountAdmins(_ context.Context) (int, error) {
	return 0, errors.New("TODO: implement SQL count where is_admin")
}
