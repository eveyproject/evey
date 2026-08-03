package collection

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type QueryService struct {
	query        library.CollectionQueryRepo
	smartQuerier SmartQuerier
}

func NewQueryService(query library.CollectionQueryRepo, smartQuerier SmartQuerier) *QueryService {
	return &QueryService{query: query, smartQuerier: smartQuerier}
}

func (s *QueryService) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.GetByID")
	defer span.End()
	return s.query.GetByID(ctx, id)
}

func (s *QueryService) List(ctx context.Context) ([]*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.List")
	defer span.End()
	return s.query.List(ctx)
}

func (s *QueryService) GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.GetItems")
	defer span.End()
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
		items, queryErr := s.smartQuerier.QueryByRules(ctx, col.Rules, sortBy, "asc", limit, offset)
		if queryErr != nil {
			return nil, 0, queryErr
		}
		total, countErr := s.smartQuerier.CountByRules(ctx, col.Rules)
		if countErr != nil {
			return nil, 0, countErr
		}
		return items, total, nil
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
	ctx, span := tracing.StartSpan(ctx, "app.collection.CountItems")
	defer span.End()
	return s.query.CountItems(ctx, collectionID)
}

// GetItemsForCollections batches item lookups for multiple collections into a
// single repository call. Used by ListCollections to avoid the N+1 pattern.
func (s *QueryService) GetItemsForCollections(ctx context.Context, collectionIDs []string) (map[string][]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.GetItemsForCollections")
	defer span.End()
	return s.query.GetItemsByCollectionIDs(ctx, collectionIDs)
}

// ListMoviesWithoutCollection is the saga-repair work list, delegated to the
// repository. See metadata.Enricher.RelinkCollections for why it exists.
func (s *QueryService) ListMoviesWithoutCollection(ctx context.Context, afterID string, limit int) ([]library.UnlinkedMovie, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.ListMoviesWithoutCollection")
	defer span.End()
	return s.query.ListMoviesWithoutCollection(ctx, afterID, limit)
}

// GetForMediaItem returns the collections holding this title.
func (s *QueryService) GetForMediaItem(ctx context.Context, mediaItemID string) ([]*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.GetForMediaItem")
	defer span.End()
	return s.query.GetByMediaItemID(ctx, mediaItemID)
}
