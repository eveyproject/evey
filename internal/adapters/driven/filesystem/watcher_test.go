package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapOp(t *testing.T) {
	fw := &FSWatcher{}
	tests := []struct {
		name string
		op   fsnotify.Op
		want library.FileOp
	}{
		{"create", fsnotify.Create, library.FileCreated},
		{"write", fsnotify.Write, library.FileModified},
		{"remove", fsnotify.Remove, library.FileRemoved},
		{"rename", fsnotify.Rename, library.FileRenamed},
		{"chmod (ignored)", fsnotify.Chmod, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fw.mapOp(tt.op))
		})
	}
}

func TestNewFSWatcher_And_Close(t *testing.T) {
	fw, err := NewFSWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	assert.NotNil(t, fw.Events())
	assert.NotNil(t, fw.Errors())
	assert.NoError(t, fw.Close())
}

func TestFSWatcher_Watch_And_Unwatch(t *testing.T) {
	fw, err := NewFSWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	defer fw.Close()

	dir := t.TempDir()
	require.NoError(t, fw.Watch(dir))
	require.NoError(t, fw.Unwatch(dir))
}

func TestFSWatcher_Watch_SkipsHiddenDirs(t *testing.T) {
	fw, err := NewFSWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	defer fw.Close()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".hidden"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "visible"), 0755))

	require.NoError(t, fw.Watch(dir))
}

// Prod review 2026-08-26: the event filter only knew videoExtensions, so a
// created .mp3 or .jpg NEVER produced an event - realtime detection was
// structurally dead for music and photo libraries even with monitoring ON.
// The watcher watches whole library roots; it must speak every media type
// the scanners ingest.
func TestFSWatcher_ForwardsAudioAndImageEvents(t *testing.T) {
	fw, err := NewFSWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	defer fw.Close()

	dir := t.TempDir()
	require.NoError(t, fw.Watch(dir))

	for _, name := range []string{"song.mp3", "cover.jpg", "movie.mkv"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}

	got := map[string]bool{}
	timeout := time.After(3 * time.Second)
	for len(got) < 3 {
		select {
		case ev := <-fw.Events():
			got[filepath.Base(ev.Path)] = true
		case <-timeout:
			t.Fatalf("timed out; events seen: %v (audio/image events are being dropped)", got)
		}
	}
	assert.True(t, got["song.mp3"], "an audio file must produce an event")
	assert.True(t, got["cover.jpg"], "an image file must produce an event")
	assert.True(t, got["movie.mkv"])
}

// cancelAfter reports a live context for the first n Err calls and Canceled
// after, to stop a walk part-way through without timing.
type cancelAfter struct {
	context.Context
	left int
}

func (c *cancelAfter) Err() error {
	if c.left > 0 {
		c.left--
		return nil
	}
	return context.Canceled
}

func TestFSWatcher_WatchContext_CancelledWalkLeavesNoWatchBehind(t *testing.T) {
	fw, err := NewFSWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	defer fw.Close()

	dir := t.TempDir()
	for _, sub := range []string{"a", "b", "c", "d"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, sub), 0o755))
	}

	err = fw.WatchContext(&cancelAfter{Context: context.Background(), left: 3}, dir)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, fw.watcher.WatchList(), "a failed root must not keep the watches it added")
}

func TestFSWatcher_WatchContext_CompletesWithALiveContext(t *testing.T) {
	fw, err := NewFSWatcher()
	if err != nil {
		t.Skipf("fsnotify not available: %v", err)
	}
	defer fw.Close()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a"), 0o755))
	require.NoError(t, fw.WatchContext(context.Background(), dir))
	assert.Len(t, fw.watcher.WatchList(), 2)
}

func TestFSWatcher_QueueOverflowIsReportedAsOverflow(t *testing.T) {
	err := wrapWatchError(fsnotify.ErrEventOverflow)
	var o interface{ Overflow() bool }
	require.ErrorAs(t, err, &o)
	assert.True(t, o.Overflow())
	require.ErrorIs(t, err, fsnotify.ErrEventOverflow)

	plain := wrapWatchError(errors.New("boom"))
	assert.False(t, errors.As(plain, &o) && o.Overflow())
}
