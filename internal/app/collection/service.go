package collection

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
)

var validSortOrders = map[string]bool{"asc": true, "desc": true}

func validateSortOrder(sortOrder string) error {
	if sortOrder == "" {
		return nil
	}
	if !validSortOrders[strings.ToLower(sortOrder)] {
		return fmt.Errorf("invalid sort order %q: must be asc or desc", sortOrder)
	}
	return nil
}

type Service struct {
	commands library.CollectionCommandRepo
	queries  library.CollectionQueryRepo
}

func NewService(commands library.CollectionCommandRepo, queries library.CollectionQueryRepo) *Service {
	return &Service{commands: commands, queries: queries}
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
	if err := validateSortOrder(col.SortOrder); err != nil {
		return err
	}
	return s.commands.Create(ctx, col)
}

func (s *Service) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	return s.queries.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*library.Collection, error) {
	return s.queries.List(ctx)
}

func (s *Service) Update(ctx context.Context, col *library.Collection) error {
	if err := validateSortOrder(col.SortOrder); err != nil {
		return err
	}
	existing, err := s.queries.GetByID(ctx, col.ID)
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
	return s.commands.Update(ctx, existing)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.commands.Delete(ctx, id)
}

func (s *Service) GetItems(ctx context.Context, id string, limit, offset int) ([]*library.MediaItem, int, error) {
	col, err := s.queries.GetByID(ctx, id)
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
		var items []*library.MediaItem
		items, err = s.queries.GetSmartItems(ctx, col, limit, offset)
		if err != nil {
			return nil, 0, err
		}
		var total int
		total, err = s.queries.CountSmartItems(ctx, col)
		if err != nil {
			return nil, 0, err
		}
		return items, total, nil
	}

	items, err := s.queries.GetManualItems(ctx, col.ID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.queries.CountManualItems(ctx, col.ID)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) AddItem(ctx context.Context, collectionID, mediaItemID string) error {
	return s.commands.AddItem(ctx, collectionID, mediaItemID)
}

func (s *Service) RemoveItem(ctx context.Context, collectionID, mediaItemID string) error {
	return s.commands.RemoveItem(ctx, collectionID, mediaItemID)
}
