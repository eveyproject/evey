package library

import (
	"context"
	"errors"
	"testing"

	libdomain "github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/mkv"
	mkvfakes "github.com/eveyproject/evey/internal/domain/mkv/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestMetadataService() (*MetadataExtractionService, *libfakes.FakeChapterRepository, *mkvfakes.FakeContainerReader, *libfakes.FakeSubtitleScanner) {
	chapterRepo := libfakes.NewFakeChapterRepository()
	reader := mkvfakes.NewFakeContainerReader()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	subScanner := libfakes.NewFakeSubtitleScanner()

	svc := NewMetadataExtractionService(chapterRepo, reader, streamRepo)
	svc.WithSubtitleScanner(subScanner)
	return svc, chapterRepo, reader, subScanner
}

func TestExtractAndStoreChapters_MKV(t *testing.T) {
	t.Parallel()
	svc, chapterRepo, reader, _ := newTestMetadataService()

	reader.Containers["/media/movie.mkv"] = &mkv.Container{
		Chapters: []mkv.Chapter{
			{ID: 1, Title: "Intro", StartMs: 0, EndMs: 5000},
			{ID: 2, Title: "Main", StartMs: 5000, EndMs: 60000},
		},
	}

	svc.extractAndStoreChapters(context.Background(), "m1", "/media/movie.mkv")

	stored := chapterRepo.All()
	require.Len(t, stored, 2)
	assert.Equal(t, shared.MediaItemID("m1"), stored[0].MediaItemID)
	assert.Equal(t, "Intro", stored[0].Title)
	assert.Equal(t, int64(0), stored[0].StartTimeMs)
	assert.Equal(t, int64(5000), stored[0].EndTimeMs)
	assert.Equal(t, 0, stored[0].SortOrder)
	assert.Equal(t, "Main", stored[1].Title)
	assert.Equal(t, 1, stored[1].SortOrder)
}

func TestExtractAndStoreChapters_NonMKV_Skipped(t *testing.T) {
	t.Parallel()
	svc, chapterRepo, _, _ := newTestMetadataService()

	svc.extractAndStoreChapters(context.Background(), "m1", "/media/movie.mp4")

	assert.Empty(t, chapterRepo.All())
}

func TestExtractAndStoreChapters_UppercaseMKV(t *testing.T) {
	t.Parallel()
	svc, chapterRepo, reader, _ := newTestMetadataService()

	reader.Containers["/media/MOVIE.MKV"] = &mkv.Container{
		Chapters: []mkv.Chapter{
			{ID: 1, Title: "Ch1", StartMs: 0, EndMs: 1000},
		},
	}

	svc.extractAndStoreChapters(context.Background(), "m1", "/media/MOVIE.MKV")
	assert.Len(t, chapterRepo.All(), 1)
}

func TestExtractAndStoreChapters_ReaderError_NoOp(t *testing.T) {
	t.Parallel()
	svc, chapterRepo, reader, _ := newTestMetadataService()
	reader.Err = errors.New("read failed")

	svc.extractAndStoreChapters(context.Background(), "m1", "/media/movie.mkv")
	assert.Empty(t, chapterRepo.All())
}

func TestExtractAndStoreChapters_EmptyChapters_NoOp(t *testing.T) {
	t.Parallel()
	svc, chapterRepo, reader, _ := newTestMetadataService()
	reader.Containers["/media/movie.mkv"] = &mkv.Container{
		Chapters: []mkv.Chapter{},
	}

	svc.extractAndStoreChapters(context.Background(), "m1", "/media/movie.mkv")
	assert.Empty(t, chapterRepo.All())
}

func TestExtractAndStoreChapters_NilChapterRepo_NoOp(t *testing.T) {
	t.Parallel()
	reader := mkvfakes.NewFakeContainerReader()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	svc := NewMetadataExtractionService(nil, reader, streamRepo)

	svc.extractAndStoreChapters(context.Background(), "m1", "/media/movie.mkv")
}

