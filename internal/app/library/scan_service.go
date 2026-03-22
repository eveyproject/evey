package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type ScanService struct {
	scanner      library.FileScanner
	prober       library.StreamProber
	subScanner   library.ExternalSubtitleScanner
	search       library.SearchIndex
	events       shared.EventBus
	mediaItems   library.MediaItemCommandRepo
	libQueries   library.LibraryQueryRepo
	libraries    library.LibraryCommandRepo
	mediaStreams  library.MediaStreamRepository
	persons      library.PersonRepository
	mediaPersons library.MediaPersonRepository

	metadata *MetadataExtractionService

	scanMu   sync.Mutex
	scanning map[string]bool
}

func NewScanService(
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
) *ScanService {
	return &ScanService{
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
		scanning:     make(map[string]bool),
	}
}

func (s *ScanService) WithMetadataExtraction(m *MetadataExtractionService) {
	s.metadata = m
}

func (s *ScanService) WithSubtitleScanner(sc library.ExternalSubtitleScanner) {
	s.subScanner = sc
}

func (s *ScanService) ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error) {
	s.scanMu.Lock()
	if s.scanning[libraryID] {
		s.scanMu.Unlock()
		return 0, 0, fmt.Errorf("scan already in progress for library %s", libraryID)
	}
	s.scanning[libraryID] = true
	s.scanMu.Unlock()
	defer func() {
		s.scanMu.Lock()
		delete(s.scanning, libraryID)
		s.scanMu.Unlock()
	}()

	var lib *library.Library
	lib, err = s.libQueries.GetByID(ctx, libraryID)
	if err != nil {
		return 0, 0, fmt.Errorf("get library: %w", err)
	}

	evtScanStarted := library.LibraryScanStarted{
		LibraryID:  libraryID,
		OccurredAt: time.Now(),
	}
	if publishErr := s.events.Publish(ctx, evtScanStarted); publishErr != nil {
		slog.Warn("publish event", slog.String("event", evtScanStarted.EventName()), slog.Any("err", publishErr))
	}

	existing, err := s.mediaItems.ListByLibraryID(ctx, libraryID)
	if err != nil {
		return 0, 0, fmt.Errorf("list existing items: %w", err)
	}
	byPath := make(map[string]*library.MediaItem, len(existing))
	byFingerprint := make(map[string]*library.MediaItem, len(existing))
	for _, item := range existing {
		byPath[item.FilePath] = item
		if item.FileFingerprint != "" {
			byFingerprint[item.FileFingerprint] = item
		}
	}

	seenPaths := make(map[string]struct{})
	type scanWork struct {
		file      library.ScannedFile
		parentID  string
		showTitle string
	}
	var toProcess []scanWork

	for _, path := range lib.Paths {
		var files []library.ScannedFile
		files, err = s.scanner.Scan(ctx, path)
		if err != nil {
			return 0, 0, fmt.Errorf("scan path %s: %w", path, err)
		}

		if lib.Type == "tvshow" {
			for _, f := range files {
				seenPaths[f.Path] = struct{}{}
			}
			a, u, e := s.scanTVShowFiles(ctx, libraryID, files)
			if e != nil {
				return 0, 0, e
			}
			added += a
			updated += u
			continue
		}

		for _, f := range files {
			seenPaths[f.Path] = struct{}{}

			if ex := byPath[f.Path]; ex != nil {
				if ex.FileSize == f.Size && ex.ModTime.Equal(f.ModTime) && ex.FileFingerprint == f.Fingerprint {
					continue
				}
			}

			if byPath[f.Path] == nil && f.Fingerprint != "" {
				if moved := byFingerprint[f.Fingerprint]; moved != nil {
					moved.FilePath = f.Path
					moved.ModTime = f.ModTime
					moved.UpdatedAt = time.Now()
					if updateErr := s.mediaItems.Update(ctx, moved); updateErr != nil {
						slog.Warn("update moved item path", slog.String("id", moved.ID), slog.Any("err", updateErr))
					}
					updated++
					continue
				}
			}

			toProcess = append(toProcess, scanWork{file: f})
		}
	}

	var addedAtomic, updatedAtomic atomic.Int64
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for _, w := range toProcess {
		wg.Add(1)
		sem <- struct{}{}
		go func(w scanWork) {
			defer wg.Done()
			defer func() { <-sem }()
			a, u, e := s.upsertMediaFile(ctx, libraryID, w.parentID, w.file, w.showTitle)
			if e != nil {
				slog.Warn("upsert media file", slog.String("path", w.file.Path), slog.Any("err", e))
				return
			}
			addedAtomic.Add(int64(a))
			updatedAtomic.Add(int64(u))
		}(w)
	}
	wg.Wait()
	added += int(addedAtomic.Load())
	updated += int(updatedAtomic.Load())

	removed, removeErr := s.removeOrphanedItems(ctx, libraryID, seenPaths)
	if removeErr != nil {
		slog.Warn("orphan cleanup failed", slog.String("libraryID", libraryID), slog.Any("err", removeErr))
	}

	now := time.Now()
	lib.LastScannedAt = &now
	if err := s.libraries.Update(ctx, lib); err != nil {
		slog.Warn("update library last scanned failed", slog.String("libraryID", libraryID), slog.Any("err", err))
	}

	evtScanCompleted := library.LibraryScanCompleted{
		LibraryID:    libraryID,
		ItemsAdded:   added,
		ItemsUpdated: updated,
		ItemsRemoved: removed,
		OccurredAt:   now,
	}
	if publishErr := s.events.Publish(ctx, evtScanCompleted); publishErr != nil {
		slog.Warn("publish event", slog.String("event", evtScanCompleted.EventName()), slog.Any("err", publishErr))
	}
	return added, updated, nil
}

