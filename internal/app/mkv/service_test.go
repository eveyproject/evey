package mkv_test

import (
	"context"
	"errors"
	"testing"

	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	mkvfakes "github.com/gravity-zero/evey/internal/domain/mkv/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMKVService() (*appmkv.Service, *mkvfakes.FakeContainerReader, *mkvfakes.FakeContainerWriter) {
	reader := mkvfakes.NewFakeContainerReader()
	writer := mkvfakes.NewFakeContainerWriter()
	return appmkv.NewService(reader, writer), reader, writer
}

func TestGetContainerInfo(t *testing.T) {
	svc, reader, _ := newMKVService()
	reader.Containers["/media/test.mkv"] = &mkv.Container{
		Path:       "/media/test.mkv",
		DurationMs: 120000,
		Tracks: []mkv.Track{
			{ID: 1, Type: mkv.VideoTrack, Codec: "h264"},
			{ID: 2, Type: mkv.AudioTrack, Codec: "aac"},
		},
	}

	info, err := svc.GetContainerInfo(context.Background(), "/media/test.mkv")
	require.NoError(t, err)
	assert.Equal(t, int64(120000), info.DurationMs)
	assert.Len(t, info.Tracks, 2)
}

func TestGetContainerInfo_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _ := newMKVService()

	_, err := svc.GetContainerInfo(context.Background(), "/missing.mkv")
	require.Error(t, err)
}

func TestGetContainerInfo_Error(t *testing.T) {
	t.Parallel()
	svc, reader, _ := newMKVService()
	reader.Err = errors.New("io error")

	_, err := svc.GetContainerInfo(context.Background(), "/media/test.mkv")
	require.Error(t, err)
}

func TestListTracks(t *testing.T) {
	t.Parallel()
	svc, reader, _ := newMKVService()
	reader.Containers["/media/movie.mkv"] = &mkv.Container{
		Path: "/media/movie.mkv",
		Tracks: []mkv.Track{
			{ID: 1, Type: mkv.VideoTrack, Codec: "h265"},
			{ID: 2, Type: mkv.AudioTrack, Codec: "ac3", Language: "en"},
			{ID: 3, Type: mkv.SubtitleTrack, Language: "fr"},
		},
	}

	tracks, err := svc.ListTracks(context.Background(), "/media/movie.mkv")
	require.NoError(t, err)
	assert.Len(t, tracks, 3)
	assert.Equal(t, mkv.VideoTrack, tracks[0].Type)
	assert.Equal(t, mkv.SubtitleTrack, tracks[2].Type)
}

func TestListTracks_NotFound(t *testing.T) {
	t.Parallel()
	svc, _, _ := newMKVService()

	_, err := svc.ListTracks(context.Background(), "/missing.mkv")
	require.Error(t, err)
}

func TestListChapters(t *testing.T) {
	t.Parallel()
	svc, reader, _ := newMKVService()
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
	svc, reader, _ := newMKVService()
	reader.Containers["/media/movie.mkv"] = &mkv.Container{Path: "/media/movie.mkv"}

	chapters, err := svc.ListChapters(context.Background(), "/media/movie.mkv")
	require.NoError(t, err)
	assert.Empty(t, chapters)
}

func TestListAttachments(t *testing.T) {
	t.Parallel()
	svc, reader, _ := newMKVService()
	reader.Containers["/media/movie.mkv"] = &mkv.Container{
		Path: "/media/movie.mkv",
		Attachments: []mkv.Attachment{
			{ID: 1, Name: "cover.jpg", MIMEType: "image/jpeg", Size: 4096},
		},
	}

	attachments, err := svc.ListAttachments(context.Background(), "/media/movie.mkv")
	require.NoError(t, err)
	assert.Len(t, attachments, 1)
	assert.Equal(t, "cover.jpg", attachments[0].Name)
}

func TestExtractAttachment(t *testing.T) {
	t.Parallel()
	svc, _, writer := newMKVService()

	err := svc.ExtractAttachment(context.Background(), "/media/movie.mkv", 1, "/tmp/out")
	require.NoError(t, err)
	assert.Equal(t, []uint64{1}, writer.ExtractAttachmentCalls)
}

func TestExtractAttachment_Error(t *testing.T) {
	t.Parallel()
	svc, _, writer := newMKVService()
	writer.Err = errors.New("extraction failed")

	err := svc.ExtractAttachment(context.Background(), "/media/movie.mkv", 1, "/tmp/out")
	require.Error(t, err)
}

func TestMuxContainer(t *testing.T) {
	t.Parallel()
	svc, _, writer := newMKVService()

	opts := mkv.MuxOptions{
		OutputPath: "/tmp/out.mkv",
		Tracks: []mkv.TrackInput{
			{SourcePath: "/tmp/video.h264", Language: "en"},
		},
	}
	err := svc.MuxContainer(context.Background(), opts)
	require.NoError(t, err)
	require.Len(t, writer.MuxCalls, 1)
	assert.Equal(t, "/tmp/out.mkv", writer.MuxCalls[0].OutputPath)
}

func TestDemuxContainer(t *testing.T) {
	t.Parallel()
	svc, _, writer := newMKVService()

	opts := mkv.DemuxOptions{OutputDir: "/tmp/tracks", TrackIDs: []uint64{1, 2}}
	err := svc.DemuxContainer(context.Background(), "/media/movie.mkv", opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"/media/movie.mkv"}, writer.DemuxCalls)
}

func TestAddTrack(t *testing.T) {
	t.Parallel()
	svc, _, writer := newMKVService()

	input := mkv.TrackInput{SourcePath: "/tmp/sub.srt", Language: "es", Name: "Spanish"}
	err := svc.AddTrack(context.Background(), "/media/movie.mkv", input)
	require.NoError(t, err)
	require.Len(t, writer.AddTrackCalls, 1)
	assert.Equal(t, "es", writer.AddTrackCalls[0].Language)
}

func TestRemoveTrack(t *testing.T) {
	t.Parallel()
	svc, _, writer := newMKVService()

	err := svc.RemoveTrack(context.Background(), "/media/movie.mkv", 3)
	require.NoError(t, err)
	assert.Equal(t, []uint64{3}, writer.RemoveTrackCalls)
}

func TestRemoveTrack_Error(t *testing.T) {
	t.Parallel()
	svc, _, writer := newMKVService()
	writer.Err = errors.New("cannot modify")

	err := svc.RemoveTrack(context.Background(), "/media/movie.mkv", 3)
	require.Error(t, err)
}
