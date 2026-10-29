package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

const defaultDebounceDelay = 2 * time.Second

// LibraryScanner is the subset of Service needed by the watcher.
type LibraryScanner interface {
	ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error)
}

type WatcherService struct {
	watcher   library.FileWatcher
	libraries library.LibraryQueryRepo
	scanner   LibraryScanner
	log       *slog.Logger
	bus       shared.EventBus
	// debounceDelay is how long FS events coalesce before a scan fires (and the
	// re-arm interval when a scan was already in progress). Injectable so tests
	// can shrink it to milliseconds - the real-time default makes timing-based
	// tests slow and flaky on slower runners.
	debounceDelay time.Duration

	mu         sync.Mutex
	pathToLib  map[string]string // watched path → library ID
	failedLibs map[string]bool   // library IDs where inotify failed → need periodic scan
	probes     *probeState       // liveness of each library's watch (watcher_probe.go)
	// noWatchLogged gates the one-shot "no library wants realtime monitoring"
	// notice. Monitoring disabled everywhere is an operator CHOICE: say it once
	// at INFO, not as a WARN every probe round (prod 2026-07-23: 247 of 254
	// daily WARNs were this, burying the real signals). Re-armed by a
	// successful watch so a later disable gets one fresh notice.
	noWatchLogged bool
	// probeTimeout is how long a probe waits for its own event. Injectable for
	// the same reason debounceDelay is: the real 30 s makes the failure-path
	// tests unbearably slow, and a test that cancels the context instead would
	// exercise the SHUTDOWN branch rather than the dead-watch one.
	probeWait time.Duration
	// networkFS reports whether a path sits on a filesystem the local kernel
	// cannot see remote writes on. Injected (adapter concern) and optional:
	// nil means "assume everything is local", i.e. the prior behaviour.
	networkFS func(path string) (string, bool)
	cancel    context.CancelFunc
	ctx       context.Context
}

type WatcherOption func(*WatcherService)

func WatcherWithEventBus(bus shared.EventBus) WatcherOption {
	return func(ws *WatcherService) { ws.bus = bus }
}

// WatcherWithDebounce overrides the FS-event debounce/re-arm interval. Tests use
// a tiny value so they don't wait real seconds (and don't flake on slow boxes).
func WatcherWithDebounce(d time.Duration) WatcherOption {
	return func(ws *WatcherService) {
		if d > 0 {
			ws.debounceDelay = d
		}
	}
}

func NewWatcherService(
	watcher library.FileWatcher,
	libraries library.LibraryQueryRepo,
	scanner LibraryScanner,
	log *slog.Logger,
	opts ...WatcherOption,
) *WatcherService {
	ws := &WatcherService{
		watcher:       watcher,
		libraries:     libraries,
		scanner:       scanner,
		log:           log,
		pathToLib:     make(map[string]string),
		failedLibs:    make(map[string]bool),
		probes:        newProbeState(),
		probeWait:     probeTimeout,
		debounceDelay: defaultDebounceDelay,
	}
	for _, opt := range opts {
		opt(ws)
	}
	return ws
}

// Start watches all libraries with RealtimeMonitoring=true and processes FS events.
func (ws *WatcherService) Start(ctx context.Context) error {
	ctx, ws.cancel = context.WithCancel(ctx)
	ws.ctx = ctx

	libs, err := ws.libraries.List(ctx, library.ListLibrariesFilter{})
	if err != nil {
		return fmt.Errorf("WatcherService.Start: %w", err)
	}
	for _, lib := range libs {
		if !lib.RealtimeMonitoring || !lib.Enabled {
			continue
		}
		ws.watchLibrary(lib)
	}

	if ws.bus != nil {
		ws.bus.Subscribe("library.created", func(ctx context.Context, ev shared.Event) {
			if e, ok := ev.(library.LibraryCreated); ok {
				lib, err := ws.libraries.GetByID(ctx, shared.LibraryID(e.LibraryID))
				if err == nil {
					ws.AddLibrary(lib)
				}
			}
		})
		ws.bus.Subscribe("library.deleted", func(_ context.Context, ev shared.Event) {
			if e, ok := ev.(library.LibraryDeleted); ok {
				ws.RemoveLibraryByID(string(e.LibraryID))
			}
		})
		ws.bus.Subscribe("library.updated", func(ctx context.Context, ev shared.Event) {
			if e, ok := ev.(library.LibraryUpdated); ok {
				ws.rearmLibrary(ctx, string(e.LibraryID))
			}
		})
	}

	go ws.processEvents(ctx)
	return nil
}

