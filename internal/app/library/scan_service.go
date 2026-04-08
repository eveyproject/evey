package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"runtime/debug"
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

type scanRef struct {
	ID              string
	FileSize        int64
	ModTime         time.Time
	FileFingerprint string
	FilePath        string
}

type scanIndex struct {
	byPath        map[string]*scanRef
	byFingerprint map[string]*scanRef
}

type scanWork struct {
	file      library.ScannedFile
	parentID  string
	showTitle string
}

func (s *ScanService) ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error) {
	s.scanMu.Lock()
	if s.scanning[libraryID] {
		s.scanMu.Unlock()
		return 0, 0, fmt.Errorf("scan already in progress for library %s", libraryID)
	}
	s.scanning[libraryID] = true
	defer func() {
		s.scanMu.Lock()
		delete(s.scanning, libraryID)
		s.scanMu.Unlock()
	}()
	s.scanMu.Unlock()

	lib, err := s.libQueries.GetByID(ctx, libraryID)
	if err != nil {
		return 0, 0, fmt.Errorf("get library: %w", err)
	}

	s.publishScanStarted(ctx, libraryID)

	idx, err := s.loadScanIndex(ctx, libraryID)
	if err != nil {
		return 0, 0, fmt.Errorf("ScanService.ScanLibrary: %w", err)
	}

	seenPaths, toProcess, tvAdded, tvUpdated, err := s.scanDirectories(ctx, lib, idx)
	if err != nil {
		return 0, 0, fmt.Errorf("ScanService.ScanLibrary: %w", err)
	}
	added += tvAdded
	updated += tvUpdated

	a, u := s.processNewItems(ctx, libraryID, toProcess)
	added += a
	updated += u

	removed := s.processDeletedItems(ctx, libraryID, seenPaths)

	s.finalizeScan(ctx, lib, libraryID, added, updated, removed)
	return added, updated, nil
}

func (s *ScanService) publishScanStarted(ctx context.Context, libraryID string) {
	evt := library.LibraryScanStarted{
		LibraryID:  libraryID,
		OccurredAt: time.Now(),
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}

func (s *ScanService) loadScanIndex(ctx context.Context, libraryID string) (*scanIndex, error) {
	summaries, err := s.mediaItems.ListScanSummaries(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list existing items: %w", err)
	}
	idx := &scanIndex{
		byPath:        make(map[string]*scanRef, len(summaries)),
		byFingerprint: make(map[string]*scanRef, len(summaries)),
	}
	for i := range summaries {
		ref := &scanRef{
			ID:              summaries[i].ID,
			FileSize:        summaries[i].FileSize,
			ModTime:         summaries[i].ModTime,
			FileFingerprint: summaries[i].FileFingerprint,
			FilePath:        summaries[i].FilePath,
		}
		idx.byPath[ref.FilePath] = ref
		if ref.FileFingerprint != "" {
			idx.byFingerprint[ref.FileFingerprint] = ref
		}
	}
	return idx, nil
}

func (s *ScanService) scanDirectories(ctx context.Context, lib *library.Library, idx *scanIndex) (seenPaths map[string]struct{}, toProcess []scanWork, added, updated int, err error) {
	seenPaths = make(map[string]struct{})

	for _, path := range lib.Paths {
		files, scanErr := s.scanner.Scan(ctx, path)
		if scanErr != nil {
			return nil, nil, 0, 0, fmt.Errorf("scan path %s: %w", path, scanErr)
		}

		if lib.Type == "tvshow" {
			for _, f := range files {
				seenPaths[f.Path] = struct{}{}
			}
			a, u, e := s.scanTVShowFiles(ctx, lib.ID, files)
			if e != nil {
				return nil, nil, 0, 0, e
			}
			added += a
			updated += u
			continue
		}

		work, u := s.detectChanges(ctx, files, idx)
		for _, f := range files {
			seenPaths[f.Path] = struct{}{}
		}
		toProcess = append(toProcess, work...)
		updated += u
	}
	return seenPaths, toProcess, added, updated, nil
}

func (s *ScanService) detectChanges(ctx context.Context, files []library.ScannedFile, idx *scanIndex) (toProcess []scanWork, moved int) {
	for _, f := range files {
		if ex := idx.byPath[f.Path]; ex != nil {
			if ex.FileSize == f.Size && ex.ModTime.Equal(f.ModTime) && ex.FileFingerprint == f.Fingerprint {
				continue
			}
		}

		if idx.byPath[f.Path] == nil && f.Fingerprint != "" {
			if ref := idx.byFingerprint[f.Fingerprint]; ref != nil {
				fullItem, getErr := s.mediaItems.GetByID(ctx, ref.ID)
				if getErr != nil {
					slog.Warn("get moved item", slog.String("id", ref.ID), slog.Any("err", getErr))
				} else {
					fullItem.FilePath = f.Path
					fullItem.ModTime = f.ModTime
					fullItem.UpdatedAt = time.Now()
					if updateErr := s.mediaItems.Update(ctx, fullItem); updateErr != nil {
						slog.Warn("update moved item path", slog.String("id", ref.ID), slog.Any("err", updateErr))
					}
				}
				moved++
				continue
			}
		}

		toProcess = append(toProcess, scanWork{file: f})
	}
	return toProcess, moved
}

func (s *ScanService) processNewItems(ctx context.Context, libraryID string, toProcess []scanWork) (added, updated int) {
	const workerTimeout = 30 * time.Second

	var addedAtomic, updatedAtomic atomic.Int64
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for _, w := range toProcess {
		wg.Add(1)
		sem <- struct{}{}
		go func(w scanWork) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in scan worker", "error", r, "stack", debug.Stack())
				}
			}()
			workerCtx, cancel := context.WithTimeout(ctx, workerTimeout)
			defer cancel()
			a, u, e := s.upsertMediaFile(workerCtx, libraryID, w.parentID, w.file, w.showTitle)
			if e != nil {
				slog.Warn("upsert media file", slog.String("path", w.file.Path), slog.Any("err", e))
				return
			}
			addedAtomic.Add(int64(a))
			updatedAtomic.Add(int64(u))
		}(w)
	}
	wg.Wait()
	return int(addedAtomic.Load()), int(updatedAtomic.Load())
}

