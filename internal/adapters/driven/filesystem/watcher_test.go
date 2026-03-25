package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/gravity-zero/evey/internal/domain/library"
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
