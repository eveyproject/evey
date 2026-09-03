package library_test

// A trickplay job for a media item that has been deleted is not a failure.
//
// Prod, 2026-09-02: the trickplay worker logged "trickplay generation failed"
// with two shapes - "get media item: not found", and a foreign-key violation
// (trickplay_manifests_media_item_id_fkey) when the manifest was written. Both
// are the same event: the item was deleted around the job (before it started,
// or during the minutes-long ffmpeg pass). Nothing is wrong; the work simply no
// longer has a subject. It must be a quiet skip, not an error the operator
// reads as a broken generator.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	applib "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrickplay_ItemDeletedBeforeStart_IsASkipNotAnError(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	gen := &countingGen{}
	media := libfakes.NewFakeMediaItemCommandRepo() // empty - the item is gone
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, "/cache", media, bus, nil)

	err := svc.GenerateTrickplay(context.Background(), "media-gone")
	require.NoError(t, err, "a deleted item is not a generation failure")
	assert.Empty(t, gen.Calls(), "nothing to generate for a subject that no longer exists")
}

func TestTrickplay_ItemDeletedDuringGeneration_IsASkipNotAnError(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	// The manifest write races a delete: the FK target vanished mid-generation.
	repo.Err = library.ErrMediaItemGone
	gen := libfakes.NewFakeTrickplayGenerator()
	media := libfakes.NewFakeMediaItemCommandRepo()
	item := &library.MediaItem{ID: "media-1", Title: "Film"}
	item.File = &library.MediaFile{Path: "/media/films/x.mkv"}
	require.NoError(t, media.Create(context.Background(), item))
	bus := sharedfakes.NewFakeEventBus()
	// The service writes a real sidecar under the cache dir before the manifest
	// row: a fixed "/cache" only works where the runner may create it (CI cannot).
	cache := t.TempDir()
	svc := applib.NewTrickplayService(repo, gen, cache, media, bus, nil)

	err := svc.GenerateTrickplay(context.Background(), "media-1")
	require.NoError(t, err, "the item vanishing during the ffmpeg pass is a skip, not a failure")
	assert.Equal(t, filepath.Join(cache, "media-1"), gen.LastOutputDir, "generation did run - the delete landed after it")
}

// A real repo error (not a vanished item) must still surface: we are widening
// "deleted" to a skip, not swallowing every write failure.
func TestTrickplay_RealRepoErrorStillSurfaces(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeTrickplayRepository()
	repo.Err = errors.New("disk full")
	gen := libfakes.NewFakeTrickplayGenerator()
	media := libfakes.NewFakeMediaItemCommandRepo()
	item := &library.MediaItem{ID: "media-2", Title: "Film"}
	item.File = &library.MediaFile{Path: "/media/films/y.mkv"}
	require.NoError(t, media.Create(context.Background(), item))
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewTrickplayService(repo, gen, t.TempDir(), media, bus, nil)

	err := svc.GenerateTrickplay(context.Background(), "media-2")
	require.Error(t, err, "a genuine write failure must not be hidden")
}