func TestExtractAndStoreChapters_NilReader_NoOp(t *testing.T) {
	t.Parallel()
	chapterRepo := libfakes.NewFakeChapterRepository()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	svc := NewMetadataExtractionService(chapterRepo, nil, streamRepo)

	svc.extractAndStoreChapters(context.Background(), "m1", "/media/movie.mkv")
	assert.Empty(t, chapterRepo.All())
}

func TestExtractAndStoreChapters_RepoError_LogsButNoPanic(t *testing.T) {
	t.Parallel()
	svc, chapterRepo, reader, _ := newTestMetadataService()
	reader.Containers["/media/movie.mkv"] = &mkv.Container{
		Chapters: []mkv.Chapter{{ID: 1, Title: "Ch1", StartMs: 0, EndMs: 1000}},
	}
	chapterRepo.Err = errors.New("db down")

	svc.extractAndStoreChapters(context.Background(), "m1", "/media/movie.mkv")
}

func TestDiscoverExternalSubtitles_Found(t *testing.T) {
	t.Parallel()
	svc, _, _, subScanner := newTestMetadataService()

	subScanner.Results["/media/movie.mkv"] = []libdomain.ExternalSubtitle{
		{Path: "/media/movie.en.srt", Language: "eng", Format: "srt"},
		{Path: "/media/movie.fr.vtt", Language: "fre", Format: "vtt", IsForced: true},
	}

	existing := []*libdomain.MediaStream{
		{StreamIndex: 0, StreamType: "video"},
		{StreamIndex: 1, StreamType: "audio"},
	}

	streams := svc.discoverExternalSubtitles(existing, "m1", "/media/movie.mkv")
	require.Len(t, streams, 2)

	assert.Equal(t, shared.MediaItemID("m1"), streams[0].MediaItemID)
	assert.Equal(t, 2, streams[0].StreamIndex)
	assert.Equal(t, "srt", streams[0].Codec)
	assert.Equal(t, "eng", streams[0].Language)
	assert.True(t, streams[0].IsExternal)
	assert.Equal(t, "/media/movie.en.srt", *streams[0].ExternalPath)
	assert.False(t, streams[0].IsForced)

	assert.Equal(t, 3, streams[1].StreamIndex)
	assert.Equal(t, "webvtt", streams[1].Codec)
	assert.Equal(t, "fre", streams[1].Language)
	assert.True(t, streams[1].IsForced)
}

func TestDiscoverExternalSubtitles_NilScanner(t *testing.T) {
	t.Parallel()
	reader := mkvfakes.NewFakeContainerReader()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	chapterRepo := libfakes.NewFakeChapterRepository()
	svc := NewMetadataExtractionService(chapterRepo, reader, streamRepo)

	streams := svc.discoverExternalSubtitles(nil, "m1", "/media/movie.mkv")
	assert.Nil(t, streams)
}

func TestDiscoverExternalSubtitles_ScannerError(t *testing.T) {
	t.Parallel()
	svc, _, _, subScanner := newTestMetadataService()
	subScanner.Err = errors.New("scan failed")

	streams := svc.discoverExternalSubtitles(nil, "m1", "/media/movie.mkv")
	assert.Nil(t, streams)
}

func TestDiscoverExternalSubtitles_NoResults(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTestMetadataService()

	streams := svc.discoverExternalSubtitles(nil, "m1", "/media/movie.mkv")
	assert.Nil(t, streams)
}

func TestDiscoverExternalSubtitles_MaxIdxFromExisting(t *testing.T) {
	t.Parallel()
	svc, _, _, subScanner := newTestMetadataService()

	subScanner.Results["/media/movie.mkv"] = []libdomain.ExternalSubtitle{
		{Path: "/media/movie.en.srt", Language: "eng", Format: "srt"},
	}
	existing := []*libdomain.MediaStream{
		{StreamIndex: 5, StreamType: "audio"},
	}

	streams := svc.discoverExternalSubtitles(existing, "m1", "/media/movie.mkv")
	require.Len(t, streams, 1)
	assert.Equal(t, 6, streams[0].StreamIndex)
}
