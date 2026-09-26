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

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/bgtasks"
	"github.com/evey-project/evey/internal/pkg/cpubudget"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"github.com/google/uuid"
)

// Trickplay generation parameters. Changing ANY of them requires bumping
// trickplayAlgoVersion below, or every pre-existing manifest silently keeps
// the old sampling forever (the missing-sweep re-lists rows below the current
// version; regeneration is throttled like any generation).
const (
	trickplayInterval   = 7
	trickplayWidth      = 320
	trickplayTileWidth  = 5
	trickplayTileHeight = 5
	// trickplayAlgoVersion 1 = the 7s/320px/5x5 parameters above. Rows at 0
	// are legacy manifests from before versioning (the original 10s/10x10
	// defaults or anything since); the ADMIN-triggered sweep regenerates them.
	trickplayAlgoVersion = 1
	// outdatedRegenAnnounceThreshold: above this many re-listed manifests the
	// admin sweep says out loud how big the job is, so a day of background
	// ffmpeg is never a surprise.
	outdatedRegenAnnounceThreshold = 50
)

// ActiveStreamChecker reports whether ANY playback is currently reading the
// disk, whatever machinery serves it. Deliberately not ActiveSessionCount: that
// one counts ffmpeg sessions only, and trickplay generation must also hold off
// for a direct-play or mkvgo-served viewer it would otherwise compete with.
type ActiveStreamChecker interface {
	ActivePlaybackCount() int
}

