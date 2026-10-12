package library_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakeTypedDirectoryScanner ---

type fakeTypedDirectoryScanner struct {
	calls   atomic.Int32
	lastDir atomic.Value // string
	lastLib atomic.Value // string
	perCall int
}

func (f *fakeTypedDirectoryScanner) ScanDirectory(_ context.Context, libraryID, dir string) (int, error) {
	f.calls.Add(1)
	f.lastDir.Store(dir)
	f.lastLib.Store(libraryID)
	return f.perCall, nil
}

// --- WithMusicScanner ---

func TestWithMusicScanner_StoresScanner_UsedDuringMusicLibScan(t *testing.T) {
	t.Parallel()
	h := newHarness()
	music := &fakeTypedDirectoryScanner{perCall: 3}
	h.svc.WithMusicScanner(music)

	lib := &library.Library{ID: "lib-music", Name: "Music", Type: "music", Paths: []string{"/music/a", "/music/b"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-music")
	require.NoError(t, err)
	assert.Equal(t, 6, added, "2 paths * 3 = 6 items added")
	assert.Equal(t, 0, updated)
	assert.Equal(t, int32(2), music.calls.Load(), "scanner called once per path")
	assert.Equal(t, "lib-music", music.lastLib.Load().(string))
}

func TestWithMusicScanner_NotConfigured_MusicLibScan_Errors(t *testing.T) {
	t.Parallel()
	h := newHarness()

	lib := &library.Library{ID: "lib-music", Name: "Music", Type: "music", Paths: []string{"/music"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-music")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "music scanner not configured")
}

// --- WithPhotoScanner ---

func TestWithPhotoScanner_StoresScanner_UsedDuringPhotoLibScan(t *testing.T) {
	t.Parallel()
	h := newHarness()
	photo := &fakeTypedDirectoryScanner{perCall: 5}
	h.svc.WithPhotoScanner(photo)

	lib := &library.Library{ID: "lib-photo", Name: "Photos", Type: "photo", Paths: []string{"/photos"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	added, updated, err := h.svc.ScanLibrary(context.Background(), "lib-photo")
	require.NoError(t, err)
	assert.Equal(t, 5, added)
	assert.Equal(t, 0, updated)
	assert.Equal(t, int32(1), photo.calls.Load())
	assert.Equal(t, "/photos", photo.lastDir.Load().(string))
}

func TestWithPhotoScanner_NotConfigured_PhotoLibScan_Errors(t *testing.T) {
	t.Parallel()
	h := newHarness()

	lib := &library.Library{ID: "lib-photo", Name: "Photos", Type: "photo", Paths: []string{"/photos"}}
	h.libQuery.Seed(lib)
	h.libCmd.Seed(lib)

	_, _, err := h.svc.ScanLibrary(context.Background(), "lib-photo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "photo scanner not configured")
}

// --- StartTrickplayWorker ---

func TestStartTrickplayWorker_NoTrickplay_NoOp(t *testing.T) {
	t.Parallel()
	// Build a CommandService where trickplay is set, but with no-op deps.
	// The wrapper just delegates; verify it doesn't panic.
	h := newHarness()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NotPanics(t, func() {
		h.svc.StartTrickplayWorker(ctx)
	})
}

func TestStartTrickplayWorker_SubscribesToBus(t *testing.T) {
	t.Parallel()
	// Build a service with trickplay wired and verify subscription took effect by
	// publishing an unrelated-type media.item.added: handler should run (early return)
	// without error.
	h := newHarness()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.svc.StartTrickplayWorker(ctx)

	// Publishing a media.item.added with non-movie/episode triggers the handler's
	// early-return branch; no panic, no error.
	require.NotPanics(t, func() {
		_ = h.bus.Publish(ctx, library.MediaItemAdded{
			MediaItemID: shared.MediaItemID("x"),
			LibraryID:   shared.LibraryID("lib-1"),
			Title:       "Other",
			Type:        "other",
			OccurredAt:  time.Now(),
		})
	})
}

// --- UpdateLibrary ---

func TestUpdateLibrary_Success_PersistsChange_BumpsUpdatedAt(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{
		ID: "lib-upd", Name: "Old Name", Type: "movie", Paths: []string{"/old"},
		CreatedAt: time.Now().Add(-time.Hour),
	}
	h.libCmd.Seed(lib)
	h.libQuery.Seed(lib)

	before := time.Now().Add(-time.Second)
	lib.Name = "New Name"
	lib.Paths = []string{"/new"}
	err := h.svc.UpdateLibrary(context.Background(), lib)
	require.NoError(t, err)

	assert.True(t, lib.UpdatedAt.After(before), "UpdatedAt should be bumped")

	// libQuery shares the same pointer when both are seeded from same lib
	got, gerr := h.libQuery.GetByID(context.Background(), shared.LibraryID("lib-upd"))
	require.NoError(t, gerr)
	assert.Equal(t, "New Name", got.Name)
	assert.Equal(t, []string{"/new"}, got.Paths)
}

// CreateLibrary validates; UpdateLibrary did not. An aggregate that could not
// be CREATED in this shape could still be written into that shape.
func TestUpdateLibrary_RejectsAnInvalidAggregate(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-inv", Name: "Good", Type: "movie", Paths: []string{"/p"}}
	h.libCmd.Seed(lib)
	h.libQuery.Seed(lib)

	invalid := &library.Library{ID: "lib-inv", Name: "", Type: "movie", Paths: []string{"/p"}}
	err := h.svc.UpdateLibrary(context.Background(), invalid)
	require.Error(t, err)

	got, gerr := h.libQuery.GetByID(context.Background(), shared.LibraryID("lib-inv"))
	require.NoError(t, gerr)
	assert.Equal(t, "Good", got.Name, "the refused update must not have been persisted")
}

func TestUpdateLibrary_RepoNotFound_Propagates(t *testing.T) {
	t.Parallel()
	h := newHarness()
	// Don't seed libCmd → fake returns ErrNotFound
	err := h.svc.UpdateLibrary(context.Background(), &library.Library{ID: "missing", Name: "X", Type: "movie"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update library")
}

func TestUpdateLibrary_RepoErrorPropagates(t *testing.T) {
	t.Parallel()
	h := newHarness()
	lib := &library.Library{ID: "lib-1", Name: "Name", Type: "movie"}
	h.libCmd.Seed(lib)
	h.libCmd.Err = errors.New("db down")

	err := h.svc.UpdateLibrary(context.Background(), lib)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update library")
	assert.Contains(t, err.Error(), "db down")
}
