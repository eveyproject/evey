package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

type TrickplayService struct {
	trickplayRepo     library.TrickplayRepository
	trickplayGen      library.TrickplayGenerator
	trickplayCacheDir string
	mediaItems        library.MediaItemCommandRepo
	events            shared.EventBus
	shutdownCtx       context.Context
}

func NewTrickplayService(
	repo library.TrickplayRepository,
	gen library.TrickplayGenerator,
	cacheDir string,
	mediaItems library.MediaItemCommandRepo,
	events shared.EventBus,
) *TrickplayService {
	return &TrickplayService{
		trickplayRepo:     repo,
		trickplayGen:      gen,
		trickplayCacheDir: cacheDir,
		mediaItems:        mediaItems,
		events:            events,
	}
}

func (s *TrickplayService) WithShutdownCtx(ctx context.Context) {
	s.shutdownCtx = ctx
}

func (s *TrickplayService) GenerateTrickplay(ctx context.Context, mediaItemID string) error {
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
			itemCtx, cancel := context.WithTimeout(bgCtx, 30*time.Minute)
			err := s.GenerateTrickplay(itemCtx, id)
			cancel()
			if err != nil {
				slog.Warn("trickplay generation failed", slog.String("media_id", id), slog.Any("err", err))
			} else {
				slog.Info("trickplay generated", slog.String("media_id", id))
			}
		}
	}()
	return len(ids), nil
}

func (s *TrickplayService) StartTrickplayWorker(ctx context.Context) {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return
	}
	sem := make(chan struct{}, 2)
	s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
		e, ok := ev.(library.MediaItemAdded)
		if !ok {
			return
		}
		// Only movies and episodes have playable files
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