type TrickplayService struct {
	trickplayRepo     library.TrickplayRepository
	trickplayGen      library.TrickplayGenerator
	trickplayCacheDir string
	mediaItems        library.MediaItemCommandRepo
	events            shared.EventBus
	configRepo        shared.ServerConfigRepository
	activeStreams     ActiveStreamChecker
	shutdownCtx       context.Context
	// budget sizes the decoder threads of one generation on the admin's CPU
	// share for this task (see cpubudget); nil decodes on one thread.
	budget *cpubudget.Budget

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
	registerTrickplayCoverage(repo)
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
	for s.activeStreams.ActivePlaybackCount() > 0 {
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

// WithCPUBudget wires the background CPU budget the generation's threads are
// sized on. The forked ffmpeg reserves those threads in the same budget
// (procfork), so the share is both the size and the gate.
func (s *TrickplayService) WithCPUBudget(b *cpubudget.Budget) {
	s.budget = b
}

// decoderThreads is the thread count one generation decodes with: one on the
// global share, the task's own share when the admin gave it one. Sized on the
// whole global share, each generation held every slot of the group and a
// season's backlog starved the scan for hours (measured on dev 26-09: scan
// waiting, trickplay 2/2); keyframe-only decoding takes 0.57 core on one
// thread anyway.
func (s *TrickplayService) decoderThreads() int {
	if s.budget == nil || !s.budget.HasOwnShare(trickplayTask) {
		return 1
	}
	return s.budget.Slots(trickplayTask)
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
		slog.DebugContext(ctx, "trickplay generation already in progress, skipping", slog.String("media_item_id", mediaItemID))
		return nil
	}
	defer s.releaseGeneration(mediaItemID)
	s.waitForIdleStreams(ctx)

	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if errors.Is(err, shared.ErrNotFound) {
		// Deleted before the job ran. Nothing to do, and nothing wrong.
		slog.DebugContext(ctx, "trickplay: media item gone before generation, skipping",
			slog.String("media_item_id", mediaItemID))
		return nil
	}
	if err != nil {
		return fmt.Errorf("get media item: %w", err)
	}
	if item.File == nil || item.File.Path == "" {
		return errors.New("media item has no file path")
	}

	interval := trickplayInterval
	width := trickplayWidth
	tileWidth := trickplayTileWidth
	tileHeight := trickplayTileHeight

	outputDir := fmt.Sprintf("%s/%s", s.trickplayCacheDir, mediaItemID)

	result, err := s.trickplayGen.Generate(ctx, item.File.Path, outputDir, interval, width, tileWidth, tileHeight, s.decoderThreads(), item.File.Duration.Milliseconds())
	if err != nil {
		s.recordGenerationFailure(ctx, mediaItemID, item.File, err)
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
		AlgoVersion: trickplayAlgoVersion,
		CreatedAt:   time.Now(),
	}

	if err := s.trickplayRepo.Create(ctx, manifest); err != nil {
		if errors.Is(err, library.ErrMediaItemGone) {
			// Deleted DURING the (minutes-long) generation - the FK target is
			// gone. The images we just made are orphaned; drop them and move on.
			slog.DebugContext(ctx, "trickplay: media item deleted during generation, discarding output",
				slog.String("media_item_id", mediaItemID))
			_ = os.RemoveAll(outputDir)
			return nil
		}
		return fmt.Errorf("store trickplay manifest: %w", err)
	}

	if s.events != nil {
		s.events.Publish(ctx, library.TrickplayGenerated{
			MediaItemID: shared.MediaItemID(mediaItemID),
			ParentID:    shared.MediaItemID(item.ParentID),
			Type:        item.Type,
			OccurredAt:  time.Now(),
		})
	}
	return nil
}

// recordGenerationFailure persists a PERMANENT generation failure so the
// missing-list stops re-serving the item at every boot (the same two files
// were re-attempted on each restart before this existed). Transient
// conditions are exempt: a context error says nothing about the file (a
// shutdown mid-batch or a blown time budget on a busy NAS), and a file the OS
// can't stat may heal (unmounted share). The recorded size+mod time are the
// DB's - the values ListMissingMediaItemIDs compares against - so a later
// rescan of a replaced file invalidates the record naturally. Best-effort: a
// failed write only logs, the next sweep simply retries.
func (s *TrickplayService) recordGenerationFailure(ctx context.Context, mediaItemID string, file *library.MediaFile, genErr error) {
	// The context itself, not only the error: an admin's Stop kills the ffmpeg
	// mid-encode and what comes back is "signal: killed", which says nothing
	// about the file either.
	if ctx.Err() != nil || errors.Is(genErr, context.Canceled) || errors.Is(genErr, context.DeadlineExceeded) {
		return
	}
	if _, statErr := os.Stat(file.Path); statErr != nil {
		return
	}
	if err := s.trickplayRepo.RecordFailure(ctx, shared.MediaItemID(mediaItemID), file.Size, file.ModTime, genErr.Error()); err != nil {
		slog.WarnContext(ctx, "trickplay: record failure failed",
			slog.String("media_id", mediaItemID), slog.Any("err", err))
	}
}

// trickplaySidecar is the JSON payload written next to the sprites. Stores the
// real per-frame timestamps so the player and intro detector can map a tile
// index ↔ source video time without assuming uniform interval (which is wrong
// when ffmpeg snaps to keyframes).
type trickplaySidecar struct {
	TileWidth       int     `json:"tile_width"`
	TileHeight      int     `json:"tile_height"`
	FramesPerSprite int     `json:"frames_per_sprite"`
	FrameTimestamps []int64 `json:"frame_ms"`
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
// Returns an error if absent - there is no fallback to interval-based timing.
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

// GenerateAllMissingTrickplays queues generation for every item without a
// manifest. includeOutdated additionally re-lists manifests stamped below the
// current parameter version - a MASS regeneration (potentially the whole
// library after a params change), which is a maintenance operation: only the
// admin-triggered endpoints pass true. The boot sweep passes false, so a
// restart after an algo bump never silently commits the box to a day of
// ffmpeg - the operator decides.
func (s *TrickplayService) GenerateAllMissingTrickplays(ctx context.Context, includeOutdated bool) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GenerateAllMissingTrickplays")
	defer span.End()
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return 0, errors.New("trickplay not configured")
	}
	if !s.isTrickplayEnabled(ctx) {
		return 0, nil
	}
	minVersion := 0 // 0: any existing manifest satisfies the version clause → missing-only
	if includeOutdated {
		minVersion = trickplayAlgoVersion
	}
	ids, err := s.trickplayRepo.ListMissingMediaItemIDs(ctx, minVersion)
	if err != nil {
		return 0, fmt.Errorf("list missing trickplays: %w", err)
	}
	if includeOutdated && len(ids) >= outdatedRegenAnnounceThreshold {
		slog.InfoContext(ctx, "trickplay: regenerating manifests after a parameter change - sequential and throttled, this can take hours",
			slog.Int("count", len(ids)))
	}
	bgCtx := s.shutdownCtx
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(bgCtx, "recovered panic in trickplay batch", slog.Any("error", r))
			}
		}()
		bgtasks.Queue(trickplayTask, len(ids))
		for i, id := range ids {
			if bgCtx.Err() != nil {
				return
			}
			// The idle-gate makes the LOAD invisible but stretches the DURATION
			// (a busy library spends its time waiting): the registry is how the
			// operator sees that the sweep runs and how much remains.
			// An admin's Stop holds the sweep here until Resume (or shutdown),
			// and cancels the item in flight through the registry's context.
			if bgtasks.WaitResumed(bgCtx, trickplayTask, nil) != nil {
				return
			}
			runCtx, done := bgtasks.BeginCtx(bgCtx, trickplayTask, fmt.Sprintf("%s (%d/%d)", id, i+1, len(ids)))
			itemCtx, cancel := context.WithTimeout(runCtx, 30*time.Minute)
			err := s.GenerateTrickplay(itemCtx, id)
			cancel()
			done()
			if err != nil {
				slog.WarnContext(bgCtx, "trickplay generation failed", slog.String("media_id", id), slog.Any("err", err))
			} else {
				slog.InfoContext(bgCtx, "trickplay generated", slog.String("media_id", id))
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

// trickplayTask is the sweep's name in the bgtasks registry.
const trickplayTask = "trickplay_generation"

func (s *TrickplayService) StartTrickplayWorker(ctx context.Context) {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return
	}
	bgtasks.RegisterControllable(trickplayTask)
	// One worker drains a FIFO: episodes land in bursts, and the one-slot
	// semaphore this replaces dropped every arrival while one was generating
	// ("trickplay queue full, dropping", prod 2026-09-25) until the next boot.
	queue := make(chan string, trickplayEventQueue)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-queue:
				s.generateQueued(ctx, id)
			}
		}
	}()
	s.events.Subscribe("media.item.added", func(_ context.Context, ev shared.Event) {
		if !s.isTrickplayEnabled(ctx) {
			return
		}
		// Stopped by an admin: the item is not lost, the next sweep lists it.
		if bgtasks.Paused(trickplayTask) {
			return
		}
		e, ok := ev.(library.MediaItemAdded)
		if !ok {
			return
		}
		if e.Type != library.MovieItem && e.Type != library.Episode {
			return
		}
		select {
		case queue <- string(e.MediaItemID):
			bgtasks.Queue(trickplayTask, 1)
		default:
			slog.WarnContext(ctx, "trickplay queue full, dropping", slog.String("media_id", string(e.MediaItemID)))
		}
	})
}

