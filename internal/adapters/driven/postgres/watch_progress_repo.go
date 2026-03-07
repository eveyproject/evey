package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/playback"
)

type WatchProgressRepository struct {
	db *sql.DB
}

func NewWatchProgressRepository(db *sql.DB) *WatchProgressRepository {
	return &WatchProgressRepository{db: db}
}

func (r *WatchProgressRepository) Upsert(_ context.Context, _ *playback.WatchProgress) error {
	return errors.New("TODO: implement SQL upsert")
}

func (r *WatchProgressRepository) GetByProfileAndMedia(_ context.Context, _, _ string) (*playback.WatchProgress, error) {
	return nil, errors.New("TODO: implement SQL select")
}

func (r *WatchProgressRepository) GetByProfile(_ context.Context, _ string, _, _ int) ([]*playback.WatchProgress, error) {
	return nil, errors.New("TODO: implement SQL select by profile")
}

func (r *WatchProgressRepository) GetResumeItems(_ context.Context, _ string, _ int) ([]*playback.WatchProgress, error) {
	return nil, errors.New("TODO: implement SQL select incomplete items")
}

func (r *WatchProgressRepository) GetNextUp(_ context.Context, _ string, _ int) ([]*playback.WatchProgress, error) {
	return nil, errors.New("TODO: implement SQL select next episodes")
}
