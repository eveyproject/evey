package filesystem

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/fsnotify/fsnotify"
)

type FSWatcher struct {
	watcher *fsnotify.Watcher
	events  chan library.FileEvent
	errors  chan error
	done    chan struct{}

	// linked maps a watched root's resolved target to the root as it was
	// configured, for roots that are symbolic links. The kernel names events
	// by the target; the catalog, the pruner and the root checks know the
	// configured path, so events are handed out under that one.
	mu     sync.RWMutex
	linked map[string]string
}

func NewFSWatcher() (*FSWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	fw := &FSWatcher{
		watcher: w,
		events:  make(chan library.FileEvent, 256),
		errors:  make(chan error, 16),
		done:    make(chan struct{}),
		linked:  map[string]string{},
	}
	go fw.loop()
	return fw, nil
}

func (fw *FSWatcher) loop() {
	defer close(fw.events)
	defer close(fw.errors)
	for {
		select {
		case <-fw.done:
			return
		case ev, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			op := fw.mapOp(ev.Op)
			if op < 0 {
				continue
			}
			// Directories: register for recursive watch AND trigger scan
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					if err := fw.watchRecursive(ev.Name); err != nil {
						slog.Warn("watcher: new directory not watched", slog.String("path", ev.Name), slog.Any("err", err))
					}
					fw.events <- library.FileEvent{Path: fw.configuredPath(ev.Name), Op: library.FileCreated}
					continue
				}
			}
			// Liveness-probe files are an internal protocol between the app's
			// prober and this adapter. They deliberately carry no media
			// extension (so the scanner ignores them), which means the filter
			// below would drop them - and every probe would time out, marking
			// healthy watches dead and sending the server into constant full
			// scans. Forward them before filtering.
			if library.IsProbeFile(ev.Name) {
				fw.events <- library.FileEvent{Path: fw.configuredPath(ev.Name), Op: op}
				continue
			}
			// The watcher watches whole library roots, so it must speak every
			// media type the scanners ingest: video AND audio AND image. This
			// filter only knew video until 2026-08-26 - a created .mp3/.jpg
			// never produced an event, and realtime detection was structurally
			// dead for music and photo libraries even with monitoring ON.
			if !isWatchableMediaFile(ev.Name) {
				continue
			}
			fw.events <- library.FileEvent{Path: fw.configuredPath(ev.Name), Op: op}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.errors <- wrapWatchError(err)
		}
	}
}

// isWatchableMediaFile reports whether a path carries an extension one of the
// three scanners (video, music, photo) would ingest. The maps live next to
// their scanners in this same package, so the three lists cannot drift apart
// from what a scan would actually pick up.
func isWatchableMediaFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if videoExtensions[ext] {
		return true
	}
	if _, ok := audioExts[ext]; ok {
		return true
	}
	_, ok := imageExts[ext]
	return ok
}

func (fw *FSWatcher) mapOp(op fsnotify.Op) library.FileOp {
	switch {
	case op.Has(fsnotify.Create):
		return library.FileCreated
	case op.Has(fsnotify.Write):
		return library.FileModified
	case op.Has(fsnotify.Remove):
		return library.FileRemoved
	case op.Has(fsnotify.Rename):
		return library.FileRenamed
	}
	return -1
}

// overflowError marks the kernel event queue overflowing: events were dropped,
// so the consumer must rescan. The app layer recognises it by the Overflow
// method alone, without importing fsnotify.
type overflowError struct{ error }

func (e overflowError) Unwrap() error  { return e.error }
func (e overflowError) Overflow() bool { return true }

func wrapWatchError(err error) error {
	if errors.Is(err, fsnotify.ErrEventOverflow) {
		return overflowError{err}
	}
	return err
}

func (fw *FSWatcher) Watch(path string) error {
	return fw.WatchContext(context.Background(), path)
}

// WatchContext registers root and its subdirectories, stopping when ctx ends
// (a hung network mount would otherwise block the walk forever). If it fails
// part-way - ctx done, or the inotify limit hit - the watches it already added
// are removed: a half-watched root would look established while going blind.
func (fw *FSWatcher) WatchContext(ctx context.Context, root string) error {
	return fw.watchRecursiveCtx(ctx, root)
}

func (fw *FSWatcher) watchRecursive(root string) error {
	return fw.watchRecursiveCtx(context.Background(), root)
}

// resolveRoot follows a root that is a symbolic link to where the watches
// must be set, and remembers the pair so events come back under the root.
// A root that cannot be resolved (missing, dead mount) is kept as it is: the
// walk reports it the way it always did.
func (fw *FSWatcher) resolveRoot(root string) string {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved == root {
		return root
	}
	fw.mu.Lock()
	fw.linked[resolved] = root
	fw.mu.Unlock()
	return resolved
}

// configuredPath turns a path the kernel names under a link's target back
// into the same path under the configured root.
func (fw *FSWatcher) configuredPath(name string) string {
	fw.mu.RLock()
	defer fw.mu.RUnlock()
	for target, root := range fw.linked {
		if name == target {
			return root
		}
		if strings.HasPrefix(name, target+string(filepath.Separator)) {
			return filepath.Join(root, name[len(target):])
		}
	}
	return name
}

func (fw *FSWatcher) watchRecursiveCtx(ctx context.Context, root string) error {
	var added []string
	err := filepath.WalkDir(fw.resolveRoot(root), func(path string, d os.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			slog.Debug("watcher: walk error", slog.String("path", path), slog.Any("err", err))
			return nil
		}
		if d.IsDir() {
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if addErr := fw.watcher.Add(path); addErr != nil {
				return addErr
			}
			added = append(added, path)
		}
		return nil
	})
	if err != nil {
		for _, p := range added {
			if rmErr := fw.watcher.Remove(p); rmErr != nil {
				slog.Debug("watchRecursive: drop watch of a failed root", slog.String("path", p), slog.Any("err", rmErr))
			}
		}
	}
	return err
}

func (fw *FSWatcher) Unwatch(path string) error {
	// Remove root + all subdirectories (Watch adds them recursively), through
	// the link when the root is one.
	resolved := fw.resolveRoot(path)
	defer func() {
		fw.mu.Lock()
		delete(fw.linked, resolved)
		fw.mu.Unlock()
	}()
	var firstErr error
	if walkErr := filepath.WalkDir(resolved, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if rmErr := fw.watcher.Remove(p); rmErr != nil && firstErr == nil {
				firstErr = rmErr
			}
		}
		return nil
	}); walkErr != nil && firstErr == nil {
		firstErr = walkErr
	}
	if firstErr != nil {
		return firstErr
	}
	return nil
}

func (fw *FSWatcher) Events() <-chan library.FileEvent {
	return fw.events
}

func (fw *FSWatcher) Errors() <-chan error {
	return fw.errors
}

func (fw *FSWatcher) Close() error {
	close(fw.done)
	return fw.watcher.Close()
}
