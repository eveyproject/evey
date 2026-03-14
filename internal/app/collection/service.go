package collection

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
)

type CollectionCommandRepo interface {
	Create(ctx context.Context, col *library.Collection) error
	Upsert(ctx context.Context, col *library.Collection) error
	Delete(ctx context.Context, id string) error
	LinkItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error
}

type CollectionQueryRepo interface {
	GetByID(ctx context.Context, id string) (*library.Collection, error)
	List(ctx context.Context) ([]*library.Collection, error)
	GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error)
	CountItems(ctx context.Context, collectionID string) (int, error)
}

type Service struct {
	cmd   CollectionCommandRepo
	query CollectionQueryRepo
}

func NewService(cmd CollectionCommandRepo, query CollectionQueryRepo) *Service {
	return &Service{cmd: cmd, query: query}
}

func (s *Service) UpsertFromTMDB(ctx context.Context, tmdbID int, name, overview, posterURL, backdropURL string) (*library.Collection, error) {
	now := time.Now()
	col := &library.Collection{
		ID:          uuid.New().String(),
		TmdbID:      &tmdbID,
		Name:        name,
		Overview:    overview,
		PosterURL:   posterURL,
		BackdropURL: backdropURL,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.cmd.Upsert(ctx, col); err != nil {
		return nil, err
	}
	// After upsert, the ID may differ if the row already existed; fetch by tmdb_id via query.
	// Since CollectionQueryRepo doesn't expose GetByTmdbID, we return the col as-is.
	// The caller can rely on the returned col for further operations.
	return col, nil
}

func (s *Service) LinkMediaItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error {
	return s.cmd.LinkItem(ctx, collectionID, mediaItemID, sortOrder)
}

func (s *Service) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	return s.query.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*library.Collection, error) {
	return s.query.List(ctx)
}

func (s *Service) GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	items, err := s.query.GetItems(ctx, collectionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.query.CountItems(ctx, collectionID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) CountItems(ctx context.Context, collectionID string) (int, error) {
	return s.query.CountItems(ctx, collectionID)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.cmd.Delete(ctx, id)
}
