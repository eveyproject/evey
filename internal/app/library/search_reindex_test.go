package library_test

import (
	"context"
	"errors"
	"testing"

	applibrary "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedItem(repo *libfakes.FakeMediaItemQueryRepo, id, libID string) {
	repo.Seed(&library.MediaItem{
		ID:        shared.MediaItemID(id),
		LibraryID: shared.LibraryID(libID),
		Title:     id,
	})
}

func TestReindexSearchIndex_AllLibraries(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	seedItem(repo, "m3", "libB")
	search := libfakes.NewFakeSearchIndex()

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Equal(t, 3, search.Indexed(), "every item across libraries indexed")
}

func TestReindexSearchIndex_SingleLibrary(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	seedItem(repo, "m3", "libB")
	search := libfakes.NewFakeSearchIndex()

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, "libA", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "only libA items counted")
	assert.Equal(t, 2, search.Indexed(), "only libA items indexed")
}

func TestReindexSearchIndex_ProgressCallback(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	seedItem(repo, "m2", "libA")
	search := libfakes.NewFakeSearchIndex()

	var lastDone, lastTotal int
	calls := 0
	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, "", func(done, total int) {
		calls++
		lastDone, lastTotal = done, total
	})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.GreaterOrEqual(t, calls, 1)
	assert.Equal(t, 2, lastDone)
	assert.Equal(t, 2, lastTotal, "total reported matches the dataset")
}

func TestReindexSearchIndex_Empty(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	search := libfakes.NewFakeSearchIndex()

	n, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Equal(t, 0, search.Indexed())
}

func TestReindexSearchIndex_ListErrorPropagates(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	repo.Err = errors.New("db down")
	search := libfakes.NewFakeSearchIndex()

	_, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, "", nil)
	require.Error(t, err)
}

func TestReindexSearchIndex_IndexErrorPropagates(t *testing.T) {
	t.Parallel()
	repo := libfakes.NewFakeMediaItemQueryRepo()
	seedItem(repo, "m1", "libA")
	search := libfakes.NewFakeSearchIndex()
	search.Err = errors.New("meili down")

	_, err := applibrary.ReindexSearchIndex(context.Background(), repo, search, "", nil)
	require.Error(t, err)
}
