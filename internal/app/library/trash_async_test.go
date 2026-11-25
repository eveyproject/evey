package library

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	mkvfakes "github.com/eveyproject/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

func newTrashSvc(t *testing.T, root string) (*CommandService, *libfakes.FakeMediaItemCommandRepo, *sharedfakes.FakeEventBus, string) {
	t.Helper()
	libQuery := libfakes.NewFakeLibraryQueryRepo()
	libQuery.Seed(&library.Library{ID: "lib1", Name: "Films", Type: "movie", Paths: []string{root}})
	items := libfakes.NewFakeMediaItemCommandRepo()
	bus := sharedfakes.NewFakeEventBus()
	svc := NewCommandService(libfakes.NewFakeLibraryCommandRepo(), libQuery, items, libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(), libfakes.NewFakeFileScanner(),
		libfakes.NewFakeStreamProber(), libfakes.NewFakeSearchIndex(), bus, libfakes.NewFakeChapterRepository(),
		mkvfakes.NewFakeContainerReader(), libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(), "/cache", nil)
	video := filepath.Join(root, "films", "Dune.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(video), 0o755))
	require.NoError(t, os.WriteFile(video, []byte("the whole film"), 0o640))
	require.NoError(t, items.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib1", Type: library.MovieItem,
		Title: "Dune", File: &library.MediaFile{Path: video, Size: 14}}))
	return svc, items, bus, video
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in 5 s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func finished(bus *sharedfakes.FakeEventBus) *library.MediaTrashFinished {
	for _, e := range bus.PublishedEvents() {
		if f, ok := e.(library.MediaTrashFinished); ok {
			return &f
		}
	}
	return nil
}

// A move within one disk is a rename: done before the answer, nothing pending.
func TestTrashMediaItem_SameDisk_DoneInTheRequest(t *testing.T) {
	root := t.TempDir()
	svc, items, _, video := newTrashSvc(t, root)

	res, err := svc.TrashMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.False(t, res.Pending)
	assert.Equal(t, filepath.Join(root, TrashDirName, "films", "Dune.mkv"), res.TrashedPath)
	assert.False(t, exists(video))
	_, gerr := items.GetByID(context.Background(), "m1")
	assert.Error(t, gerr, "the item is gone")
}

// Across disks the answer comes at once, pending; the copy, the item's
// removal and the closing event follow in the background.
func TestTrashMediaItem_AcrossDisks_AnswersAtOnceThenFinishes(t *testing.T) {
	crossDevice(t)
	root := t.TempDir()
	svc, items, bus, video := newTrashSvc(t, root)

	res, err := svc.TrashMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.True(t, res.Pending)
	want := filepath.Join(root, TrashDirName, "films", "Dune.mkv")
	assert.Equal(t, want, res.TrashedPath)

	waitFor(t, func() bool { return finished(bus) != nil })
	f := finished(bus)
	assert.Empty(t, f.Err)
	assert.Equal(t, "Dune", f.Title)
	assert.Equal(t, want, f.TrashedPath)
	assert.True(t, exists(want))
	assert.False(t, exists(video))
	_, gerr := items.GetByID(context.Background(), "m1")
	assert.Error(t, gerr, "the item is gone once the copy is whole")
}

// A background copy that fails keeps the item and the file, and says why.
func TestTrashMediaItem_AcrossDisks_FailureKeepsItemAndSays(t *testing.T) {
	prev := rename
	rename = func(from, to string) error {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EXDEV} // and the partial cannot be renamed either
	}
	t.Cleanup(func() { rename = prev })
	root := t.TempDir()
	svc, items, bus, video := newTrashSvc(t, root)

	res, err := svc.TrashMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.True(t, res.Pending)
	waitFor(t, func() bool { return finished(bus) != nil })
	assert.NotEmpty(t, finished(bus).Err)
	assert.True(t, exists(video))
	_, gerr := items.GetByID(context.Background(), "m1")
	assert.NoError(t, gerr, "the item stays when the file could not move")
}

// The same item asked twice while its copy runs is one move, not two.
func TestTrashMediaItem_AcrossDisks_SecondAskIsTheSameMove(t *testing.T) {
	crossDevice(t)
	root := t.TempDir()
	svc, _, bus, _ := newTrashSvc(t, root)
	block := make(chan struct{})
	svc.trashHold = block // the worker waits here before copying

	a, err := svc.TrashMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	b, err := svc.TrashMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, a.TrashedPath, b.TrashedPath)
	assert.True(t, b.Pending)
	close(block)
	waitFor(t, func() bool { return finished(bus) != nil })
	n := 0
	for _, e := range bus.PublishedEvents() {
		if _, ok := e.(library.MediaTrashFinished); ok {
			n++
		}
	}
	assert.Equal(t, 1, n)
}
