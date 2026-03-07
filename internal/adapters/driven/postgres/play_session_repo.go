package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/playback"
)

type PlaySessionRepository struct {
	db *sql.DB
}

func NewPlaySessionRepository(db *sql.DB) *PlaySessionRepository {
	return &PlaySessionRepository{db: db}
}

func (r *PlaySessionRepository) Create(_ context.Context, _ *playback.PlaySession) error {
	return errors.New("TODO: implement SQL insert")
}

func (r *PlaySessionRepository) Update(_ context.Context, _ *playback.PlaySession) error {
	return errors.New("TODO: implement SQL update")
}

func (r *PlaySessionRepository) GetByID(_ context.Context, _ string) (*playback.PlaySession, error) {
	return nil, errors.New("TODO: implement SQL select")
}

func (r *PlaySessionRepository) GetActiveByProfile(_ context.Context, _ string) ([]*playback.PlaySession, error) {
	return nil, errors.New("TODO: implement SQL select active by profile")
}

func (r *PlaySessionRepository) GetHistory(_ context.Context, _ playback.PlaySessionFilter) ([]*playback.PlaySession, error) {
	return nil, errors.New("TODO: implement SQL select with filters")
}

func (r *PlaySessionRepository) CountActiveByUser(_ context.Context, _ string) (int, error) {
	return 0, errors.New("TODO: implement SQL count active sessions by user")
}
