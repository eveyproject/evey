package collection

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

// SmartQuerier evaluates collection rules against media_items.
type SmartQuerier interface {
	QueryByRules(ctx context.Context, rules []library.CollectionRule, sortBy, sortOrder string, limit, offset int) ([]*library.MediaItem, error)
	// CountByRules returns how many media items match the rules - for accurate
	// pagination totals without materializing every hydrated row.
	CountByRules(ctx context.Context, rules []library.CollectionRule) (int, error)
}

// MediaItemGetter is a narrow read interface for media item lookups.
type MediaItemGetter interface {
	GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error)
}

// Service is a backward-compatible facade delegating to CommandService and QueryService.
type Service struct {
	Command *CommandService
	Query   *QueryService
}

func NewService(cmd library.CollectionCommandRepo, query library.CollectionQueryRepo, smartQuerier SmartQuerier, mediaItems MediaItemGetter) *Service {
	return &Service{
		Command: NewCommandService(cmd, query, mediaItems),
		Query:   NewQueryService(query, smartQuerier),
	}
}

func (s *Service) Create(ctx context.Context, name, overview string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	return s.Command.Create(ctx, name, overview, rules, sortBy)
}

func (s *Service) Update(ctx context.Context, id, name string, overview *string, rules []library.CollectionRule, sortBy string) error {
	return s.Command.Update(ctx, id, name, overview, rules, sortBy)
}

func (s *Service) Upsert(ctx context.Context, tmdbID int, name, overview, posterURL, backdropURL string) (*library.Collection, error) {
	return s.Command.Upsert(ctx, tmdbID, name, overview, posterURL, backdropURL)
}

func (s *Service) CreateSmart(ctx context.Context, name string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	return s.Command.CreateSmart(ctx, name, rules, sortBy)
}

func (s *Service) UpdateSmart(ctx context.Context, id, name string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	return s.Command.UpdateSmart(ctx, id, name, rules, sortBy)
}

func (s *Service) LinkMediaItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error {
	return s.Command.LinkMediaItem(ctx, collectionID, mediaItemID, sortOrder)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.Command.Delete(ctx, id)
}

func (s *Service) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	return s.Query.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*library.Collection, error) {
	return s.Query.List(ctx)
}

func (s *Service) GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, int, error) {
	return s.Query.GetItems(ctx, collectionID, limit, offset)
}

func (s *Service) CountItems(ctx context.Context, collectionID string) (int, error) {
	return s.Query.CountItems(ctx, collectionID)
}

func (s *Service) GetItemsForCollections(ctx context.Context, collectionIDs []string) (map[string][]*library.MediaItem, error) {
	return s.Query.GetItemsForCollections(ctx, collectionIDs)
}

func (s *Service) ListMoviesWithoutCollection(ctx context.Context, afterID string, limit int) ([]library.UnlinkedMovie, error) {
	return s.Query.ListMoviesWithoutCollection(ctx, afterID, limit)
}

func (s *Service) UnlinkMediaItem(ctx context.Context, collectionID, mediaItemID string) error {
	return s.Command.UnlinkMediaItem(ctx, collectionID, mediaItemID)
}

func (s *Service) ReorderItems(ctx context.Context, collectionID string, mediaItemIDs []string) error {
	return s.Command.ReorderItems(ctx, collectionID, mediaItemIDs)
}

func (s *Service) GetForMediaItem(ctx context.Context, mediaItemID string) ([]*library.Collection, error) {
	return s.Query.GetForMediaItem(ctx, mediaItemID)
}
