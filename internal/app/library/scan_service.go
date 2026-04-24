package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/scanindex"
)

// ErrScanInProgress is returned by ScanLibrary when a scan is already running
// for the given library. Callers (gRPC handler, HTTP handler) should map this
// to a user-friendly 409/FailedPrecondition response.
var ErrScanInProgress = errors.New("scan already in progress")

func catalogGenres(item *library.MediaItem) []string {
	if item == nil || item.Catalog == nil {
		return nil
	}
	return item.Catalog.Genres
}

type TypedDirectoryScanner interface {
	ScanDirectory(ctx context.Context, libraryID, dir string) (int, error)
}

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

	metadata     *MetadataExtractionService
	musicScanner TypedDirectoryScanner
	photoScanner TypedDirectoryScanner
	pruner       *OrphanPruner

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
		pruner:       NewOrphanPruner(mediaItems, search, events),
		scanning:     make(map[string]bool),
	}
}

func (s *ScanService) WithMetadataExtraction(m *MetadataExtractionService) {
	s.metadata = m
}

func (s *ScanService) WithSubtitleScanner(sc library.ExternalSubtitleScanner) {
	s.subScanner = sc
}

func (s *ScanService) WithMusicScanner(sc TypedDirectoryScanner) {
	s.musicScanner = sc
}

func (s *ScanService) WithPhotoScanner(sc TypedDirectoryScanner) {
	s.photoScanner = sc
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
		return 0, 0, fmt.Errorf("%w for library %s", ErrScanInProgress, libraryID)
	}
	s.scanning[libraryID] = true
	defer func() {
		s.scanMu.Lock()
		delete(s.scanning, libraryID)
		s.scanMu.Unlock()
	}()
	s.scanMu.Unlock()

	lib, err := s.libQueries.GetByID(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return 0, 0, fmt.Errorf("get library: %w", err)
	}

	s.publishScanStarted(ctx, libraryID)

	switch string(lib.Type) {
	case "music":
		if s.musicScanner == nil {
			return 0, 0, fmt.Errorf("music scanner not configured")
		}
		var total int
		for _, p := range lib.Paths {
			n, e := s.musicScanner.ScanDirectory(ctx, libraryID, p)
			if e != nil {
				slog.Warn("music scan path failed", slog.String("path", p), slog.Any("err", e))
				continue
			}
			total += n
		}
		s.finalizeScan(ctx, lib, libraryID, total, 0, 0)
		return total, 0, nil
	case "photo":
		if s.photoScanner == nil {
			return 0, 0, fmt.Errorf("photo scanner not configured")
		}
		var total int
		for _, p := range lib.Paths {
			n, e := s.photoScanner.ScanDirectory(ctx, libraryID, p)
			if e != nil {
				slog.Warn("photo scan path failed", slog.String("path", p), slog.Any("err", e))
				continue
			}
			total += n
		}
		s.finalizeScan(ctx, lib, libraryID, total, 0, 0)
		return total, 0, nil
	}

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

	metrics.ScanQueueDepth.WithLabelValues(libraryID).Set(float64(len(toProcess)))
	a, u := s.processNewItems(ctx, libraryID, toProcess)
	metrics.ScanQueueDepth.WithLabelValues(libraryID).Set(0)
	added += a
	updated += u

	removed, pruneErr := s.pruner.Prune(ctx, libraryID, seenPaths)
	if pruneErr != nil {
		slog.Warn("orphan cleanup failed", slog.String("libraryID", libraryID), slog.Any("err", pruneErr))
	}

	s.finalizeScan(ctx, lib, libraryID, added, updated, removed)
	return added, updated, nil
}

