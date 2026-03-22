package library

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type CommandService struct {
	libraries   library.LibraryCommandRepo
	mediaItems  library.MediaItemCommandRepo
	mediaStreams library.MediaStreamRepository
	events      shared.EventBus

	scan      *ScanService
	trickplay *TrickplayService
	metadata  *MetadataExtractionService
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
	scanSvc := NewScanService(
		libraries, libQueries, mediaItems, mediaStreams,
		persons, mediaPersons, scanner, prober, search, events,
	)
	return &CommandService{
		libraries:   libraries,
		mediaItems:  mediaItems,
		mediaStreams: mediaStreams,
		events:      events,
		scan:        scanSvc,
	}
}

func (s *CommandService) WithChapters(chapters library.ChapterRepository, reader mkv.ContainerReader) {
	s.metadata = NewMetadataExtractionService(chapters, reader, s.mediaStreams)
	s.scan.WithMetadataExtraction(s.metadata)
}

func (s *CommandService) WithSubtitleScanner(sc library.ExternalSubtitleScanner) {
	s.scan.WithSubtitleScanner(sc)
	if s.metadata != nil {
		s.metadata.WithSubtitleScanner(sc)
	}
}

func (s *CommandService) WithTrickplay(repo library.TrickplayRepository, gen library.TrickplayGenerator, cacheDir string, configRepo shared.ServerConfigRepository) {
	s.trickplay = NewTrickplayService(repo, gen, cacheDir, s.mediaItems, s.events, configRepo)
}

// Validate panics if required With*() deps were not set.
// Must be called after all With*() calls (e.g. from wire.go).
func (s *CommandService) Validate() {
	if s.metadata == nil {
		panic("library.CommandService: WithChapters not called")
	}
	if s.trickplay == nil {
		panic("library.CommandService: WithTrickplay not called")
	}
}

func (s *CommandService) WithShutdownCtx(ctx context.Context) {
	if s.trickplay != nil {
		s.trickplay.WithShutdownCtx(ctx)
	}
}

// --- Delegated to ScanService ---

func (s *CommandService) ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error) {
	return s.scan.ScanLibrary(ctx, libraryID)
}

func (s *CommandService) ProbeAndStoreStreams(ctx context.Context, mediaItemID, filePath string) int64 {
	return s.scan.ProbeAndStoreStreams(ctx, mediaItemID, filePath)
}

// --- Delegated to MetadataExtractionService ---

func (s *CommandService) GetChapters(ctx context.Context, mediaItemID string) ([]*library.Chapter, error) {
	if s.metadata == nil {
		return nil, nil
	}
	return s.metadata.GetChapters(ctx, mediaItemID)
}

// StartPeriodicScan runs a lightweight scan at the given interval for specific library IDs
// where inotify is unavailable. If libraryIDs is empty, scans all libraries.
func (s *CommandService) StartPeriodicScan(ctx context.Context, libQueries library.LibraryQueryRepo, interval time.Duration, libraryIDs []string) {
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	if len(libraryIDs) > 0 {
		slog.Info("periodic scan started (fallback for inotify-failed libraries)", slog.Duration("interval", interval), slog.Int("libraries", len(libraryIDs)))
	} else {
		slog.Info("periodic scan started (all libraries)", slog.Duration("interval", interval))
	}
	targetSet := make(map[string]bool, len(libraryIDs))
	for _, id := range libraryIDs {
		targetSet[id] = true
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				libs, err := libQueries.List(ctx)
				if err != nil {
					slog.Warn("periodic scan: list libraries", slog.Any("err", err))
					continue
				}
				for _, lib := range libs {
					if len(targetSet) > 0 && !targetSet[lib.ID] {
						continue
					}
					added, scanErr := func() (a int, err error) {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("periodic scan: panic recovered", slog.String("library", lib.Name), slog.Any("panic", r))
								err = fmt.Errorf("panic: %v", r)
							}
						}()
						a, _, err = s.ScanLibrary(ctx, lib.ID)
						return
					}()
					if scanErr != nil {
						slog.Debug("periodic scan: skip", slog.String("library", lib.Name), slog.Any("err", scanErr))
						continue
					}
					if added > 0 {
						slog.Info("periodic scan", slog.String("library", lib.Name), slog.Int("new", added))
					}
				}
			}
		}
	}()
}

// --- Delegated to TrickplayService ---

func (s *CommandService) GenerateTrickplay(ctx context.Context, mediaItemID string) error {
	if s.trickplay == nil {
		return fmt.Errorf("trickplay not configured")
	}
	return s.trickplay.GenerateTrickplay(ctx, mediaItemID)
}

func (s *CommandService) GetTrickplayManifest(ctx context.Context, mediaItemID string) (*library.TrickplayManifest, error) {
	if s.trickplay == nil {
		return nil, fmt.Errorf("trickplay not configured")
	}
	return s.trickplay.GetTrickplayManifest(ctx, mediaItemID)
}

func (s *CommandService) GenerateAllMissingTrickplays(ctx context.Context) (int, error) {
	if s.trickplay == nil {
		return 0, fmt.Errorf("trickplay not configured")
	}
	return s.trickplay.GenerateAllMissingTrickplays(ctx)
}

func (s *CommandService) StartTrickplayWorker(ctx context.Context) {
	if s.trickplay == nil {
		return
	}
	s.trickplay.StartTrickplayWorker(ctx)
}

func (s *CommandService) SetTrickplayActiveStreamChecker(c ActiveStreamChecker) {
	if s.trickplay != nil {
		s.trickplay.WithActiveStreamChecker(c)
	}
}

func (s *CommandService) StartBackgroundGeneration(ctx context.Context) {
	if s.trickplay == nil {
		return
	}
	s.trickplay.StartBackgroundGeneration(ctx)
}

// --- Direct methods (library CRUD) ---

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

func (s *CommandService) DeleteMediaItem(ctx context.Context, id string) error {
	return s.mediaItems.Delete(ctx, id)
}

func (s *CommandService) GetMediaStreams(ctx context.Context, mediaItemID string) ([]*library.MediaStream, error) {
	return s.mediaStreams.GetByMediaItemID(ctx, mediaItemID)
}
