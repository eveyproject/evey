package library

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type Service struct {
	libraries    library.LibraryRepository
	mediaItems   library.MediaItemRepository
	mediaStreams  library.MediaStreamRepository
	persons      library.PersonRepository
	mediaPersons library.MediaPersonRepository
	scanner      library.FileScanner
	search       library.SearchIndex
	events       shared.EventBus
}

func NewService(
	libraries library.LibraryRepository,
	mediaItems library.MediaItemRepository,
	mediaStreams library.MediaStreamRepository,
	persons library.PersonRepository,
	mediaPersons library.MediaPersonRepository,
	scanner library.FileScanner,
	search library.SearchIndex,
	events shared.EventBus,
) *Service {
	return &Service{
		libraries:    libraries,
		mediaItems:   mediaItems,
		mediaStreams:  mediaStreams,
		persons:      persons,
		mediaPersons: mediaPersons,
		scanner:      scanner,
		search:       search,
		events:       events,
	}
}

func (s *Service) CreateLibrary(ctx context.Context, lib *library.Library) error {
	now := time.Now()
	lib.CreatedAt = now
	lib.UpdatedAt = now
	if err := s.libraries.Create(ctx, lib); err != nil {
		return fmt.Errorf("create library: %w", err)
	}
	s.events.Publish(ctx, library.LibraryCreated{
		LibraryID:  lib.ID,
		Name:       lib.Name,
		Type:       lib.Type,
		OccurredAt: now,
	})
	return nil
}

func (s *Service) GetLibrary(ctx context.Context, id string) (*library.Library, error) {
	return s.libraries.GetByID(ctx, id)
}

func (s *Service) ListLibraries(ctx context.Context) ([]*library.Library, error) {
	return s.libraries.List(ctx)
}

func (s *Service) DeleteLibrary(ctx context.Context, id string) error {
	if err := s.libraries.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete library: %w", err)
	}
	s.events.Publish(ctx, library.LibraryDeleted{
		LibraryID:  id,
		OccurredAt: time.Now(),
	})
	return nil
}

func (s *Service) ScanLibrary(ctx context.Context, libraryID string) error {
	lib, err := s.libraries.GetByID(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("get library: %w", err)
	}

	s.events.Publish(ctx, library.LibraryScanStarted{
		LibraryID:  libraryID,
		OccurredAt: time.Now(),
	})

	var added, updated int
	for _, path := range lib.Paths {
		files, err := s.scanner.Scan(ctx, path)
		if err != nil {
			return fmt.Errorf("scan path %s: %w", path, err)
		}
		for _, f := range files {
			existing, err := s.mediaItems.GetByFilePath(ctx, f.Path)
			if err != nil && !errors.Is(err, library.ErrNotFound) {
				return fmt.Errorf("check media item %s: %w", f.Path, err)
			}
			if errors.Is(err, library.ErrNotFound) {
				item := &library.MediaItem{
					ID:          uuid.New().String(),
					LibraryID:   libraryID,
					FilePath:    f.Path,
					FileSize:    f.Size,
					Title:       f.Title,
					Type:        string(f.Type),
					Year:        f.Year,
					IndexNumber: f.Episode,
					AddedAt:     time.Now(),
					UpdatedAt:   time.Now(),
				}
				if err := s.mediaItems.Create(ctx, item); err != nil {
					return fmt.Errorf("create media item: %w", err)
				}
				s.search.IndexMediaItem(ctx, item)
				added++
				s.events.Publish(ctx, library.MediaItemAdded{
					MediaItemID: item.ID,
					LibraryID:   libraryID,
					Title:       item.Title,
					Type:        item.Type,
					Genres:      item.Genres,
					OccurredAt:  time.Now(),
				})
			} else {
				existing.FileSize = f.Size
				existing.UpdatedAt = time.Now()
				s.mediaItems.Update(ctx, existing)
				s.search.IndexMediaItem(ctx, existing)
				updated++
			}
		}
	}

	now := time.Now()
	lib.LastScannedAt = &now
	s.libraries.Update(ctx, lib)

	s.events.Publish(ctx, library.LibraryScanCompleted{
		LibraryID:    libraryID,
		ItemsAdded:   added,
		ItemsUpdated: updated,
		OccurredAt:   now,
	})
	return nil
}

func (s *Service) GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error) {
	return s.mediaItems.GetByID(ctx, id)
}

func (s *Service) ListMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return s.mediaItems.List(ctx, filter)
}

func (s *Service) SearchMedia(ctx context.Context, query string, filters map[string]string) ([]*library.MediaItem, error) {
	ids, err := s.search.Search(ctx, query, filters)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	var items []*library.MediaItem
	for _, id := range ids {
		item, err := s.mediaItems.GetByID(ctx, id)
		if err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}