func (s *ScanService) processDeletedItems(ctx context.Context, libraryID string, seenPaths map[string]struct{}) int {
	removed, removeErr := s.removeOrphanedItems(ctx, libraryID, seenPaths)
	if removeErr != nil {
		slog.Warn("orphan cleanup failed", slog.String("libraryID", libraryID), slog.Any("err", removeErr))
	}
	return removed
}

func (s *ScanService) finalizeScan(ctx context.Context, lib *library.Library, libraryID string, added, updated, removed int) {
	now := time.Now()
	lib.LastScannedAt = &now
	if err := s.libraries.Update(ctx, lib); err != nil {
		slog.Warn("update library last scanned failed", slog.String("libraryID", libraryID), slog.Any("err", err))
	}

	evt := library.LibraryScanCompleted{
		LibraryID:    libraryID,
		ItemsAdded:   added,
		ItemsUpdated: updated,
		ItemsRemoved: removed,
		OccurredAt:   now,
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}

func (s *ScanService) removeOrphanedItems(ctx context.Context, libraryID string, seenPaths map[string]struct{}) (int, error) {
	dbPaths, err := s.mediaItems.ListFilePaths(ctx, libraryID)
	if err != nil {
		return 0, fmt.Errorf("list file paths: %w", err)
	}
	var orphanIDs []string
	for id, path := range dbPaths {
		if _, seen := seenPaths[path]; !seen {
			orphanIDs = append(orphanIDs, id)
		}
	}
	if len(orphanIDs) > 0 {
		if delErr := s.mediaItems.DeleteByIDs(ctx, orphanIDs); delErr != nil {
			slog.Warn("batch delete orphaned media items failed", slog.Any("err", delErr))
		}
		for _, id := range orphanIDs {
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
		}
	}

	pruned, pruneErr := s.pruneEmptyContainers(ctx, libraryID)
	if pruneErr != nil {
		slog.Warn("prune empty containers failed", slog.Any("err", pruneErr))
	}

	return len(orphanIDs) + pruned, nil
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

