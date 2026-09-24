package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/cpubudget"
	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type CommandServiceOption func(*CommandService)

func WithShutdownCtx(ctx context.Context) CommandServiceOption {
	return func(s *CommandService) {
		if s.trickplay != nil {
			s.trickplay.WithShutdownCtx(ctx)
		}
	}
}

// WithCPUBudget hands the background CPU budget to the two workers here that
// size themselves on it: the scan's per-file fan-out and the trickplay
// decoder threads.
func WithCPUBudget(b *cpubudget.Budget) CommandServiceOption {
	return func(s *CommandService) {
		if s.scan != nil {
			s.scan.WithCPUBudget(b)
		}
		if s.trickplay != nil {
			s.trickplay.WithCPUBudget(b)
		}
	}
}

type CommandService struct {
	libraries  library.LibraryCommandRepo
	mediaItems library.MediaItemCommandRepo
	events     shared.EventBus

	scan *ScanService
	// aliveWatchedLibs reports the libraries whose watch the watcher POSITIVELY
	// vouches for - only those may be demoted to the backstop cadence. See
	// WithWatchHealth for why the polarity is alive-set, not unwatched-set.
	aliveWatchedLibs func() map[string]bool
	watchedScanMu    sync.Mutex
	lastWatchedScan  map[string]time.Time
	trickplay        *TrickplayService
	metadata         *MetadataExtractionService

	// search reindex is a WRITE (rebuilds the search index), so it lives on the
	// command side. mediaQuery reads the rows to index; both are nil unless
	// WithSearchReindex wired them (ReindexSearch then returns an error).
	search     library.SearchIndex
	mediaQuery library.MediaItemQueryRepo
	// mediaTranslations lets the reindex attach translated titles to the search
	// docs so a full rebuild is multi-language-searchable immediately (not only
	// after the next per-item re-enrich). nil = skip translation enrichment.
	mediaTranslations reindexTranslationReader
	// mediaCast lets the reindex attach people names so the searchable cast
	// attribute survives a full rebuild. nil = skip cast enrichment.
	mediaCast reindexCastReader
}

// WithSearchReindex enables CommandService.ReindexSearch by wiring the read repo
// the reindex sweeps and the translation/cast readers it enriches the docs from
// (either may be nil). The SearchIndex itself is already a constructor argument.
func WithSearchReindex(mediaQuery library.MediaItemQueryRepo, translations reindexTranslationReader, cast reindexCastReader) CommandServiceOption {
	return func(s *CommandService) {
		s.mediaQuery = mediaQuery
		s.mediaTranslations = translations
		s.mediaCast = cast
	}
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
		search:     search,
		trickplay:  NewTrickplayService(trickplayRepo, trickplayGen, trickplayCacheDir, mediaItems, events, trickplayConfigRepo),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ReindexSearch rebuilds the search index from the database for the given
// library (or all libraries when libraryID is ""). It WRITES the index, so it
// belongs on the command side; the shared logic lives in ReindexSearchIndex
// (also used by the CLI). Requires WithSearchReindex to have been wired.
func (s *CommandService) ReindexSearch(ctx context.Context, libraryID string, onProgress func(done, total int)) (int, error) {
	if s.mediaQuery == nil || s.search == nil {
		return 0, errors.New("library.CommandService.ReindexSearch: search reindex not configured (WithSearchReindex)")
	}
	return ReindexSearchIndex(ctx, s.mediaQuery, s.search, s.mediaTranslations, s.mediaCast, libraryID, onProgress)
}

func (s *CommandService) WithSubtitleScanner(sc library.ExternalSubtitleScanner) {
	s.scan.WithSubtitleScanner(sc)
	if s.metadata != nil {
		s.metadata.WithSubtitleScanner(sc)
	}
}

// WithIngestPlanning arms the scan-time serving verdict (mkvgo
// ops.Ingest) on the underlying scan service; gated by the
// library.ingest_serving_plan server_config setting.
func (s *CommandService) WithIngestPlanning(cfg shared.ServerConfigRepository, planner library.IngestPlanner, reindexer mkv.Reindexer) {
	s.scan.WithIngestPlanning(cfg, planner, reindexer)
}

// WithIngestActivePlayback lets the ingest pacer stretch its inter-file pause
// while streams are being served (weak-NAS politeness).
func (s *CommandService) WithIngestActivePlayback(count func() int) {
	s.scan.WithIngestActivePlayback(count)
}

// WithIngestKeyframeWarmer arms the scan-time keyframe warm-up consuming the
// freshly computed serving verdict (first-play fast path).
func (s *CommandService) WithIngestKeyframeWarmer(warm func(ctx context.Context, mediaItemID, path string) bool) {
	s.scan.WithKeyframeWarmer(warm)
}

// WithInterleaveProber arms the scan-time interleave measure (see
// ScanService.WithInterleaveProber).
func (s *CommandService) WithInterleaveProber(p InterleaveProber) {
	s.scan.WithInterleaveProber(p)
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
	dur, _, _ := s.scan.ProbeAndStoreStreams(ctx, mediaItemID, filePath)
	return dur
}

// StartPeriodicScan runs a lightweight scan at the given interval for specific library IDs
// where inotify is unavailable. If libraryIDs is empty, scans all libraries.
func (s *CommandService) StartPeriodicScan(ctx context.Context, libQueries library.LibraryQueryRepo, interval time.Duration, libraryIDs []string) {
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	if len(libraryIDs) > 0 {
		slog.InfoContext(ctx, "periodic scan started (fallback for inotify-failed libraries)", slog.Duration("interval", interval), slog.Int("libraries", len(libraryIDs)))
	} else {
		slog.InfoContext(ctx, "periodic scan started (all libraries)", slog.Duration("interval", interval))
	}
	targetSet := make(map[string]bool, len(libraryIDs))
	for _, id := range libraryIDs {
		targetSet[id] = true
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic recovered in periodic scan", "error", r, "stack", string(debug.Stack()))
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var lastScan time.Time

		// Boot scan, before the first tick. inotify is structurally blind to
		// anything that happened while the process was down, and the ticker
		// does not fire until T+interval - so without this, files added while
		// the server was stopped stay invisible for a whole interval. That was
		// tolerable at a 2-minute cadence and is not once the fallback is
		// spaced out.
		slog.InfoContext(ctx, "library scan: startup pass beginning (inotify cannot see changes made while the server was down)")
		s.runScanPass(ctx, libQueries, targetSet, "startup")
		lastScan = time.Now()
		slog.InfoContext(ctx, "library scan: startup pass complete",
			slog.Duration("next_periodic_in", interval),
			slog.Duration("watched_backstop_interval", watchedScanInterval))
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				// The walk below hits the same disk the viewer is reading from;
				// back off while anything is playing (see playbackScanBackoff).
				if !s.scan.shouldScanNow(now, lastScan, interval) {
					continue
				}
				lastScan = now
				s.runScanPass(ctx, libQueries, targetSet, "periodic")
			}
		}
	}()
}

