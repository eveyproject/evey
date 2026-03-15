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

func (s *CommandService) WithTrickplay(repo library.TrickplayRepository, gen library.TrickplayGenerator, cacheDir string) {
	s.trickplay = NewTrickplayService(repo, gen, cacheDir, s.mediaItems, s.events)
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