func (ws *WatcherService) watchLibrary(lib *library.Library) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	allFailed := true
	for _, p := range lib.Paths {
		if err := ws.watcher.Watch(p); err != nil {
			ws.log.Warn("watcher: inotify failed for path, will use periodic scan", slog.String("path", p), slog.String("library", lib.Name), slog.Any("err", err))
			continue
		}
		allFailed = false
		ws.pathToLib[p] = string(lib.ID)
		ws.noWatchLogged = false
		ws.log.Info("watcher: watching", slog.String("path", p), slog.String("library", lib.Name))
	}
	if allFailed && len(lib.Paths) > 0 {
		ws.failedLibs[string(lib.ID)] = true
		ws.log.Info("watcher: library needs periodic scan fallback", slog.String("library", lib.Name))
	}
}

// AddLibrary registers a library for watching at runtime (e.g. after creation).
func (ws *WatcherService) AddLibrary(lib *library.Library) {
	if !lib.RealtimeMonitoring || !lib.Enabled {
		return
	}
	ws.watchLibrary(lib)
}

// rearmLibrary drops a library's watch and arms it again from what is stored
// now: its paths may have moved, and its monitoring switch may have flipped
// either way.
func (ws *WatcherService) rearmLibrary(ctx context.Context, libraryID string) {
	lib, err := ws.libraries.GetByID(ctx, shared.LibraryID(libraryID))
	if err != nil {
		ws.log.Warn("watcher: updated library not readable, watch left as it was", slog.String("library_id", libraryID), slog.Any("err", err))
		return
	}
	ws.RemoveLibraryByID(libraryID)
	ws.AddLibrary(lib)
}

// RemoveLibraryByID stops watching all paths for a library by ID (used when lib is already deleted).
// Its watch failure and probe verdict go with it: they described those paths.
func (ws *WatcherService) RemoveLibraryByID(libraryID string) {
	ws.probes.forget(libraryID)
	ws.mu.Lock()
	delete(ws.failedLibs, libraryID)
	var pathsToUnwatch []string
	for p, id := range ws.pathToLib {
		if id == libraryID {
			pathsToUnwatch = append(pathsToUnwatch, p)
			delete(ws.pathToLib, p)
		}
	}
	ws.mu.Unlock()

	for _, p := range pathsToUnwatch {
		if err := ws.watcher.Unwatch(p); err != nil {
			ws.log.Warn("watcher: unwatch failed", slog.String("path", p), slog.Any("err", err))
		}
	}
}

func (ws *WatcherService) Stop() {
	if ws.cancel != nil {
		ws.cancel()
	}
	if err := ws.watcher.Close(); err != nil {
		ws.log.Warn("watcher: close failed", slog.Any("err", err))
	}
}

// processEvents debounces FS events per library and triggers a scan.
func (ws *WatcherService) processEvents(ctx context.Context) {
	pending := make(map[string]*time.Timer) // libraryID → debounce timer
	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, t := range pending {
				t.Stop()
			}
			mu.Unlock()
			return

		case ev, ok := <-ws.watcher.Events():
			if !ok {
				return
			}
			// Our own liveness probe: deliver it to whoever is waiting and stop
			// there. It must never reach the debounce below - the probe fires on
			// a timer, so scanning on it would walk the whole library every
			// probe round, which is precisely what this machinery exists to
			// avoid.
			if isProbePath(ev.Path) {
				ws.probes.deliver(ev.Path)
				continue
			}

			libID := ws.resolveLibrary(ev.Path)
			if libID == "" {
				continue
			}
			ws.log.Debug("watcher: file event", slog.String("path", ev.Path), slog.Int("op", int(ev.Op)), slog.String("library_id", libID))

			mu.Lock()
			if t, exists := pending[libID]; exists {
				t.Reset(ws.debounceDelay)
			} else {
				pending[libID] = time.AfterFunc(ws.debounceDelay, func() {
					err := ws.triggerScan(ctx, libID)
					mu.Lock()
					defer mu.Unlock()
					// If a scan was already running this fire was rejected, so the
					// file(s) that triggered it were NOT picked up. Re-arm instead
					// of dropping the pending entry, so we retry once the in-flight
					// scan frees up (otherwise the change is lost until the next FS
					// event or a scheduled scan).
					if errors.Is(err, ErrScanInProgress) {
						if t := pending[libID]; t != nil {
							t.Reset(ws.debounceDelay)
							return
						}
					}
					delete(pending, libID)
				})
			}
			mu.Unlock()

		case err, ok := <-ws.watcher.Errors():
			if !ok {
				return
			}
			ws.log.Warn("watcher: error", slog.Any("err", err))
		}
	}
}

