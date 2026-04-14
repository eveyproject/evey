package postgres_test

import (
	"context"
	"testing"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int             { return &v }
func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }
func stringPtr(v string) *string    { return &v }

func TestMediaStreamRepo_CreateBatch_GetByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mslib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("msmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	streams := []*library.MediaStream{
		{
			ID: "s1", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video",
			Codec: "h264", CodecLongName: "H.264", Profile: "High", Language: "und",
			IsDefault: true, Width: intPtr(1920), Height: intPtr(1080),
			Framerate: float64Ptr(23.976), BitDepth: intPtr(8),
		},
		{
			ID: "s2", MediaItemID: item.ID, StreamIndex: 1, StreamType: "audio",
			Codec: "aac", CodecLongName: "AAC", Language: "en",
			IsDefault: true, Channels: intPtr(6), ChannelLayout: stringPtr("5.1"),
			SampleRate: intPtr(48000), Bitrate: int64Ptr(384000),
		},
	}
	require.NoError(t, streamRepo.CreateBatch(ctx, streams))

	got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "s1", got[0].ID)
	assert.Equal(t, "video", got[0].StreamType)
	assert.Equal(t, "h264", got[0].Codec)
	assert.Equal(t, 1920, *got[0].Width)
	assert.Equal(t, 1080, *got[0].Height)

	assert.Equal(t, "s2", got[1].ID)
	assert.Equal(t, "audio", got[1].StreamType)
	assert.Equal(t, 6, *got[1].Channels)
}

func TestMediaStreamRepo_CreateBatch_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	streamRepo := postgres.NewMediaStreamRepository(db)
	require.NoError(t, streamRepo.CreateBatch(context.Background(), nil))
}

func TestMediaStreamRepo_DeleteByMediaItemID(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mslib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("msmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	streams := []*library.MediaStream{
		{ID: "sd1", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "h264"},
	}
	require.NoError(t, streamRepo.CreateBatch(ctx, streams))

	require.NoError(t, streamRepo.DeleteByMediaItemID(ctx, item.ID))

	got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}
