package mkv_test

import (
	"context"
	"testing"

	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	mkvfakes "github.com/gravity-zero/evey/internal/domain/mkv/fakes"
	"github.com/stretchr/testify/assert"
)

func TestGetContainerInfo(t *testing.T) {
	reader := mkvfakes.NewFakeContainerReader()
	writer := mkvfakes.NewFakeContainerWriter()

	reader.Containers["/media/test.mkv"] = &mkv.Container{
		Path:       "/media/test.mkv",
		DurationMs: 120000,
		Tracks: []mkv.Track{
			{ID: 1, Type: mkv.VideoTrack, Codec: "h264"},
			{ID: 2, Type: mkv.AudioTrack, Codec: "aac"},
		},
	}

	svc := appmkv.NewService(reader, writer)
	info, err := svc.GetContainerInfo(context.Background(), "/media/test.mkv")
	assert.NoError(t, err)
	assert.Equal(t, int64(120000), info.DurationMs)
	assert.Len(t, info.Tracks, 2)
}