// WithWatchHealth wires the watcher's liveness verdict: the set of libraries
// whose watch the watcher POSITIVELY vouches for. Only those are walked every
// watchedScanInterval; everything else - watch failed, watch dead, or simply
// monitoring disabled - keeps the operator's configured cadence. The polarity
// matters: the previous contract handed over the UNWATCHED set, and with
// monitoring disabled everywhere that set was EMPTY, so absence read as "watch
// alive" and every library was silently demoted to the hourly backstop (prod
// 2026-08-26: 365 fresh music files sat invisible for hours). Absence of proof
// must mean "scan", never "trust". Optional - unset means "trust nothing",
// i.e. scan everything on every tick.
func (s *CommandService) WithWatchHealth(aliveWatched func() map[string]bool) *CommandService {
	s.aliveWatchedLibs = aliveWatched
	return s
}

// runScanPass scans every targeted library once. Shared by the startup pass and
// the periodic ticker so the two can never drift apart. A per-library panic or
// error is contained: one bad library must not abort the others, nor kill the
// loop that will retry them.
func (s *CommandService) runScanPass(ctx context.Context, libQueries library.LibraryQueryRepo, targetSet map[string]bool, reason string) {
	libs, err := libQueries.List(ctx, library.ListLibrariesFilter{})
	if err != nil {
		slog.WarnContext(ctx, "scan pass: list libraries", slog.String("reason", reason), slog.Any("err", err))
		return
	}
	// Snapshot once per pass: a library whose watch is answering does not need
	// walking on every tick. Only a POSITIVE entry in the alive set demotes a
	// library to the backstop - a library the watcher never registered (watch
	// failed, or monitoring disabled by the operator) keeps the periodic
	// cadence, because nothing else will ever see its new files.
	var aliveWatched map[string]bool
	if s.aliveWatchedLibs != nil && reason == "periodic" {
		aliveWatched = s.aliveWatchedLibs()
	}
	for _, lib := range libs {
		if len(targetSet) > 0 && !targetSet[string(lib.ID)] {
			continue
		}
		if aliveWatched[string(lib.ID)] && !s.dueForWatchedScan(string(lib.ID)) {
			// THE question someone will ask in production is "why is my library
			// not being scanned". Answer it here, with the reason and when the
			// backstop is next due - a silent skip is undebuggable remotely.
			slog.DebugContext(ctx, "scan pass: skipped, watch is alive and the backstop is not due",
				slog.String("library", lib.Name), slog.String("library_id", string(lib.ID)),
				slog.Duration("since_last_walk", s.sinceWatchedScan(string(lib.ID))),
				slog.Duration("backstop_interval", watchedScanInterval))
			continue
		}
		scanReason := reason
		if reason == "periodic" && aliveWatched[string(lib.ID)] {
			scanReason = "watched_backstop"
		}
		metrics.LibraryScanTotal.WithLabelValues(string(lib.ID), scanReason).Inc()
		added, scanErr := func() (a int, err error) {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "scan pass: panic recovered", slog.String("library", lib.Name), slog.Any("panic", r))
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			a, _, err = s.ScanLibrary(ctx, string(lib.ID))
			return
		}()
		if scanErr != nil {
			slog.DebugContext(ctx, "scan pass: skip", slog.String("library", lib.Name), slog.String("reason", reason), slog.Any("err", scanErr))
			continue
		}
		// Always log the walk, not only when it found something: "the scan ran
		// and found nothing" and "the scan never ran" are different diagnoses
		// and used to look identical from the outside.
		if added > 0 {
			slog.InfoContext(ctx, "scan pass: new media found",
				slog.String("library", lib.Name), slog.String("reason", scanReason), slog.Int("new", added))
		} else {
			slog.DebugContext(ctx, "scan pass: completed, nothing new",
				slog.String("library", lib.Name), slog.String("reason", scanReason))
		}
	}
}

