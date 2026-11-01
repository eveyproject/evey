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

// defaultWalkTimeout bounds the recursive watch registration of ONE library
// root. A hung network mount would otherwise block the walk forever.
const defaultWalkTimeout = 5 * time.Minute

// contextWatcher is the optional, cancellable form of FileWatcher.Watch. An
// adapter that implements it lets the service bound a root's walk; one that
// does not is called through Watch as before.
type contextWatcher interface {
	WatchContext(ctx context.Context, path string) error
}

// overflowSignal is implemented by the errors a watcher reports when the
// kernel event queue overflowed: events were dropped, so only a scan can
// recover what they described.
type overflowSignal interface{ Overflow() bool }

func isOverflow(err error) bool {
	var o overflowSignal
	return errors.As(err, &o) && o.Overflow()
}

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
	// walkTimeout bounds one root's watch registration. Injectable for tests.
	walkTimeout time.Duration
	// gen counts removals per library so a walk that started before a removal
	// or re-arm (the walk runs outside mu) can tell its result is stale.
	gen map[string]uint64
	// walking maps the roots being registered right now to their library, so an
	// event raised under a half-registered root is still routed instead of
	// dropped for the minutes a big walk can take.
	walking map[string]string
	// initialDone is closed once the boot-time watch of every library finished.
	initialDone chan struct{}
}

type WatcherOption func(*WatcherService)

func WatcherWithEventBus(bus shared.EventBus) WatcherOption {
	return func(ws *WatcherService) { ws.bus = bus }
}

// WatcherWithWalkTimeout overrides the per-root bound on watch registration.
func WatcherWithWalkTimeout(d time.Duration) WatcherOption {
	return func(ws *WatcherService) {
		if d > 0 {
			ws.walkTimeout = d
		}
	}
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
		walkTimeout:   defaultWalkTimeout,
		gen:           make(map[string]uint64),
		walking:       make(map[string]string),
		initialDone:   make(chan struct{}),
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
	// Walking every root can take minutes on a large or remote library, and
	// boot must not wait for it. Until a library's watch is recorded it is
	// absent from AliveWatchedLibraries, so the periodic scan keeps covering it.
	var wanted []pendingWatch
	for _, lib := range libs {
		if lib.RealtimeMonitoring && lib.Enabled {
			wanted = append(wanted, pendingWatch{lib: lib, gen: ws.beginWatch(lib)})
		}
	}
	go ws.watchInitial(ctx, wanted)

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

type pendingWatch struct {
	lib *library.Library
	gen uint64
}

// beginWatch marks the library's roots as being registered and returns the
// generation the result must still match when it is recorded.
func (ws *WatcherService) beginWatch(lib *library.Library) uint64 {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	for _, p := range lib.Paths {
		ws.walking[p] = string(lib.ID)
	}
	return ws.gen[string(lib.ID)]
}

// watchInitial registers the boot-time watches in the background.
func (ws *WatcherService) watchInitial(ctx context.Context, wanted []pendingWatch) {
	defer close(ws.initialDone)
	defer func() {
		if r := recover(); r != nil {
			ws.log.Error("watcher: initial watch panicked", slog.Any("panic", r))
		}
	}()
	for _, w := range wanted {
		if ctx.Err() != nil {
			return
		}
		ws.watchLibrary(w.lib, w.gen)
	}
}

// WaitInitialWatch blocks until the boot-time watch of every library finished.
func (ws *WatcherService) WaitInitialWatch() { <-ws.initialDone }

// watchRoot registers one root, bounded by walkTimeout. On any failure the
// adapter has already dropped the watches this root added.
func (ws *WatcherService) watchRoot(path string) error {
	base := ws.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, ws.walkTimeout)
	defer cancel()
	if cw, ok := ws.watcher.(contextWatcher); ok {
		return cw.WatchContext(ctx, path)
	}
	return ws.watcher.Watch(path)
}

func (ws *WatcherService) watchLibrary(lib *library.Library, gen uint64) {
	libID := string(lib.ID)

	// The walk runs without mu: it can take minutes, and holding the lock would
	// stall event routing, the probe and RemoveLibraryByID for that long.
	var watched []string
	anyFailed := false
	for _, p := range lib.Paths {
		if err := ws.watchRoot(p); err != nil {
			ws.log.Warn("watcher: inotify failed for path, will use periodic scan", slog.String("path", p), slog.String("library", lib.Name), slog.Any("err", err))
			anyFailed = true
			continue
		}
		watched = append(watched, p)
	}

	ws.mu.Lock()
	for _, p := range lib.Paths {
		if ws.walking[p] == libID {
			delete(ws.walking, p)
		}
	}
	if ws.gen[libID] != gen {
		// Removed or re-armed while walking: this result describes paths nobody
		// asked to watch any more. Keep a path only if the newer watch owns it.
		var stale []string
		for _, p := range watched {
			if _, owned := ws.pathToLib[p]; !owned {
				stale = append(stale, p)
			}
		}
		ws.mu.Unlock()
		for _, p := range stale {
			if err := ws.watcher.Unwatch(p); err != nil {
				ws.log.Warn("watcher: unwatch failed", slog.String("path", p), slog.Any("err", err))
			}
		}
		return
	}
	for _, p := range watched {
		ws.pathToLib[p] = libID
		ws.noWatchLogged = false
		ws.log.Info("watcher: watching", slog.String("path", p), slog.String("library", lib.Name))
	}
	// A library is only as watched as its worst root: one blind root hides
	// changes the others never show, so it keeps the frequent periodic scan.
	if anyFailed {
		ws.failedLibs[libID] = true
		ws.log.Info("watcher: library needs periodic scan fallback", slog.String("library", lib.Name))
	}
	ws.mu.Unlock()
}

// AddLibrary registers a library for watching at runtime (e.g. after creation).
func (ws *WatcherService) AddLibrary(lib *library.Library) {
	if !lib.RealtimeMonitoring || !lib.Enabled {
		return
	}
	ws.watchLibrary(lib, ws.beginWatch(lib))
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
	ws.gen[libraryID]++
	for p, id := range ws.walking {
		if id == libraryID {
			delete(ws.walking, p)
		}
	}
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

	arm := func(libID string) {
		mu.Lock()
		defer mu.Unlock()
		if t, exists := pending[libID]; exists {
			t.Reset(ws.debounceDelay)
			return
		}
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

			arm(libID)

		case err, ok := <-ws.watcher.Errors():
			if !ok {
				return
			}
			if isOverflow(err) {
				// Dropped events cannot be attributed to a library, so every
				// watched one is rescanned through the usual debounce.
				ws.log.Warn("watcher: event queue overflowed, rescanning watched libraries", slog.Any("err", err))
				for _, libID := range ws.watchedLibraryIDs() {
					arm(libID)
				}
				continue
			}
			ws.log.Warn("watcher: error", slog.Any("err", err))
		}
	}
}

func (ws *WatcherService) watchedLibraryIDs() []string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	seen := make(map[string]bool, len(ws.pathToLib))
	var ids []string
	for _, id := range ws.pathToLib {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// resolveLibrary finds which library owns the given file path.
func (ws *WatcherService) resolveLibrary(filePath string) string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if id := resolveIn(ws.pathToLib, filePath); id != "" {
		return id
	}
	return resolveIn(ws.walking, filePath)
}

func resolveIn(roots map[string]string, filePath string) string {
	for watchedPath, libID := range roots {
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
