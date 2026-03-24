package collection

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type QueryService struct {
	query       library.CollectionQueryRepo
	smartQuerier SmartQuerier
}

func NewQueryService(query library.CollectionQueryRepo, smartQuerier SmartQuerier) *QueryService {
	return &QueryService{query: query, smartQuerier: smartQuerier}
}

func (s *QueryService) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	return s.query.GetByID(ctx, id)
}

func (s *QueryService) List(ctx context.Context) ([]*library.Collection, error) {
	return s.query.List(ctx)
}

func (s *QueryService) GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, int, error) {
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

func (s *QueryService) CountItems(ctx context.Context, collectionID string) (int, error) {
	return s.query.CountItems(ctx, collectionID)
}
