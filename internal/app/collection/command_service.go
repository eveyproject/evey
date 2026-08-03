package collection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/google/uuid"
)

type CommandService struct {
	cmd        library.CollectionCommandRepo
	query      library.CollectionQueryRepo
	mediaItems MediaItemGetter
}

func NewCommandService(cmd library.CollectionCommandRepo, query library.CollectionQueryRepo, mediaItems MediaItemGetter) *CommandService {
	return &CommandService{cmd: cmd, query: query, mediaItems: mediaItems}
}

func (s *CommandService) Create(ctx context.Context, name, overview string, rules []library.CollectionRule, sortBy string) (*library.Collection, error) {
	ctx, span := tracing.StartSpan(ctx, "app.collection.Create")
	defer span.End()
	col, err := library.NewCollection(uuid.New().String(), name, rules, sortBy)
	if err != nil {
		return nil, err
	}
	col.Overview = overview
	if err := s.cmd.Create(ctx, col); err != nil {
		return nil, err
	}
	return col, nil
}

func (s *CommandService) Update(ctx context.Context, id, name string, overview *string, rules []library.CollectionRule, sortBy string) error {
	ctx, span := tracing.StartSpan(ctx, "app.collection.Update")
	defer span.End()
	col, err := s.query.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("collection not found: %w", err)
	}
	col.Name = name
	// Absent means keep: a client editing only the name must not wipe the
	// synopsis a TMDB saga came with.
	if overview != nil {
		col.Overview = *overview
	}
	col.Rules = rules
	col.SortBy = sortBy
	col.UpdatedAt = time.Now()
	// Update, not Upsert: the upsert arbitrates on tmdb_id, which is NULL for
	// every hand-built collection, so the insert fell through to a primary-key
	// violation - renaming one returned 500, and the sort_by=manual write that
	// follows a reorder failed the same way (reproduced on dev 2026-08-04).
	return s.cmd.Update(ctx, col)
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
	// The TMDB path builds its own shape (TmdbID + artwork) rather than going
	// through NewCollection, but it owes the same invariants. Its input is a
	// THIRD PARTY's payload - a collection ref whose name comes back empty
	// otherwise lands as a `name = ''` row (the column is NOT NULL but the
	// empty string satisfies it), and the library then shows a nameless
	// collection nobody can identify or search for. Refusing here means the
	// film simply is not linked, and the caller logs it (Enricher.linkCollection).
	if err := col.Validate(); err != nil {
		return nil, err
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
	col, err := library.NewCollection(uuid.New().String(), name, rules, sortBy)
	if err != nil {
		return nil, err
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

// UnlinkMediaItem removes one member from a collection.
func (s *CommandService) UnlinkMediaItem(ctx context.Context, collectionID, mediaItemID string) error {
	return s.cmd.UnlinkItem(ctx, collectionID, mediaItemID)
}

// ReorderItems stamps the hand-set order of a collection's members. Ordering
// only takes effect for a collection whose SortBy is "manual"; the caller is
// responsible for that choice, so a reorder never silently rewrites how an
// auto-ordered saga reads.
func (s *CommandService) ReorderItems(ctx context.Context, collectionID string, mediaItemIDs []string) error {
	return s.cmd.SetItemOrder(ctx, collectionID, mediaItemIDs)
}
