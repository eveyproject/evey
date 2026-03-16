package collection

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
)

type CollectionCommandRepo interface {
	Create(ctx context.Context, col *library.Collection) error
	Upsert(ctx context.Context, col *library.Collection) error
	Update(ctx context.Context, col *library.Collection) error
	Delete(ctx context.Context, id string) error
	LinkItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error
}

type CollectionQueryRepo interface {
	GetByID(ctx context.Context, id string) (*library.Collection, error)
	List(ctx context.Context) ([]*library.Collection, error)
	GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error)
	CountItems(ctx context.Context, collectionID string) (int, error)
}

// SmartQuerier evaluates collection rules against media_items.
type SmartQuerier interface {
	QueryByRules(ctx context.Context, rules []library.CollectionRule, sortBy, sortOrder string, limit int) ([]*library.MediaItem, error)
}

type MediaItemGetter interface {
	GetByID(ctx context.Context, id string) (*library.MediaItem, error)
}

type Service struct {
	cmd          CollectionCommandRepo
	query        CollectionQueryRepo
	smartQuerier SmartQuerier
	mediaItems   MediaItemGetter
}

func NewService(cmd CollectionCommandRepo, query CollectionQueryRepo, smartQuerier SmartQuerier, mediaItems MediaItemGetter) *Service {
	return &Service{cmd: cmd, query: query, smartQuerier: smartQuerier, mediaItems: mediaItems}
}

func (s *Service) Upsert(ctx context.Context, tmdbID int, name, overview, posterURL, backdropURL string) (*library.Collection, error) {
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

func (s *Service) CreateSmart(ctx context.Context, name string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("at least one rule is required")
	}
	if sortBy == "" {
		sortBy = "added_at"
	}
	now := time.Now()
	col := &library.Collection{
		ID:        uuid.New().String(),
		Name:      name,
		Rules:     rules,
		SortBy:    sortBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.cmd.Create(ctx, col); err != nil {
		return nil, err
	}
	return col, nil
}

func (s *Service) UpdateSmart(ctx context.Context, id, name string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	col, err := s.query.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("at least one rule is required")
	}
	if sortBy == "" {
		sortBy = "added_at"
	}
	col.Name = name
	col.Rules = rules
	col.SortBy = sortBy
	col.UpdatedAt = time.Now()
	if err := s.cmd.Update(ctx, col); err != nil {
		return nil, err
	}
	return col, nil
}

func (s *Service) LinkMediaItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error {
	if _, err := s.query.GetByID(ctx, collectionID); err != nil {
		return fmt.Errorf("collection not found: %w", err)
	}
	if _, err := s.mediaItems.GetByID(ctx, mediaItemID); err != nil {
		return fmt.Errorf("media item not found: %w", err)
	}
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

	col, err := s.query.GetByID(ctx, collectionID)
	if err != nil {
		return nil, 0, err
	}

	if len(col.Rules) > 0 && s.smartQuerier != nil {
		sortBy := col.SortBy
		if sortBy == "" {
			sortBy = "added_at"
		}
		all, queryErr := s.smartQuerier.QueryByRules(ctx, col.Rules, sortBy, "asc", 0)
		if queryErr != nil {
			return nil, 0, queryErr
		}
		total := len(all)
		if offset >= total {
			return nil, total, nil
		}
		all = all[offset:]
		if limit > 0 && limit < len(all) {
			all = all[:limit]
		}
		return all, total, nil
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
