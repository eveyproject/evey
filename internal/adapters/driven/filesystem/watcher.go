package filesystem

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/gravity-zero/evey/internal/domain/library"
)

type FSWatcher struct {
	watcher *fsnotify.Watcher
	events  chan library.FileEvent
	errors  chan error
	done    chan struct{}
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
						slog.Warn("watchRecursive", slog.Any("err", err))
					}
					fw.events <- library.FileEvent{Path: ev.Name, Op: library.FileCreated}
					continue
				}
			}
			ext := strings.ToLower(filepath.Ext(ev.Name))
			if !videoExtensions[ext] {
				continue
			}
			fw.events <- library.FileEvent{Path: ev.Name, Op: op}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.errors <- err
		}
	}
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

func (fw *FSWatcher) Watch(path string) error {
	return fw.watchRecursive(path)
}

func (fw *FSWatcher) watchRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			slog.Debug("watcher: walk error", slog.String("path", path), slog.Any("err", err))
			return nil
		}
		if d.IsDir() {
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return fw.watcher.Add(path)
		}
		return nil
	})
}

func (fw *FSWatcher) Unwatch(path string) error {
	// Remove root + all subdirectories (Watch adds them recursively)
	var firstErr error
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if rmErr := fw.watcher.Remove(p); rmErr != nil && firstErr == nil {
				firstErr = rmErr
			}
		}
		return nil
	})
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