// resolveLibrary finds which library owns the given file path.
func (ws *WatcherService) resolveLibrary(filePath string) string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	for watchedPath, libID := range ws.pathToLib {
		rel, err := filepath.Rel(watchedPath, filePath)
		if err != nil {
			continue
		}
		if len(rel) > 0 && rel[0] != '.' {
			return libID
		}
		if rel == "." {
			return libID
		}
	}
	return ""
}

func (ws *WatcherService) triggerScan(ctx context.Context, libraryID string) (err error) {
	if ws.scanner == nil {
		return errors.New("watcher: no scanner wired")
	}
	// This runs on the debounce timer's own goroutine (time.AfterFunc), where an
	// unrecovered panic in ScanLibrary would take down the whole process. The
	// scheduler path already recovers (scheduler.go runScan); stay symmetric so a
	// buggy scan of one library can't crash the server via a filesystem event.
	// Surfacing it as an error lets the debounce callback drop the pending entry
	// like any other scan failure (a panic never matches ErrScanInProgress, so it
	// is not re-armed).
	defer func() {
		if r := recover(); r != nil {
			ws.log.Error("watcher: scan panicked", slog.String("library_id", libraryID), slog.Any("panic", r))
			err = fmt.Errorf("watcher: scan of %s panicked: %v", libraryID, r)
		}
	}()
	ws.log.Info("watcher: triggering scan", slog.String("library_id", libraryID))
	added, updated, err := ws.scanner.ScanLibrary(ctx, libraryID)
	if err != nil {
		ws.log.Error("watcher: scan failed", slog.String("library_id", libraryID), slog.Any("err", err))
		return err
	}
	ws.log.Info("watcher: scan complete", slog.String("library_id", libraryID), slog.Int("added", added), slog.Int("updated", updated))
	return nil
}

// UnwatchedLibraries reports the libraries the periodic scan must still walk
// frequently: those whose watches all failed to establish, plus those whose
// liveness probe went unanswered. Safe to call from another goroutine.
//
// An empty result does NOT mean "no scanning needed" - a watch can be alive and
// still blind (a network share written by another host), so the caller keeps a
// spaced-out full scan as a backstop. See watcher_probe.go.
func (ws *WatcherService) UnwatchedLibraries() map[string]bool {
	out := ws.probes.UnhealthyLibraries()
	ws.mu.Lock()
	for id := range ws.failedLibs {
		out[id] = true
	}
	ws.mu.Unlock()
	return out
}

// AliveWatchedLibraries is the POSITIVE view the periodic scan's adaptive
// cadence needs: the libraries with at least one established watch, minus
// those the health machinery cannot vouch for (failed watch, dead probe).
// Only a library in THIS set may be demoted to the spaced-out backstop.
//
// The polarity is the whole point. Feeding the scan the unwatched-set meant
// that with realtime monitoring disabled on every library the set came back
// EMPTY, absence read as "watch alive", and every library was silently demoted
// to the hourly backstop - in prod (2026-08-26) 365 fresh music files sat
// invisible for hours while the operator's 2-minute cadence config did
// nothing. A library this map does not name gets scanned; proof, not absence,
// is what buys the slower cadence. Safe to call from another goroutine.
func (ws *WatcherService) AliveWatchedLibraries() map[string]bool {
	unhealthy := ws.probes.UnhealthyLibraries()
	ws.mu.Lock()
	defer ws.mu.Unlock()
	out := make(map[string]bool, len(ws.pathToLib))
	for _, libID := range ws.pathToLib {
		if unhealthy[libID] || ws.failedLibs[libID] {
			continue
		}
		out[libID] = true
	}
	return out
}

// WatcherWithProbeTimeout overrides how long a liveness probe waits for its own
// event. Tests use a tiny value; production keeps probeTimeout.
func WatcherWithProbeTimeout(d time.Duration) WatcherOption {
	return func(ws *WatcherService) {
		if d > 0 {
			ws.probeWait = d
		}
	}
}

// WatcherWithNetworkFSDetector wires network-share detection. A library on
// NFS/SMB gets a PASSING probe for the wrong reason - the server's own write
// raises the local event, while another client's writes never do - so the
// prober must refuse to vouch for it rather than trust that green.
func WatcherWithNetworkFSDetector(fn func(path string) (string, bool)) WatcherOption {
	return func(ws *WatcherService) { ws.networkFS = fn }
}
