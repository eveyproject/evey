package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/adapters/driven/ffmpeg"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

// ActiveStreamChecker reports whether any transcode/playback is active.
type ActiveStreamChecker interface {
	ActiveSessionCount() int
}

type TrickplayService struct {
	trickplayRepo     library.TrickplayRepository
	trickplayGen      library.TrickplayGenerator
	trickplayCacheDir string
	mediaItems        library.MediaItemCommandRepo
	events            shared.EventBus
	configRepo        shared.ServerConfigRepository
	activeStreams      ActiveStreamChecker
	shutdownCtx       context.Context
}

func NewTrickplayService(
	repo library.TrickplayRepository,
	gen library.TrickplayGenerator,
	cacheDir string,
	mediaItems library.MediaItemCommandRepo,
	events shared.EventBus,
	configRepo shared.ServerConfigRepository,
) *TrickplayService {
	return &TrickplayService{
		trickplayRepo:     repo,
		trickplayGen:      gen,
		trickplayCacheDir: cacheDir,
		mediaItems:        mediaItems,
		events:            events,
		configRepo:        configRepo,
	}
}

func (s *TrickplayService) WithActiveStreamChecker(c ActiveStreamChecker) {
	s.activeStreams = c
}

func (s *TrickplayService) waitForIdleStreams(ctx context.Context) {
	if s.activeStreams == nil {
		return
	}
	for s.activeStreams.ActiveSessionCount() > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}

func (s *TrickplayService) WithShutdownCtx(ctx context.Context) {
	s.shutdownCtx = ctx
}

func (s *TrickplayService) isTrickplayEnabled(ctx context.Context) bool {
	if s.configRepo == nil {
		return true
	}
	entry, err := s.configRepo.Get(ctx, "trickplay.enabled")
	if err != nil {
		return true
	}
	return entry.Value != "false"
}

// isAdaptiveCPU reads `trickplay.adaptive_cpu` from config. Default: true (gentle 25%).
// When false, uses 50% of cores for faster generation.
func (s *TrickplayService) isAdaptiveCPU(ctx context.Context) bool {
	if s.configRepo == nil {
		return true
	}
	entry, err := s.configRepo.Get(ctx, "trickplay.adaptive_cpu")
	if err != nil {
		return true
	}
	return entry.Value != "false"
}

func (s *TrickplayService) GenerateTrickplay(ctx context.Context, mediaItemID string) error {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return errors.New("trickplay not configured")
	}
	if !s.isTrickplayEnabled(ctx) {
		return nil
	}
	s.waitForIdleStreams(ctx)

	item, err := s.mediaItems.GetByID(ctx, mediaItemID)
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	if item.FilePath == "" {
		return errors.New("media item has no file path")
	}

	interval := 15
	width := 120
	tileWidth := 10
	tileHeight := 10

	outputDir := fmt.Sprintf("%s/%s", s.trickplayCacheDir, mediaItemID)

	adaptive := s.isAdaptiveCPU(ctx)
	workers := ffmpeg.TrickplayWorkers(adaptive)
	result, err := s.trickplayGen.Generate(ctx, item.FilePath, outputDir, interval, width, tileWidth, tileHeight, workers)
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

func (s *TrickplayService) GetTrickplayManifest(ctx context.Context, mediaItemID string) (*library.TrickplayManifest, error) {
	if s.trickplayRepo == nil {
		return nil, errors.New("trickplay not configured")
	}
	return s.trickplayRepo.GetByMediaItemID(ctx, mediaItemID)
}

func (s *TrickplayService) GenerateAllMissingTrickplays(ctx context.Context) (int, error) {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return 0, errors.New("trickplay not configured")
	}
	if !s.isTrickplayEnabled(ctx) {
		return 0, nil
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
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in trickplay batch", slog.Any("error", r))
			}
		}()
		for _, id := range ids {
			if bgCtx.Err() != nil {
				return
			}
			itemCtx, cancel := context.WithTimeout(bgCtx, 30*time.Minute)
			err := s.GenerateTrickplay(itemCtx, id)
			cancel()
			if err != nil {
				slog.Warn("trickplay generation failed", slog.String("media_id", id), slog.Any("err", err))
			} else {
				slog.Info("trickplay generated", slog.String("media_id", id))
			}
			// Throttle: pause between items to avoid hogging CPU
			select {
			case <-bgCtx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}()
	return len(ids), nil
}

func (s *TrickplayService) StartTrickplayWorker(ctx context.Context) {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return
	}
	sem := make(chan struct{}, 1)
	s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
		if !s.isTrickplayEnabled(ctx) {
			return
		}
		e, ok := ev.(library.MediaItemAdded)
		if !ok {
			return
		}
		if e.Type != string(library.MovieItem) && e.Type != string(library.Episode) {
			return
		}
		select {
		case sem <- struct{}{}:
		default:
			slog.Warn("trickplay queue full, dropping", slog.String("media_id", e.MediaItemID))
			return
		}
		go func() {
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("recovered panic in trickplay worker", slog.Any("error", r))
				}
			}()
			tctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
			defer cancel()
			if _, err := s.GetTrickplayManifest(tctx, e.MediaItemID); err == nil {
				return
			}
			if err := s.GenerateTrickplay(tctx, e.MediaItemID); err != nil {
				slog.Warn("trickplay generation failed", slog.String("media_id", e.MediaItemID), slog.Any("err", err))
			}
		}()
	})
}

func (s *TrickplayService) StartBackgroundGeneration(ctx context.Context) {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in trickplay background gen", slog.Any("error", r))
			}
		}()
		select {
		case <-time.After(60 * time.Second):
		case <-ctx.Done():
			return
		}
		if !s.isTrickplayEnabled(ctx) {
			return
		}
		slog.Info("trickplay: starting background generation of missing trickplays")
		n, err := s.GenerateAllMissingTrickplays(ctx)
		if err != nil {
			slog.Warn("trickplay: background generation failed", slog.Any("err", err))
			return
		}
		if n > 0 {
			slog.Info("trickplay: background generation queued", slog.Int("count", n))
		}
	}()
}
