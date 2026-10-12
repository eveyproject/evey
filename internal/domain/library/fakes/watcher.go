package fakes

import (
	"sync"

	"github.com/eveyproject/evey/internal/domain/library"
)

type FakeFileWatcher struct {
	mu       sync.Mutex
	watched  map[string]bool
	events   chan library.FileEvent
	errors   chan error
	closed   bool
	WatchErr error
}

func NewFakeFileWatcher() *FakeFileWatcher {
	return &FakeFileWatcher{
		watched: make(map[string]bool),
		events:  make(chan library.FileEvent, 256),
		errors:  make(chan error, 16),
	}
}

func (w *FakeFileWatcher) Watch(path string) error {
	if w.WatchErr != nil {
		return w.WatchErr
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.watched[path] = true
	return nil
}

func (w *FakeFileWatcher) Unwatch(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.watched, path)
	return nil
}

func (w *FakeFileWatcher) Events() <-chan library.FileEvent {
	return w.events
}

func (w *FakeFileWatcher) Errors() <-chan error {
	return w.errors
}

func (w *FakeFileWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		close(w.events)
		close(w.errors)
	}
	return nil
}

func (w *FakeFileWatcher) SendEvent(ev library.FileEvent) {
	w.events <- ev
}

func (w *FakeFileWatcher) WatchedPaths() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	paths := make([]string, 0, len(w.watched))
	for p := range w.watched {
		paths = append(paths, p)
	}
	return paths
}