func (s *ScanService) removeOrphanedItems(ctx context.Context, libraryID string, seenPaths map[string]struct{}) (int, error) {
	dbPaths, err := s.mediaItems.ListFilePaths(ctx, libraryID)
	if err != nil {
		return 0, fmt.Errorf("list file paths: %w", err)
	}
	removed := 0
	for id, path := range dbPaths {
		if _, seen := seenPaths[path]; seen {
			continue
		}
		if delErr := s.mediaItems.Delete(ctx, id); delErr != nil {
			slog.Warn("delete orphaned media item failed", slog.String("id", id), slog.String("path", path), slog.Any("err", delErr))
			continue
		}
		if s.search != nil {
			if idxErr := s.search.RemoveMediaItem(ctx, id); idxErr != nil {
				slog.Warn("remove orphan from search index failed", slog.String("id", id), slog.Any("err", idxErr))
			}
		}
		evt := library.MediaItemRemoved{
			MediaItemID: id,
			LibraryID:   libraryID,
			OccurredAt:  time.Now(),
		}
		if pubErr := s.events.Publish(ctx, evt); pubErr != nil {
			slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", pubErr))
		}
		removed++
	}

	// Prune empty seasons (no child episodes) and empty series (no child seasons)
	pruned, pruneErr := s.pruneEmptyContainers(ctx, libraryID)
	if pruneErr != nil {
		slog.Warn("prune empty containers failed", slog.Any("err", pruneErr))
	}
	removed += pruned

	return removed, nil
}