// trickplayEventQueue bounds the items waiting behind the one in flight; a
// whole season copied at once fits.
const trickplayEventQueue = 1024

// generateQueued is one event-driven generation, skipped when a manifest
// already exists.
func (s *TrickplayService) generateQueued(ctx context.Context, id string) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "recovered panic in trickplay worker", slog.Any("error", r))
		}
	}()
	tctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	// Under the task's context like the sweep: the dashboard sees the item,
	// and the fork is charged to (and gated by) this task's share.
	tctx, done := bgtasks.BeginCtx(tctx, trickplayTask, id)
	defer done()
	if _, err := s.GetTrickplayManifest(tctx, id); err == nil {
		return
	}
	if err := s.GenerateTrickplay(tctx, id); err != nil {
		slog.WarnContext(tctx, "trickplay generation failed", slog.String("media_id", id), slog.Any("err", err))
	}
}

func (s *TrickplayService) StartBackgroundGeneration(ctx context.Context) {
	if s.trickplayRepo == nil || s.trickplayGen == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "recovered panic in trickplay background gen", slog.Any("error", r))
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
		slog.InfoContext(ctx, "trickplay: starting background generation of missing trickplays")
		// Boot sweep: MISSING manifests only. Outdated (params-version below
		// current) regenerate on explicit admin trigger - a restart must never
		// silently start a library-wide regeneration.
		n, err := s.GenerateAllMissingTrickplays(ctx, false)
		if err != nil {
			slog.WarnContext(ctx, "trickplay: background generation failed", slog.Any("err", err))
			return
		}
		if n > 0 {
			slog.InfoContext(ctx, "trickplay: background generation queued", slog.Int("count", n))
		}
	}()
}