// sinceWatchedScan is how long since this library's last backstop walk, for
// logging. Zero when it has never run in this process.
func (s *CommandService) sinceWatchedScan(libID string) time.Duration {
	s.watchedScanMu.Lock()
	defer s.watchedScanMu.Unlock()
	last, ok := s.lastWatchedScan[libID]
	if !ok {
		return 0
	}
	return time.Since(last)
}

// dueForWatchedScan reports whether a library with a proven-alive watch is due
// for its hourly backstop walk, and records the pass when it is.
func (s *CommandService) dueForWatchedScan(libID string) bool {
	s.watchedScanMu.Lock()
	defer s.watchedScanMu.Unlock()
	if s.lastWatchedScan == nil {
		s.lastWatchedScan = make(map[string]time.Time)
	}
	last, seen := s.lastWatchedScan[libID]
	if seen && time.Since(last) < watchedScanInterval {
		return false
	}
	s.lastWatchedScan[libID] = time.Now()
	return true
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
	// Admin-triggered: a maintenance action may sweep up outdated manifests
	// (parameter-version below current), not just missing ones.
	return s.trickplay.GenerateAllMissingTrickplays(ctx, true)
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
	ctx, span := tracing.StartSpan(ctx, "app.library.CreateLibrary")
	defer span.End()
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
		slog.WarnContext(ctx, "publish event", slog.String("event", evtCreated.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *CommandService) UpdateLibrary(ctx context.Context, lib *library.Library) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.UpdateLibrary")
	defer span.End()
	// Same reason as CreateLibrary, which does validate: an invariant only
	// checked on the create path stops being an invariant at the first update.
	if err := lib.Validate(); err != nil {
		return fmt.Errorf("update library: %w", err)
	}
	lib.UpdatedAt = time.Now()
	if err := s.libraries.Update(ctx, lib); err != nil {
		return fmt.Errorf("update library: %w", err)
	}
	return nil
}

func (s *CommandService) DeleteLibrary(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.DeleteLibrary")
	defer span.End()
	if err := s.libraries.Delete(ctx, shared.LibraryID(id)); err != nil {
		return fmt.Errorf("delete library: %w", err)
	}
	evtDeleted := library.LibraryDeleted{
		LibraryID:  shared.LibraryID(id),
		OccurredAt: time.Now(),
	}
	if err := s.events.Publish(ctx, evtDeleted); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtDeleted.EventName()), slog.Any("err", err))
	}
	return nil
}

// RelocateMediaFile repoints a media item at a file that moved on disk (the
// seek-index wrong-container repair renames a mislabelled .mkv to .mp4). Only
// the path changes: the bytes are the same file, so the fingerprint, streams
// and watch history all stay valid. The next scan then finds the item where
// the catalog says it is instead of one vanished .mkv and one unknown .mp4.
func (s *CommandService) RelocateMediaFile(ctx context.Context, id, newPath string) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.RelocateMediaFile")
	defer span.End()
	item, err := s.mediaItems.GetByID(ctx, shared.MediaItemID(id))
	if err != nil {
		return fmt.Errorf("CommandService.RelocateMediaFile: %w", err)
	}
	if item.File == nil {
		return fmt.Errorf("CommandService.RelocateMediaFile: media %s has no file", id)
	}
	item.File.Path = newPath
	item.UpdatedAt = time.Now()
	if err := s.mediaItems.Update(ctx, item); err != nil {
		return fmt.Errorf("CommandService.RelocateMediaFile: %w", err)
	}
	return nil
}

func (s *CommandService) DeleteMediaItem(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.DeleteMediaItem")
	defer span.End()
	if err := s.mediaItems.Delete(ctx, shared.MediaItemID(id)); err != nil {
		return fmt.Errorf("CommandService.DeleteMediaItem: %w", err)
	}
	// Drop the search document too, or the title stays findable until the
	// 15-minute reconcile notices the count drift and rebuilds. Best-effort and
	// after the DB delete: the row is already gone, and the resilient search
	// adapter queues the removal for replay when the backend is down, so
	// failing here would only refuse a delete that already happened.
	if s.search != nil {
		if idxErr := s.search.RemoveMediaItem(ctx, id); idxErr != nil {
			slog.WarnContext(ctx, "library: search index remove failed",
				slog.String("mediaItemID", id), slog.Any("err", idxErr))
		}
	}
	return nil
}