func (svc *ScanService) pruneEmptyContainers(ctx context.Context, libraryID string) (int, error) {
	all, err := svc.mediaItems.ListByLibraryID(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	childCount := make(map[string]int)
	for _, item := range all {
		if item.ParentID != "" {
			childCount[item.ParentID]++
		}
	}
	pruned := 0
	// First pass: delete empty seasons (no child episodes)
	for _, item := range all {
		if item.Type != string(library.Season) {
			continue
		}
		if childCount[item.ID] > 0 {
			continue
		}
		if dErr := svc.mediaItems.Delete(ctx, item.ID); dErr != nil {
			slog.Warn("delete empty season", slog.String("id", item.ID), slog.Any("err", dErr))
			continue
		}
		childCount[item.ParentID]--
		pruned++
	}
	// Second pass: delete empty series (seasons may have been removed above)
	for _, item := range all {
		if item.Type != string(library.Series) {
			continue
		}
		if childCount[item.ID] > 0 {
			continue
		}
		if dErr := svc.mediaItems.Delete(ctx, item.ID); dErr != nil {
			slog.Warn("delete empty series", slog.String("id", item.ID), slog.Any("err", dErr))
			continue
		}
		if svc.search != nil {
			_ = svc.search.RemoveMediaItem(ctx, item.ID)
		}
		pruned++
	}
	return pruned, nil
}

func (s *ScanService) upsertMediaFile(ctx context.Context, libraryID, parentID string, f library.ScannedFile, seriesTitle ...string) (added, updated int, err error) {
	var existing *library.MediaItem
	var getErr error
	if f.Fingerprint != "" {
		existing, getErr = s.mediaItems.GetByFileFingerprint(ctx, f.Fingerprint)
		if errors.Is(getErr, shared.ErrNotFound) {
			existing, getErr = s.mediaItems.GetByFilePath(ctx, f.Path)
		}
	} else {
		existing, getErr = s.mediaItems.GetByFilePath(ctx, f.Path)
	}
	if getErr != nil && !errors.Is(getErr, shared.ErrNotFound) {
		return 0, 0, fmt.Errorf("check media item %s: %w", f.Path, getErr)
	}
	if errors.Is(getErr, shared.ErrNotFound) {
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
			if err := s.mediaItems.Update(ctx, item); err != nil {
				slog.Warn("update media item duration failed", slog.String("mediaItemID", item.ID), slog.Any("err", err))
			}
		}
		if s.metadata != nil {
			s.metadata.extractAndStoreChapters(ctx, item.ID, f.Path)
		}
		if s.search != nil {
			if err := s.search.IndexMediaItem(ctx, item, nil); err != nil {
				slog.Warn("index media item failed", slog.String("mediaItemID", item.ID), slog.Any("err", err))
			}
		}
		showTitle := ""
		if len(seriesTitle) > 0 {
			showTitle = seriesTitle[0]
		}
		evtAdded := library.MediaItemAdded{
			MediaItemID: item.ID,
			LibraryID:   libraryID,
			Title:       item.Title,
			SeriesTitle: showTitle,
			Type:        item.Type,
			Genres:      item.Genres,
			OccurredAt:  time.Now(),
		}
		if err := s.events.Publish(ctx, evtAdded); err != nil {
			slog.Warn("publish event", slog.String("event", evtAdded.EventName()), slog.Any("err", err))
		}
		return 1, 0, nil
	}
	unchanged := existing.FileSize == f.Size && (f.Fingerprint == "" || existing.FileFingerprint == f.Fingerprint)
	if unchanged {
		if existing.FilePath != f.Path || existing.ParentID != parentID {
			existing.FilePath = f.Path
			existing.ParentID = parentID
			existing.UpdatedAt = time.Now()
			if err := s.mediaItems.Update(ctx, existing); err != nil {
				slog.Warn("update media item failed", slog.String("mediaItemID", existing.ID), slog.Any("err", err))
			}
		}
		return 0, 1, nil
	}
	existing.ParentID = parentID
	existing.FilePath = f.Path
	existing.FileSize = f.Size
	existing.FileFingerprint = f.Fingerprint
	existing.UpdatedAt = time.Now()
	if dur := s.ProbeAndStoreStreams(ctx, existing.ID, f.Path); dur > 0 {
		existing.DurationMs = dur
	}
	if s.metadata != nil {
		s.metadata.extractAndStoreChapters(ctx, existing.ID, f.Path)
	}
	if err := s.mediaItems.Update(ctx, existing); err != nil {
		slog.Warn("update media item failed", slog.String("mediaItemID", existing.ID), slog.Any("err", err))
	}
	if s.search != nil {
		if err := s.search.IndexMediaItem(ctx, existing, nil); err != nil {
			slog.Warn("index media item failed", slog.String("mediaItemID", existing.ID), slog.Any("err", err))
		}
	}
	if existing.Overview == "" {
		showTitle := ""
		if len(seriesTitle) > 0 {
			showTitle = seriesTitle[0]
		}
		evtReAdded := library.MediaItemAdded{
			MediaItemID: existing.ID,
			LibraryID:   existing.LibraryID,
			Title:       existing.Title,
			SeriesTitle: showTitle,
			Type:        existing.Type,
			Genres:      existing.Genres,
			OccurredAt:  time.Now(),
		}
		if err := s.events.Publish(ctx, evtReAdded); err != nil {
			slog.Warn("publish event", slog.String("event", evtReAdded.EventName()), slog.Any("err", err))
		}
	}
	return 0, 1, nil
}

