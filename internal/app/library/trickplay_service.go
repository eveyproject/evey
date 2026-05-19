package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/cpuutil"
	"github.com/evey-project/evey/internal/pkg/tracing"
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

	// genMu guards inProgress, the set of media item IDs currently being
	// generated. Two concurrent generations for the same item write the same
	// sprite directory and corrupt each other's output, so a second request
	// for an already-running item is skipped (generation is idempotent).
	genMu      sync.Mutex
	inProgress map[string]struct{}
}

// claimGeneration marks mediaItemID as generating and returns true. It returns
// false if a generation for the same item is already in progress, in which case
// the caller must skip to avoid corrupting the shared sprite directory.
func (s *TrickplayService) claimGeneration(mediaItemID string) bool {
	s.genMu.Lock()
	defer s.genMu.Unlock()
	if s.inProgress == nil {
		s.inProgress = make(map[string]struct{})
	}
	if _, running := s.inProgress[mediaItemID]; running {
		return false
	}
	s.inProgress[mediaItemID] = struct{}{}
	return true
}

func (s *TrickplayService) releaseGeneration(mediaItemID string) {
	s.genMu.Lock()
	delete(s.inProgress, mediaItemID)
	s.genMu.Unlock()
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
	ctx, span := tracing.StartSpan(ctx, "app.library.GenerateTrickplay")
	defer span.End()
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return errors.New("trickplay not configured")
	}
	if !s.isTrickplayEnabled(ctx) {
		return nil
	}
	if !s.claimGeneration(mediaItemID) {
		slog.Debug("trickplay generation already in progress, skipping", slog.String("media_item_id", mediaItemID))
		return nil
	}
	defer s.releaseGeneration(mediaItemID)
	s.waitForIdleStreams(ctx)

	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	if item.File == nil || item.File.Path == "" {
		return errors.New("media item has no file path")
	}

	interval := 7
	width := 320
	tileWidth := 5
	tileHeight := 5

	outputDir := fmt.Sprintf("%s/%s", s.trickplayCacheDir, mediaItemID)

	adaptive := s.isAdaptiveCPU(ctx)
	workers := cpuutil.TrickplayWorkers(adaptive)
	result, err := s.trickplayGen.Generate(ctx, item.File.Path, outputDir, interval, width, tileWidth, tileHeight, workers)
	if err != nil {
		return fmt.Errorf("generate trickplay: %w", err)
	}

	if err := writeTrickplaySidecar(outputDir, tileWidth, tileHeight, result.FrameTimestampsMs); err != nil {
		return fmt.Errorf("write trickplay sidecar: %w", err)
	}

	manifest := &library.TrickplayManifest{
		ID:          uuid.New().String(),
		MediaItemID: shared.MediaItemID(mediaItemID),
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

	if s.events != nil {
		s.events.Publish(ctx, library.TrickplayGenerated{
			MediaItemID: shared.MediaItemID(mediaItemID),
			ParentID:    shared.MediaItemID(item.ParentID),
			Type:        string(item.Type),
			OccurredAt:  time.Now(),
		})
	}
	return nil
}

// trickplaySidecar is the JSON payload written next to the sprites. Stores the
// real per-frame timestamps so the player and intro detector can map a tile
// index ↔ source video time without assuming uniform interval (which is wrong
// when ffmpeg snaps to keyframes).
type trickplaySidecar struct {
	TileWidth        int     `json:"tile_width"`
	TileHeight       int     `json:"tile_height"`
	FramesPerSprite  int     `json:"frames_per_sprite"`
	FrameTimestamps  []int64 `json:"frame_ms"`
}

func writeTrickplaySidecar(outputDir string, tileWidth, tileHeight int, frameMs []int64) error {
	if len(frameMs) == 0 {
		return errors.New("no frame timestamps")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("ensure %s: %w", outputDir, err)
	}
	sc := trickplaySidecar{
		TileWidth:       tileWidth,
		TileHeight:      tileHeight,
		FramesPerSprite: tileWidth * tileHeight,
		FrameTimestamps: frameMs,
	}
	buf, err := json.Marshal(sc)
	if err != nil {
		return fmt.Errorf("marshal sidecar: %w", err)
	}
	path := filepath.Join(outputDir, "trickplay.json")
	if err := os.WriteFile(path, buf, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// readTrickplaySidecar reads the sidecar written by writeTrickplaySidecar.
// Returns an error if absent — there is no fallback to interval-based timing.
func readTrickplaySidecar(outputDir string) (*trickplaySidecar, error) {
	path := filepath.Join(outputDir, "trickplay.json")
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc trickplaySidecar
	if err := json.Unmarshal(buf, &sc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &sc, nil
}

func (s *TrickplayService) GetTrickplayManifest(ctx context.Context, mediaItemID string) (*library.TrickplayManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetTrickplayManifest")
	defer span.End()
	if s.trickplayRepo == nil {
		return nil, errors.New("trickplay not configured")
	}
	return s.trickplayRepo.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
}

func (s *TrickplayService) GenerateAllMissingTrickplays(ctx context.Context) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GenerateAllMissingTrickplays")
	defer span.End()
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
			slog.Warn("trickplay queue full, dropping", slog.String("media_id", string(e.MediaItemID)))
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
			if _, err := s.GetTrickplayManifest(tctx, string(e.MediaItemID)); err == nil {
				return
			}
			if err := s.GenerateTrickplay(tctx, string(e.MediaItemID)); err != nil {
				slog.Warn("trickplay generation failed", slog.String("media_id", string(e.MediaItemID)), slog.Any("err", err))
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
