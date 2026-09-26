package mkv_test

import (
	"context"
	"testing"

	appmkv "github.com/evey-project/evey/internal/app/mkv"
	"github.com/evey-project/evey/internal/domain/mkv"
	mkvfakes "github.com/evey-project/evey/internal/domain/mkv/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMKVService() (*appmkv.Service, *mkvfakes.FakeContainerReader) {
	reader := mkvfakes.NewFakeContainerReader()
	return appmkv.NewService(reader), reader
}

func TestListChapters(t *testing.T) {
	t.Parallel()
	svc, reader := newMKVService()
	reader.Containers["/media/movie.mkv"] = &mkv.Container{
		Path: "/media/movie.mkv",
		Chapters: []mkv.Chapter{
			{ID: 1, Title: "Opening", StartMs: 0, EndMs: 90000},
			{ID: 2, Title: "Act 1", StartMs: 90000, EndMs: 3600000},
		},
	}

	chapters, err := svc.ListChapters(context.Background(), "/media/movie.mkv")
	require.NoError(t, err)
	assert.Len(t, chapters, 2)
	assert.Equal(t, "Opening", chapters[0].Title)
}

func TestListChapters_Empty(t *testing.T) {
	t.Parallel()
	svc, reader := newMKVService()
	reader.Containers["/media/movie.mkv"] = &mkv.Container{Path: "/media/movie.mkv"}

	chapters, err := svc.ListChapters(context.Background(), "/media/movie.mkv")
	require.NoError(t, err)
	assert.Empty(t, chapters)
}
