package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/domain/shared"
)

type CommandServiceOption func(*CommandService)

func WithShutdownCtx(ctx context.Context) CommandServiceOption {
	return func(s *CommandService) {
		if s.trickplay != nil {
			s.trickplay.WithShutdownCtx(ctx)
		}
	}
}

type CommandService struct {
	libraries  library.LibraryCommandRepo
	mediaItems library.MediaItemCommandRepo
	events     shared.EventBus

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
	chapters library.ChapterRepository,
	ebmlReader mkv.ContainerReader,
	trickplayRepo library.TrickplayRepository,
	trickplayGen library.TrickplayGenerator,
	trickplayCacheDir string,
	trickplayConfigRepo shared.ServerConfigRepository,
	opts ...CommandServiceOption,
) *CommandService {
	scanSvc := NewScanService(
		libraries, libQueries, mediaItems, mediaStreams,
		persons, mediaPersons, scanner, prober, search, events,
	)
	metadata := NewMetadataExtractionService(chapters, ebmlReader, mediaStreams)
	scanSvc.WithMetadataExtraction(metadata)
	s := &CommandService{
		libraries:  libraries,
		mediaItems: mediaItems,
		events:     events,
		scan:       scanSvc,
		metadata:   metadata,
		trickplay:  NewTrickplayService(trickplayRepo, trickplayGen, trickplayCacheDir, mediaItems, events, trickplayConfigRepo),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *CommandService) WithSubtitleScanner(sc library.ExternalSubtitleScanner) {
	s.scan.WithSubtitleScanner(sc)
	if s.metadata != nil {
		s.metadata.WithSubtitleScanner(sc)
	}
}

func (s *CommandService) WithMusicScanner(sc TypedDirectoryScanner) {
	s.scan.WithMusicScanner(sc)
}

func (s *CommandService) WithPhotoScanner(sc TypedDirectoryScanner) {
	s.scan.WithPhotoScanner(sc)
}

// --- Delegated to ScanService ---

func (s *CommandService) ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error) {
	return s.scan.ScanLibrary(ctx, libraryID)
}

func (s *CommandService) ProbeAndStoreStreams(ctx context.Context, mediaItemID, filePath string) shared.Duration {
	return s.scan.ProbeAndStoreStreams(ctx, mediaItemID, filePath)
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
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in periodic scan", "error", r, "stack", string(debug.Stack()))
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				libs, err := libQueries.List(ctx, library.ListLibrariesFilter{})
				if err != nil {
					slog.Warn("periodic scan: list libraries", slog.Any("err", err))
					continue
				}
				for _, lib := range libs {
					if len(targetSet) > 0 && !targetSet[string(lib.ID)] {
						continue
					}
					added, scanErr := func() (a int, err error) {
						defer func() {
							if r := recover(); r != nil {
								slog.Error("periodic scan: panic recovered", slog.String("library", lib.Name), slog.Any("panic", r))
								err = fmt.Errorf("panic: %v", r)
							}
						}()
						a, _, err = s.ScanLibrary(ctx, string(lib.ID))
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
		return errors.New("trickplay not configured")
	}
	return s.trickplay.GenerateTrickplay(ctx, mediaItemID)
}

func (s *CommandService) GenerateAllMissingTrickplays(ctx context.Context) (int, error) {
	if s.trickplay == nil {
		return 0, errors.New("trickplay not configured")
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
	if err := lib.Validate(); err != nil {
		return fmt.Errorf("create library: %w", err)
	}
	now := time.Now()
	lib.CreatedAt = now
	lib.UpdatedAt = now
	if err := s.libraries.Create(ctx, lib); err != nil {
		return fmt.Errorf("create library: %w", err)
	}
	evtCreated := library.LibraryCreated{
		LibraryID:  lib.ID,
		Name:       lib.Name,
		Type:       string(lib.Type),
		OccurredAt: now,
	}
	if err := s.events.Publish(ctx, evtCreated); err != nil {
		slog.Warn("publish event", slog.String("event", evtCreated.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) UpdateLibrary(ctx context.Context, lib *library.Library) error {
	lib.UpdatedAt = time.Now()
	if err := s.libraries.Update(ctx, lib); err != nil {
		return fmt.Errorf("update library: %w", err)
	}
	return nil
}

func (s *CommandService) DeleteLibrary(ctx context.Context, id string) error {
	if err := s.libraries.Delete(ctx, shared.LibraryID(id)); err != nil {
		return fmt.Errorf("delete library: %w", err)
	}
	evtDeleted := library.LibraryDeleted{
		LibraryID:  shared.LibraryID(id),
		OccurredAt: time.Now(),
	}
	if err := s.events.Publish(ctx, evtDeleted); err != nil {
		slog.Warn("publish event", slog.String("event", evtDeleted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) DeleteMediaItem(ctx context.Context, id string) error {
	if err := s.mediaItems.Delete(ctx, shared.MediaItemID(id)); err != nil {
		return fmt.Errorf("CommandService.DeleteMediaItem: %w", err)
	}
	return nil
}