func (s *ScanService) publishScanStarted(ctx context.Context, libraryID string) {
	evt := library.LibraryScanStarted{
		LibraryID:  shared.LibraryID(libraryID),
		OccurredAt: time.Now(),
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}

func (s *ScanService) loadScanIndex(ctx context.Context, libraryID string) (*scanindex.Index, error) {
	summaries, err := s.mediaItems.ListScanSummaries(ctx, shared.LibraryID(libraryID))
	if err != nil {
		return nil, fmt.Errorf("list existing items: %w", err)
	}
	entries := make([]scanindex.Summary, len(summaries))
	for i, row := range summaries {
		entries[i] = scanindex.Summary{
			ID:      row.ID,
			Path:    row.FilePath,
			Size:    row.FileSize,
			ModTime: row.ModTime,
			Hash:    row.FileFingerprint,
		}
	}
	return scanindex.New(entries), nil
}

func (s *ScanService) scanDirectories(ctx context.Context, lib *library.Library, idx *scanindex.Index) (seenPaths map[string]struct{}, toProcess []scanWork, added, updated int, err error) {
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
			a, u, e := s.scanTVShowFiles(ctx, string(lib.ID), files)
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

func (s *ScanService) detectChanges(ctx context.Context, files []library.ScannedFile, idx *scanindex.Index) (toProcess []scanWork, moved int) {
	for _, f := range files {
		if _, unchanged := idx.Unchanged(f.Path, f.Size, f.ModTime); unchanged {
			// Extra fingerprint gate when both sides have one: catches the
			// rare inode/mtime coincidence where the OS reused both for
			// different content.
			exHash, _ := idx.ExistingHash(f.Path)
			if exHash == "" || f.Fingerprint == "" || exHash == f.Fingerprint {
				continue
			}
		}

		if movedID, isMove := idx.Moved(f.Path, f.Fingerprint); isMove {
			fullItem, getErr := s.mediaItems.GetByID(ctx, shared.MediaItemID(movedID))
			if getErr != nil {
				slog.Warn("get moved item", slog.String("id", movedID), slog.Any("err", getErr))
			} else {
				if fullItem.File == nil {
					fullItem.File = &library.MediaFile{}
				}
				fullItem.File.Path = f.Path
				fullItem.File.ModTime = f.ModTime
				fullItem.UpdatedAt = time.Now()
				if updateErr := s.mediaItems.Update(ctx, fullItem); updateErr != nil {
					slog.Warn("update moved item path", slog.String("id", movedID), slog.Any("err", updateErr))
				}
			}
			moved++
			continue
		}

		toProcess = append(toProcess, scanWork{file: f})
	}
	return toProcess, moved
}

func (s *ScanService) processNewItems(ctx context.Context, libraryID string, toProcess []scanWork) (added, updated int) {
	// 5min accommodates ffprobe + extractChapters + meili index on 4K 40GB+
	// files; 30s was sufficient for 1080p but timed out on UHD content,
	// starving the final Update of its budget.
	const workerTimeout = 5 * time.Minute

	var addedAtomic, updatedAtomic atomic.Int64
	sem := make(chan struct{}, scanWorkerCount())
	var wg sync.WaitGroup
	for _, w := range toProcess {
		wg.Add(1)
		sem <- struct{}{}
		go func(w scanWork) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic recovered in scan worker", "error", r, "stack", string(debug.Stack()))
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

func (s *ScanService) finalizeScan(ctx context.Context, lib *library.Library, libraryID string, added, updated, removed int) {
	now := time.Now()
	lib.LastScannedAt = &now
	if err := s.libraries.Update(ctx, lib); err != nil {
		slog.Warn("update library last scanned failed", slog.String("libraryID", libraryID), slog.Any("err", err))
	}

	evt := library.LibraryScanCompleted{
		LibraryID:    shared.LibraryID(libraryID),
		ItemsAdded:   added,
		ItemsUpdated: updated,
		ItemsRemoved: removed,
		OccurredAt:   now,
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.Warn("publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}

func (s *ScanService) upsertMediaFile(ctx context.Context, libraryID, parentID string, f library.ScannedFile, seriesTitle ...string) (added, updated int, err error) {
	showTitle := ""
	if len(seriesTitle) > 0 {
		showTitle = seriesTitle[0]
	}
	existing, err := s.findExistingMediaFile(ctx, f)
	if err != nil {
		return 0, 0, err
	}
	if existing == nil {
		if err := s.createMediaFile(ctx, libraryID, parentID, f, showTitle); err != nil {
			return 0, 0, err
		}
		return 1, 0, nil
	}
	s.updateMediaFile(ctx, existing, parentID, f, showTitle)
	return 0, 1, nil
}

// findExistingMediaFile locates the media item that already owns the
// scanned file. Prefers fingerprint match (stable across path moves),
// falls back to file path. Returns (nil, nil) when no match exists —
// caller creates a fresh item in that case.
func (s *ScanService) findExistingMediaFile(ctx context.Context, f library.ScannedFile) (*library.MediaItem, error) {
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
	if errors.Is(getErr, shared.ErrNotFound) {
		return nil, nil
	}
	if getErr != nil {
		return nil, fmt.Errorf("check media item %s: %w", f.Path, getErr)
	}
	return existing, nil
}

// createMediaFile materializes a new MediaItem + child enrichment
// (probe streams, chapters, meili index) and publishes MediaItemAdded.
// Called only when findExistingMediaFile returned nil.
func (s *ScanService) createMediaFile(ctx context.Context, libraryID, parentID string, f library.ScannedFile, showTitle string) error {
	item := &library.MediaItem{
		ID:          shared.MediaItemID(uuid.New().String()),
		LibraryID:   shared.LibraryID(libraryID),
		ParentID:    parentID,
		Title:       f.Title,
		ScanTitle:   f.Title,
		Type:        f.Type,
		IndexNumber: f.Episode,
		AddedAt:     time.Now(),
		UpdatedAt:   time.Now(),
		File: &library.MediaFile{
			Path:        f.Path,
			Size:        f.Size,
			Fingerprint: f.Fingerprint,
			ModTime:     f.ModTime,
		},
	}
	if f.Year != nil {
		item.Catalog = &library.MediaCatalog{Year: f.Year}
	}
	if err := s.mediaItems.Create(ctx, item); err != nil {
		return fmt.Errorf("create media item: %w", err)
	}
	if dur := s.ProbeAndStoreStreams(ctx, string(item.ID), f.Path); dur > 0 {
		item.File.DurationMs = dur
		if err := s.mediaItems.Update(ctx, item); err != nil {
			slog.Warn("update media item duration failed", slog.String("mediaItemID", string(item.ID)), slog.Any("err", err))
		}
	}
	if s.metadata != nil {
		s.metadata.extractAndStoreChapters(ctx, string(item.ID), f.Path)
	}
	if s.search != nil {
		if err := s.search.IndexMediaItem(ctx, item, nil); err != nil {
			slog.Warn("index media item failed", slog.String("mediaItemID", string(item.ID)), slog.Any("err", err))
		}
	}
	evtAdded := library.MediaItemAdded{
		MediaItemID: item.ID,
		LibraryID:   shared.LibraryID(libraryID),
		Title:       item.Title,
		SeriesTitle: showTitle,
		Type:        string(item.Type),
		Genres:      catalogGenres(item),
		OccurredAt:  time.Now(),
	}
	if err := s.events.Publish(ctx, evtAdded); err != nil {
		slog.Warn("publish event", slog.String("event", evtAdded.EventName()), slog.Any("err", err))
	}
	return nil
}

// updateMediaFile reconciles an existing MediaItem against the freshly
// scanned file. Handles two paths: the fast "file moved / reparented
// but content unchanged" loop, and the full "content replaced" rescan
// that re-probes streams + chapters and re-publishes MediaItemAdded so
// translation/artwork jobs re-enrich the item.
func (s *ScanService) updateMediaFile(ctx context.Context, existing *library.MediaItem, parentID string, f library.ScannedFile, showTitle string) {
	if existing.File == nil {
		existing.File = &library.MediaFile{}
	}
	unchanged := existing.File.Size == f.Size && (f.Fingerprint == "" || existing.File.Fingerprint == f.Fingerprint)
	if unchanged {
		// Backfill ModTime when it was never persisted (older rows created
		// before the scanner started setting File.ModTime). Without this,
		// SameMtime would keep returning false each scan for those rows
		// and we'd pointlessly re-enter updateMediaFile every tick.
		mtimeStale := !scanindex.SameMtime(existing.File.ModTime, f.ModTime)
		if existing.File.Path != f.Path || existing.ParentID != parentID || mtimeStale {
			existing.File.Path = f.Path
			existing.ParentID = parentID
			existing.File.ModTime = f.ModTime
			existing.UpdatedAt = time.Now()
			if err := s.mediaItems.Update(ctx, existing); err != nil {
				slog.Warn("update media item failed", slog.String("mediaItemID", string(existing.ID)), slog.Any("err", err))
			}
		}
		return
	}
	existing.ParentID = parentID
	existing.File.Path = f.Path
	existing.File.Size = f.Size
	existing.File.Fingerprint = f.Fingerprint
	existing.File.ModTime = f.ModTime
	existing.UpdatedAt = time.Now()
	// Persist the new fingerprint + mtime BEFORE the heavy probe/chapters
	// pipeline. Otherwise on a 4K 46GB file the probe + extractChapters can
	// eat the whole workerCtx budget (30s) and the Update that follows
	// fails with 'begin tx: context deadline exceeded' — the stale
	// fingerprint then survives, the next scan re-flags the file as
	// changed, and the loop pins CPU forever. Updating first means a
	// timeout only loses the duration_ms refresh (recoverable next scan)
	// instead of the change-detection cornerstone.
	if err := s.mediaItems.Update(ctx, existing); err != nil {
		slog.Warn("update media item failed", slog.String("mediaItemID", string(existing.ID)), slog.Any("err", err))
	}
	if dur := s.ProbeAndStoreStreams(ctx, string(existing.ID), f.Path); dur > 0 {
		existing.File.DurationMs = dur
		if err := s.mediaItems.Update(ctx, existing); err != nil {
			slog.Warn("update media item duration failed", slog.String("mediaItemID", string(existing.ID)), slog.Any("err", err))
		}
	}
	if s.metadata != nil {
		s.metadata.extractAndStoreChapters(ctx, string(existing.ID), f.Path)
	}
	if s.search != nil {
		if err := s.search.IndexMediaItem(ctx, existing, nil); err != nil {
			slog.Warn("index media item failed", slog.String("mediaItemID", string(existing.ID)), slog.Any("err", err))
		}
	}
	if existing.Overview == "" {
		evtReAdded := library.MediaItemAdded{
			MediaItemID: existing.ID,
			LibraryID:   existing.LibraryID,
			Title:       existing.Title,
			SeriesTitle: showTitle,
			Type:        string(existing.Type),
			Genres:      catalogGenres(existing),
			OccurredAt:  time.Now(),
		}
		if err := s.events.Publish(ctx, evtReAdded); err != nil {
			slog.Warn("publish event", slog.String("event", evtReAdded.EventName()), slog.Any("err", err))
		}
	}
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
		st.MediaItemID = shared.MediaItemID(mediaItemID)
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

	if err := s.mediaStreams.ReplaceBatch(ctx, shared.MediaItemID(mediaItemID), allStreams); err != nil {
		slog.Warn("replace media streams failed", slog.String("mediaItemID", mediaItemID), slog.Any("err", err))
	}
	return result.DurationMs
}

// scanWorkerCount returns the scan goroutine pool size.
// Override via EVEY_LIBRARY_SCAN_WORKERS (int > 0). Defaults to runtime.NumCPU().
func scanWorkerCount() int {
	if v := os.Getenv("EVEY_LIBRARY_SCAN_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return runtime.NumCPU()
}

