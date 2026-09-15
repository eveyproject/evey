package library_test

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RelocateMediaFile repoints ONLY the path: same bytes, so the fingerprint and
// everything hanging off the item stay as they were.
func TestRelocateMediaFile_ChangesOnlyThePath(t *testing.T) {
	t.Parallel()
	svc, _, _, media, _, _, _, _ := newCmdSvcWithSearch()
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{
		ID: "m1", LibraryID: "lib1", Title: "Film",
		File: &library.MediaFile{Path: "/media/film.mkv", Fingerprint: "fp-1", Size: 42},
	}))

	require.NoError(t, svc.RelocateMediaFile(context.Background(), "m1", "/media/film.mp4"))

	got, err := media.GetByID(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "/media/film.mp4", got.File.Path)
	assert.Equal(t, "fp-1", got.File.Fingerprint, "same bytes, same fingerprint")
	assert.EqualValues(t, 42, got.File.Size)
	assert.Equal(t, "Film", got.Title)
	assert.Equal(t, []string{"m1"}, media.UpdatedIDs)
}

func TestRelocateMediaFile_Refusals(t *testing.T) {
	t.Parallel()
	svc, _, _, media, _, _, _, _ := newCmdSvcWithSearch()
	// Unknown item.
	require.Error(t, svc.RelocateMediaFile(context.Background(), "ghost", "/media/x.mp4"))
	// An item without a file has nothing to relocate.
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{ID: "series", LibraryID: "lib1", Title: "Series"}))
	require.Error(t, svc.RelocateMediaFile(context.Background(), "series", "/media/x.mp4"))
	// A refused write is reported, and nothing pretends it happened.
	require.NoError(t, media.Create(context.Background(), &library.MediaItem{ID: "m2", LibraryID: "lib1", File: &library.MediaFile{Path: "/media/a.mkv"}}))
	media.ErrUpdate = errors.New("db down")
	require.Error(t, svc.RelocateMediaFile(context.Background(), "m2", "/media/a.mp4"))
	got, _ := media.GetByID(context.Background(), "m2")
	assert.Equal(t, "/media/a.mkv", got.File.Path)
}
