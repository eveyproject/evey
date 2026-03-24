package library

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

const debounceDelay = 2 * time.Second

// LibraryScanner is the subset of Service needed by the watcher.
type LibraryScanner interface {
	ScanLibrary(ctx context.Context, libraryID string) (added, updated int, err error)
}

type WatcherService struct {
	watcher   library.FileWatcher
	libraries library.LibraryRepository
	scanner   LibraryScanner
	log       *slog.Logger
	bus       shared.EventBus

	mu            sync.Mutex
	pathToLib     map[string]string // watched path → library ID
	failedLibs    map[string]bool   // library IDs where inotify failed → need periodic scan
	cancel        context.CancelFunc
	ctx           context.Context
}

func NewWatcherService(
	watcher library.FileWatcher,
	libraries library.LibraryRepository,
	scanner LibraryScanner,
	log *slog.Logger,
) *WatcherService {
	return &WatcherService{
		watcher:    watcher,
		libraries:  libraries,
		scanner:    scanner,
		log:        log,
		pathToLib:  make(map[string]string),
		failedLibs: make(map[string]bool),
	}
}

func (ws *WatcherService) WithEventBus(bus shared.EventBus) *WatcherService {
	ws.bus = bus
	return ws
}

// Start watches all libraries with RealtimeMonitoring=true and processes FS events.
func (ws *WatcherService) Start(ctx context.Context) error {
	ctx, ws.cancel = context.WithCancel(ctx)
	ws.ctx = ctx

	libs, err := ws.libraries.List(ctx)
	if err != nil {
		return err
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
				lib, err := ws.libraries.GetByID(ctx, e.LibraryID)
				if err == nil {
					ws.AddLibrary(lib)
				}
			}
		})
		ws.bus.Subscribe("library.deleted", func(_ context.Context, ev shared.Event) {
			if e, ok := ev.(library.LibraryDeleted); ok {
				ws.RemoveLibraryByID(e.LibraryID)
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
		ws.pathToLib[p] = lib.ID
		ws.log.Info("watcher: watching", slog.String("path", p), slog.String("library", lib.Name))
	}
	if allFailed && len(lib.Paths) > 0 {
		ws.failedLibs[lib.ID] = true
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

// RemoveLibraryByID stops watching all paths for a library by ID (used when lib is already deleted).
func (ws *WatcherService) RemoveLibraryByID(libraryID string) {
	ws.mu.Lock()
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
			libID := ws.resolveLibrary(ev.Path)
			if libID == "" {
				continue
			}
			ws.log.Debug("watcher: file event", slog.String("path", ev.Path), slog.Int("op", int(ev.Op)), slog.String("library_id", libID))

			mu.Lock()
			if t, exists := pending[libID]; exists {
				t.Reset(debounceDelay)
			} else {
				pending[libID] = time.AfterFunc(debounceDelay, func() {
					ws.triggerScan(ctx, libID)
					mu.Lock()
					delete(pending, libID)
					mu.Unlock()
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

func (ws *WatcherService) triggerScan(ctx context.Context, libraryID string) {
	ws.log.Info("watcher: triggering scan", slog.String("library_id", libraryID))
	added, updated, err := ws.scanner.ScanLibrary(ctx, libraryID)
	if err != nil {
		ws.log.Error("watcher: scan failed", slog.String("library_id", libraryID), slog.Any("err", err))
		return
	}
	ws.log.Info("watcher: scan complete", slog.String("library_id", libraryID), slog.Int("added", added), slog.Int("updated", updated))
}
