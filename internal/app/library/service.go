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
	prober       library.StreamProber
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
	prober library.StreamProber,
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
		prober:       prober,
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

func (s *Service) ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error) {
	var lib *library.Library
	lib, err = s.libraries.GetByID(ctx, libraryID)
	if err != nil {
		return 0, 0, fmt.Errorf("get library: %w", err)
	}

	s.events.Publish(ctx, library.LibraryScanStarted{
		LibraryID:  libraryID,
		OccurredAt: time.Now(),
	})

	for _, path := range lib.Paths {
		var files []library.ScannedFile
		files, err = s.scanner.Scan(ctx, path)
		if err != nil {
			return 0, 0, fmt.Errorf("scan path %s: %w", path, err)
		}
		if lib.Type == "tvshow" {
			a, u, e := s.scanTVShowFiles(ctx, libraryID, files)
			if e != nil {
				return 0, 0, e
			}
			added += a
			updated += u
		} else {
			for _, f := range files {
				a, u, e := s.upsertMediaFile(ctx, libraryID, "", f)
				if e != nil {
					return 0, 0, e
				}
				added += a
				updated += u
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
	return added, updated, nil
}

// upsertMediaFile creates or updates a single media item. parentID may be empty.
func (s *Service) upsertMediaFile(ctx context.Context, libraryID, parentID string, f library.ScannedFile) (added, updated int, err error) {
	var existing *library.MediaItem
	var getErr error
	if f.Fingerprint != "" {
		existing, getErr = s.mediaItems.GetByFileFingerprint(ctx, f.Fingerprint)
		if errors.Is(getErr, library.ErrNotFound) {
			existing, getErr = s.mediaItems.GetByFilePath(ctx, f.Path)
		}
	} else {
		existing, getErr = s.mediaItems.GetByFilePath(ctx, f.Path)
	}
	if getErr != nil && !errors.Is(getErr, library.ErrNotFound) {
		return 0, 0, fmt.Errorf("check media item %s: %w", f.Path, getErr)
	}
	if errors.Is(getErr, library.ErrNotFound) {
		item := &library.MediaItem{
			ID:              uuid.New().String(),
			LibraryID:       libraryID,
			ParentID:        parentID,
			FilePath:        f.Path,
			FileSize:        f.Size,
			FileFingerprint: f.Fingerprint,
			Title:           f.Title,
			Type:            string(f.Type),
			Year:            f.Year,
			IndexNumber:     f.Episode,
			AddedAt:         time.Now(),
			UpdatedAt:       time.Now(),
		}
		if err = s.mediaItems.Create(ctx, item); err != nil {
			return 0, 0, fmt.Errorf("create media item: %w", err)
		}
		if dur := s.ProbeAndStoreStreams(ctx, item.ID, f.Path); dur > 0 {
			item.DurationMs = dur
			_ = s.mediaItems.Update(ctx, item)
		}
		s.search.IndexMediaItem(ctx, item)
		s.events.Publish(ctx, library.MediaItemAdded{
			MediaItemID: item.ID,
			LibraryID:   libraryID,
			Title:       item.Title,
			Type:        item.Type,
			OccurredAt:  time.Now(),
		})
		return 1, 0, nil
	}
	existing.ParentID = parentID
	existing.FilePath = f.Path
	existing.FileSize = f.Size
	existing.FileFingerprint = f.Fingerprint
	existing.UpdatedAt = time.Now()
	if dur := s.ProbeAndStoreStreams(ctx, existing.ID, f.Path); dur > 0 {
		existing.DurationMs = dur
	}
	s.mediaItems.Update(ctx, existing)
	s.search.IndexMediaItem(ctx, existing)
	// Re-emit so enricher re-fetches metadata for items with no overview (e.g. after overview clear)
	if existing.Overview == "" {
		s.events.Publish(ctx, library.MediaItemAdded{
			MediaItemID: existing.ID,
			LibraryID:   existing.LibraryID,
			Title:       existing.Title,
			Type:        existing.Type,
			OccurredAt:  time.Now(),
		})
	}
	return 0, 1, nil
}

func (s *Service) ProbeAndStoreStreams(ctx context.Context, mediaItemID, filePath string) int64 {
	if s.prober == nil {
		return 0
	}
	result, err := s.prober.ProbeStreams(ctx, filePath)
	if err != nil {
		return 0
	}
	for _, st := range result.Streams {
		st.MediaItemID = mediaItemID
	}
	_ = s.mediaStreams.DeleteByMediaItemID(ctx, mediaItemID)
	_ = s.mediaStreams.CreateBatch(ctx, result.Streams)
	return result.DurationMs
}

func (s *Service) GetMediaStreams(ctx context.Context, mediaItemID string) ([]*library.MediaStream, error) {
	return s.mediaStreams.GetByMediaItemID(ctx, mediaItemID)
}

// getOrCreateItem finds an item by fingerprint, then by library+type+title, or creates it.
func (s *Service) getOrCreateItem(ctx context.Context, item *library.MediaItem) (*library.MediaItem, bool, error) {
	existing, err := s.mediaItems.GetByFileFingerprint(ctx, item.FileFingerprint)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, library.ErrNotFound) {
		return nil, false, err
	}
	// Fallback: find by library+type+title to avoid creating duplicates on rescan
	existing, err = s.mediaItems.GetByLibraryTypeTitle(ctx, item.LibraryID, item.Type, item.Title, item.ParentID)
	if err == nil {
		existing.FileFingerprint = item.FileFingerprint
		_ = s.mediaItems.Update(ctx, existing)
		return existing, false, nil
	}
	if !errors.Is(err, library.ErrNotFound) {
		return nil, false, fmt.Errorf("lookup by title %q: %w", item.Title, err)
	}
	if err = s.mediaItems.Create(ctx, item); err != nil {
		return nil, false, err
	}
	return item, true, nil
}

// scanTVShowFiles builds series→season→episode hierarchy for a tvshow library.
func (s *Service) scanTVShowFiles(ctx context.Context, libraryID string, files []library.ScannedFile) (added, updated int, err error) {
	seriesIDs := map[string]string{}        // showTitle → series item ID
	seasonIDs := map[string]string{}        // "showTitle:seasonNum" → season item ID

	for _, f := range files {
		showTitle := f.Title
		seasonNum := 0
		if f.Season != nil {
			seasonNum = *f.Season
		}

		// 1. series item
		seriesID, ok := seriesIDs[showTitle]
		if !ok {
			fp := fmt.Sprintf("series:%s:%s", libraryID, showTitle)
			seriesItem, created, createErr := s.getOrCreateItem(ctx, &library.MediaItem{
				ID:              uuid.New().String(),
				LibraryID:       libraryID,
				Type:            "series",
				Title:           showTitle,
				FileFingerprint: fp,
				AddedAt:         time.Now(),
				UpdatedAt:       time.Now(),
			})
			if createErr != nil {
				return 0, 0, fmt.Errorf("create series %q: %w", showTitle, createErr)
			}
			if created {
				s.events.Publish(ctx, library.MediaItemAdded{
					MediaItemID: seriesItem.ID,
					LibraryID:   libraryID,
					Title:       seriesItem.Title,
					Type:        seriesItem.Type,
					OccurredAt:  time.Now(),
				})
				added++
			}
			seriesID = seriesItem.ID
			seriesIDs[showTitle] = seriesID
		}

		// 2. season item
		seasonKey := fmt.Sprintf("%s:%d", showTitle, seasonNum)
		seasonID, ok := seasonIDs[seasonKey]
		if !ok {
			fp := fmt.Sprintf("season:%s:%s:%d", libraryID, showTitle, seasonNum)
			n := seasonNum
			seasonItem, created, createErr := s.getOrCreateItem(ctx, &library.MediaItem{
				ID:              uuid.New().String(),
				LibraryID:       libraryID,
				ParentID:        seriesID,
				Type:            "season",
				Title:           fmt.Sprintf("Season %d", seasonNum),
				FileFingerprint: fp,
				IndexNumber:     &n,
				AddedAt:         time.Now(),
				UpdatedAt:       time.Now(),
			})
			if createErr != nil {
				return 0, 0, fmt.Errorf("create season %d for %q: %w", seasonNum, showTitle, createErr)
			}
			if created {
				added++
			}
			seasonID = seasonItem.ID
			seasonIDs[seasonKey] = seasonID
		}

		// 3. episode item — title as S01E05 (real episode name comes from TMDB enrichment)
		episodeFile := f
		if f.Episode != nil {
			s := 0
			if f.Season != nil {
				s = *f.Season
			}
			episodeFile.Title = fmt.Sprintf("S%02dE%02d", s, *f.Episode)
		}
		a, u, episodeErr := s.upsertMediaFile(ctx, libraryID, seasonID, episodeFile)
		if episodeErr != nil {
			return 0, 0, episodeErr
		}
		added += a
		updated += u
	}
	return added, updated, nil
}

func (s *Service) GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error) {
	return s.mediaItems.GetByID(ctx, id)
}

func (s *Service) ListMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	return s.mediaItems.List(ctx, filter)
}

func (s *Service) CountMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	return s.mediaItems.Count(ctx, filter)
}

// GetCast returns the cast for a media item joined with person info.
func (s *Service) GetCast(ctx context.Context, mediaItemID string) ([]*library.MediaPerson, []*library.Person, error) {
	mediaPersons, err := s.mediaPersons.GetByMediaItemID(ctx, mediaItemID)
	if err != nil {
		return nil, nil, fmt.Errorf("get media people: %w", err)
	}
	var persons []*library.Person
	for _, mp := range mediaPersons {
		p, err := s.persons.GetByID(ctx, mp.PersonID)
		if err != nil {
			continue
		}
		persons = append(persons, p)
	}
	return mediaPersons, persons, nil
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
