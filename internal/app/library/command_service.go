package library

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type CommandService struct {
	libraries    library.LibraryCommandRepo
	libQueries   library.LibraryQueryRepo
	mediaItems   library.MediaItemCommandRepo
	mediaStreams  library.MediaStreamRepository
	persons      library.PersonRepository
	mediaPersons library.MediaPersonRepository
	chapters     library.ChapterRepository
	mkvReader    mkv.ContainerReader
	scanner      library.FileScanner
	prober       library.StreamProber
	search       library.SearchIndex
	events       shared.EventBus
}

func NewCommandService(
	libraries library.LibraryCommandRepo,
	libQueries library.LibraryQueryRepo,
	mediaItems library.MediaItemCommandRepo,
	mediaStreams library.MediaStreamRepository,
	persons library.PersonRepository,
	mediaPersons library.MediaPersonRepository,
	scanner library.FileScanner,
	prober library.StreamProber,
	search library.SearchIndex,
	events shared.EventBus,
) *CommandService {
	return &CommandService{
		libraries:    libraries,
		libQueries:   libQueries,
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

func (s *CommandService) WithChapters(chapters library.ChapterRepository, reader mkv.ContainerReader) {
	s.chapters = chapters
	s.mkvReader = reader
}

func (s *CommandService) GetChapters(ctx context.Context, mediaItemID string) ([]*library.Chapter, error) {
	if s.chapters == nil {
		return nil, nil
	}
	return s.chapters.GetByMediaItemID(ctx, mediaItemID)
}

func (s *CommandService) extractAndStoreChapters(ctx context.Context, mediaItemID, filePath string) {
	if s.chapters == nil || s.mkvReader == nil || !strings.HasSuffix(strings.ToLower(filePath), ".mkv") {
		return
	}
	mkvChapters, err := s.mkvReader.ReadChapters(ctx, filePath)
	if err != nil || len(mkvChapters) == 0 {
		return
	}
	chapters := make([]*library.Chapter, len(mkvChapters))
	for i, ch := range mkvChapters {
		chapters[i] = &library.Chapter{
			ID:          uuid.New().String(),
			MediaItemID: mediaItemID,
			Title:       ch.Title,
			StartTimeMs: ch.StartMs,
			EndTimeMs:   ch.EndMs,
			SortOrder:   i,
		}
	}
	_ = s.chapters.ReplaceBatch(ctx, mediaItemID, chapters)
}

func (s *CommandService) CreateLibrary(ctx context.Context, lib *library.Library) error {
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

func (s *CommandService) DeleteLibrary(ctx context.Context, id string) error {
	if err := s.libraries.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete library: %w", err)
	}
	s.events.Publish(ctx, library.LibraryDeleted{
		LibraryID:  id,
		OccurredAt: time.Now(),
	})
	return nil
}

func (s *CommandService) ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error) {
	var lib *library.Library
	lib, err = s.libQueries.GetByID(ctx, libraryID)
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

func (s *CommandService) upsertMediaFile(ctx context.Context, libraryID, parentID string, f library.ScannedFile) (added, updated int, err error) {
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
		s.extractAndStoreChapters(ctx, item.ID, f.Path)
		s.search.IndexMediaItem(ctx, item, nil)
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
	s.extractAndStoreChapters(ctx, existing.ID, f.Path)
	s.mediaItems.Update(ctx, existing)
	s.search.IndexMediaItem(ctx, existing, nil)
	// Re-emit so enricher re-fetches metadata for items with no overview
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

func (s *CommandService) ProbeAndStoreStreams(ctx context.Context, mediaItemID, filePath string) int64 {
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

func (s *CommandService) getOrCreateItem(ctx context.Context, item *library.MediaItem) (*library.MediaItem, bool, error) {
	existing, err := s.mediaItems.GetByFileFingerprint(ctx, item.FileFingerprint)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, library.ErrNotFound) {
		return nil, false, err
	}
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

func (s *CommandService) scanTVShowFiles(ctx context.Context, libraryID string, files []library.ScannedFile) (added, updated int, err error) {
	seriesIDs := map[string]string{}
	seasonIDs := map[string]string{}

	for _, f := range files {
		showTitle := f.Title
		seasonNum := 0
		if f.Season != nil {
			seasonNum = *f.Season
		}

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
