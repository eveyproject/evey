package collection

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
)

type Service struct {
	collections library.CollectionRepository
}

func NewService(collections library.CollectionRepository) *Service {
	return &Service{collections: collections}
}

func (s *Service) Create(ctx context.Context, col *library.Collection) error {
	if col.ID == "" {
		col.ID = uuid.New().String()
	}
	now := time.Now()
	col.CreatedAt = now
	col.UpdatedAt = now
	if col.SortBy == "" {
		col.SortBy = "sort_title"
	}
	if col.SortOrder == "" {
		col.SortOrder = "asc"
	}
	return s.collections.Create(ctx, col)
}

func (s *Service) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	return s.collections.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*library.Collection, error) {
	return s.collections.List(ctx)
}

func (s *Service) Update(ctx context.Context, col *library.Collection) error {
	existing, err := s.collections.GetByID(ctx, col.ID)
	if err != nil {
		return fmt.Errorf("get collection: %w", err)
	}
	existing.Name = col.Name
	existing.Overview = col.Overview
	existing.Type = col.Type
	existing.Rules = col.Rules
	existing.SortBy = col.SortBy
	existing.SortOrder = col.SortOrder
	existing.UpdatedAt = time.Now()
	return s.collections.Update(ctx, existing)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.collections.Delete(ctx, id)
}

func (s *Service) GetItems(ctx context.Context, id string, limit, offset int) ([]*library.MediaItem, int, error) {
	col, err := s.collections.GetByID(ctx, id)
	if err != nil {
		return nil, 0, fmt.Errorf("get collection: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	if col.Type == library.CollectionSmart {
		items, err := s.collections.GetSmartItems(ctx, col, limit, offset)
		if err != nil {
			return nil, 0, err
		}
		total, err := s.collections.CountSmartItems(ctx, col)
		if err != nil {
			return nil, 0, err
		}
		return items, total, nil
	}

	items, err := s.collections.GetManualItems(ctx, col.ID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.collections.CountManualItems(ctx, col.ID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) AddItem(ctx context.Context, collectionID, mediaItemID string) error {
	return s.collections.AddItem(ctx, collectionID, mediaItemID)
}

func (s *Service) RemoveItem(ctx context.Context, collectionID, mediaItemID string) error {
	return s.collections.RemoveItem(ctx, collectionID, mediaItemID)
}
