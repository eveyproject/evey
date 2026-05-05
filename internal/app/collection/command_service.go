package collection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type CommandService struct {
	cmd        library.CollectionCommandRepo
	query      library.CollectionQueryRepo
	mediaItems MediaItemGetter
}

func NewCommandService(cmd library.CollectionCommandRepo, query library.CollectionQueryRepo, mediaItems MediaItemGetter) *CommandService {
	return &CommandService{cmd: cmd, query: query, mediaItems: mediaItems}
}

func (s *CommandService) Create(ctx context.Context, name string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.Create")
	defer span.End()
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

func (s *CommandService) Update(ctx context.Context, id, name string, rules []library.CollectionRule, sortBy string) error {
	ctx, span := tracing.StartSpan(ctx, "app.collection.Update")
	defer span.End()
	col, err := s.query.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("collection not found: %w", err)
	}
	col.Name = name
	col.Rules = rules
	col.SortBy = sortBy
	col.UpdatedAt = time.Now()
	return s.cmd.Upsert(ctx, col)
}

func (s *CommandService) Upsert(ctx context.Context, tmdbID int, name, overview, posterURL, backdropURL string) (*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.Upsert")
	defer span.End()
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
	return col, nil
}

func (s *CommandService) CreateSmart(ctx context.Context, name string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.CreateSmart")
	defer span.End()
	if name == "" {
		return nil, errors.New("name is required")
	}
	if len(rules) == 0 {
		return nil, errors.New("at least one rule is required")
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

func (s *CommandService) UpdateSmart(ctx context.Context, id, name string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.UpdateSmart")
	defer span.End()
	col, err := s.query.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, errors.New("name is required")
	}
	if len(rules) == 0 {
		return nil, errors.New("at least one rule is required")
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

func (s *CommandService) LinkMediaItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error {
	ctx, span := tracing.StartSpan(ctx, "app.collection.LinkMediaItem")
	defer span.End()
	if _, err := s.query.GetByID(ctx, collectionID); err != nil {
		return fmt.Errorf("collection not found: %w", err)
	}
	if _, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID)); err != nil {
		return fmt.Errorf("media item not found: %w", err)
	}
	return s.cmd.LinkItem(ctx, collectionID, mediaItemID, sortOrder)
}

func (s *CommandService) Delete(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.collection.Delete")
	defer span.End()
	return s.cmd.Delete(ctx, id)
}
