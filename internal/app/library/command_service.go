package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
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
	subScanner   library.ExternalSubtitleScanner
	search       library.SearchIndex
	events       shared.EventBus
	trickplayRepo     library.TrickplayRepository
	trickplayGen      library.TrickplayGenerator
	trickplayCacheDir string
	shutdownCtx  context.Context

	scanMu   sync.Mutex
	scanning map[string]bool
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
		scanning:     make(map[string]bool),
	}
}

func (s *CommandService) WithChapters(chapters library.ChapterRepository, reader mkv.ContainerReader) {
	s.chapters = chapters
	s.mkvReader = reader
}

func (s *CommandService) WithSubtitleScanner(sc library.ExternalSubtitleScanner) {
	s.subScanner = sc
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
	if err := s.chapters.ReplaceBatch(ctx, mediaItemID, chapters); err != nil {
		slog.Warn("store chapters failed", slog.String("mediaItemID", mediaItemID), slog.Any("err", err))
	}
}

func (s *CommandService) CreateLibrary(ctx context.Context, lib *library.Library) error {
	now := time.Now()
	lib.CreatedAt = now
	lib.UpdatedAt = now
	if err := s.libraries.Create(ctx, lib); err != nil {
		return fmt.Errorf("create library: %w", err)
	}
	evtCreated := library.LibraryCreated{
		LibraryID:  lib.ID,
		Name:       lib.Name,
		Type:       lib.Type,
		OccurredAt: now,
	}
	if err := s.events.Publish(ctx, evtCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtCreated.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) DeleteLibrary(ctx context.Context, id string) error {
	if err := s.libraries.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete library: %w", err)
	}
	evtDeleted := library.LibraryDeleted{
		LibraryID:  id,
		OccurredAt: time.Now(),
	}
	if err := s.events.Publish(ctx, evtDeleted); err != nil {
		slog.Warn("publish event", slog.String("event", evtDeleted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error) {
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
	if err := s.libraries.Update(ctx, lib); err != nil {
		slog.Warn("update library last scanned failed", slog.String("libraryID", libraryID), slog.Any("err", err))
	}

	evtScanCompleted := library.LibraryScanCompleted{
		LibraryID:    libraryID,
		ItemsAdded:   added,
		ItemsUpdated: updated,
		OccurredAt:   now,
	}
	if publishErr := s.events.Publish(ctx, evtScanCompleted); publishErr != nil {
		slog.Warn("publish event", slog.String("event", evtScanCompleted.EventName()), slog.Any("err", publishErr))
	}
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
			if err := s.mediaItems.Update(ctx, item); err != nil {
				slog.Warn("update media item duration failed", slog.String("mediaItemID", item.ID), slog.Any("err", err))
			}
		}
		s.extractAndStoreChapters(ctx, item.ID, f.Path)
		if err := s.search.IndexMediaItem(ctx, item, nil); err != nil {
			slog.Warn("index media item failed", slog.String("mediaItemID", item.ID), slog.Any("err", err))
		}
		evtAdded := library.MediaItemAdded{
			MediaItemID: item.ID,
			LibraryID:   libraryID,
			Title:       item.Title,
			Type:        item.Type,
			OccurredAt:  time.Now(),
		}
		if err := s.events.Publish(ctx, evtAdded); err != nil {
			slog.Warn("publish event", slog.String("event", evtAdded.EventName()), slog.Any("err", err))
		}
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
	if err := s.mediaItems.Update(ctx, existing); err != nil {
		slog.Warn("update media item failed", slog.String("mediaItemID", existing.ID), slog.Any("err", err))
	}
	if err := s.search.IndexMediaItem(ctx, existing, nil); err != nil {
		slog.Warn("index media item failed", slog.String("mediaItemID", existing.ID), slog.Any("err", err))
	}
	// Re-emit so enricher re-fetches metadata for items with no overview
	if existing.Overview == "" {
		evtReAdded := library.MediaItemAdded{
			MediaItemID: existing.ID,
			LibraryID:   existing.LibraryID,
			Title:       existing.Title,
			Type:        existing.Type,
			OccurredAt:  time.Now(),
		}
		if err := s.events.Publish(ctx, evtReAdded); err != nil {
			slog.Warn("publish event", slog.String("event", evtReAdded.EventName()), slog.Any("err", err))
		}
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

	extSubs := s.discoverExternalSubtitles(result.Streams, mediaItemID, filePath)
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

func (s *CommandService) discoverExternalSubtitles(existing []*library.MediaStream, mediaItemID, filePath string) []*library.MediaStream {
	if s.subScanner == nil {
		return nil
	}
	subs, err := s.subScanner.ScanForSubtitles(filePath)
	if err != nil || len(subs) == 0 {
		return nil
	}

	maxIdx := 0
	for _, st := range existing {
		if st.StreamIndex >= maxIdx {
			maxIdx = st.StreamIndex + 1
		}
	}

	var streams []*library.MediaStream
	for _, sub := range subs {
		path := sub.Path
		codec := sub.Format
		if codec == "vtt" {
			codec = "webvtt"
		}
		st := &library.MediaStream{
			ID:           uuid.New().String(),
			MediaItemID:  mediaItemID,
			StreamIndex:  maxIdx,
			StreamType:   string(library.Subtitle),
			Codec:        codec,
			Language:     sub.Language,
			IsForced:     sub.IsForced,
			IsExternal:   true,
			ExternalPath: &path,
		}
		streams = append(streams, st)
		maxIdx++
	}
	return streams
}

func (s *CommandService) GetMediaStreams(ctx context.Context, mediaItemID string) ([]*library.MediaStream, error) {
	return s.mediaStreams.GetByMediaItemID(ctx, mediaItemID)
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
		if updateErr := s.mediaItems.Update(ctx, existing); updateErr != nil {
			slog.Warn("update media item fingerprint failed", slog.String("mediaItemID", existing.ID), slog.Any("err", updateErr))
		}
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
				evtSeriesAdded := library.MediaItemAdded{
					MediaItemID: seriesItem.ID,
					LibraryID:   libraryID,
					Title:       seriesItem.Title,
					Type:        seriesItem.Type,
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

func (s *CommandService) WithTrickplay(repo library.TrickplayRepository, gen library.TrickplayGenerator, cacheDir string) {
	s.trickplayRepo = repo
	s.trickplayGen = gen
	s.trickplayCacheDir = cacheDir
}

func (s *CommandService) WithShutdownCtx(ctx context.Context) {
	s.shutdownCtx = ctx
}

func (s *CommandService) GenerateTrickplay(ctx context.Context, mediaItemID string) error {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return errors.New("trickplay not configured")
	}

	item, err := s.mediaItems.GetByID(ctx, mediaItemID)
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	if item.FilePath == "" {
		return errors.New("media item has no file path")
	}

	interval := 10
	width := 160
	tileWidth := 10
	tileHeight := 10

	outputDir := fmt.Sprintf("%s/%s", s.trickplayCacheDir, mediaItemID)

	result, err := s.trickplayGen.Generate(ctx, item.FilePath, outputDir, interval, width, tileWidth, tileHeight)
	if err != nil {
		return fmt.Errorf("generate trickplay: %w", err)
	}

	manifest := &library.TrickplayManifest{
		ID:          uuid.New().String(),
		MediaItemID: mediaItemID,
		Interval:    interval,
		Width:       width,
		Height:      0,
		TileWidth:   tileWidth,
		TileHeight:  tileHeight,
		ImageCount:  result.ImageCount,
		ImagePath:   result.ImageDir,
		CreatedAt:   time.Now(),
	}

	if err := s.trickplayRepo.Create(ctx, manifest); err != nil {
		return fmt.Errorf("store trickplay manifest: %w", err)
	}
	return nil
}

func (s *CommandService) GetTrickplayManifest(ctx context.Context, mediaItemID string) (*library.TrickplayManifest, error) {
	if s.trickplayRepo == nil {
		return nil, errors.New("trickplay not configured")
	}
	return s.trickplayRepo.GetByMediaItemID(ctx, mediaItemID)
}

func (s *CommandService) GenerateAllMissingTrickplays(ctx context.Context) (int, error) {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return 0, errors.New("trickplay not configured")
	}
	ids, err := s.trickplayRepo.ListMissingMediaItemIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list missing trickplays: %w", err)
	}
	bgCtx := s.shutdownCtx
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	go func() {
		for _, id := range ids {
			if bgCtx.Err() != nil {
				return
			}
			if err := s.GenerateTrickplay(bgCtx, id); err != nil {
				slog.Warn("trickplay generation failed", slog.String("media_id", id), slog.Any("err", err))
			} else {
				slog.Info("trickplay generated", slog.String("media_id", id))
			}
		}
	}()
	return len(ids), nil
}