func (s *ScanService) ProbeAndStoreStreams(ctx context.Context, mediaItemID, filePath string) int64 {
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

	var extSubs []*library.MediaStream
	if s.metadata != nil {
		extSubs = s.metadata.discoverExternalSubtitles(result.Streams, mediaItemID, filePath)
	} else if s.subScanner != nil {
		m := &MetadataExtractionService{subScanner: s.subScanner}
		extSubs = m.discoverExternalSubtitles(result.Streams, mediaItemID, filePath)
	}
	result.Streams = append(result.Streams, extSubs...)
	allStreams := result.Streams

	if err := s.mediaStreams.DeleteByMediaItemID(ctx, mediaItemID); err != nil {
		slog.Warn("delete media streams failed", slog.String("mediaItemID", mediaItemID), slog.Any("err", err))
	}
	if err := s.mediaStreams.CreateBatch(ctx, allStreams); err != nil {
		slog.Warn("create media streams failed", slog.String("mediaItemID", mediaItemID), slog.Any("err", err))
	}
	return result.DurationMs
}

func (s *ScanService) getOrCreateItem(ctx context.Context, item *library.MediaItem) (*library.MediaItem, bool, error) {
	existing, err := s.mediaItems.GetByFileFingerprint(ctx, item.FileFingerprint)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, false, err
	}
	existing, err = s.mediaItems.GetByLibraryTypeTitle(ctx, item.LibraryID, item.Type, item.Title, item.ParentID)
	if err == nil {
		existing.FileFingerprint = item.FileFingerprint
		if updateErr := s.mediaItems.Update(ctx, existing); updateErr != nil {
			slog.Warn("update media item fingerprint failed", slog.String("mediaItemID", existing.ID), slog.Any("err", updateErr))
		}
		return existing, false, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, false, fmt.Errorf("lookup by title %q: %w", item.Title, err)
	}
	if err = s.mediaItems.Create(ctx, item); err != nil {
		return nil, false, err
	}
	return item, true, nil
}

func (s *ScanService) scanTVShowFiles(ctx context.Context, libraryID string, files []library.ScannedFile) (added, updated int, err error) {
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
				evtSeriesAdded := library.MediaItemAdded{
					MediaItemID: seriesItem.ID,
					LibraryID:   libraryID,
					Title:       seriesItem.Title,
					Type:        seriesItem.Type,
					Genres:      seriesItem.Genres,
					OccurredAt:  time.Now(),
				}
				if err := s.events.Publish(ctx, evtSeriesAdded); err != nil {
					slog.Warn("publish event", slog.String("event", evtSeriesAdded.EventName()), slog.Any("err", err))
				}
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
			se := 0
			if f.Season != nil {
				se = *f.Season
			}
			episodeFile.Title = fmt.Sprintf("S%02dE%02d", se, *f.Episode)
		}
		a, u, episodeErr := s.upsertMediaFile(ctx, libraryID, seasonID, episodeFile, showTitle)
		if episodeErr != nil {
			return 0, 0, episodeErr
		}
		added += a
		updated += u
	}
	return added, updated, nil
}
