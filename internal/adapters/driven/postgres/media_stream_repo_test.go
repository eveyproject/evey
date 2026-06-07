package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
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
	assert.Equal(t, library.StreamTypeVideo, got[0].StreamType)
	assert.Equal(t, "h264", got[0].Codec)
	assert.Equal(t, 1920, *got[0].Width)
	assert.Equal(t, 1080, *got[0].Height)

	assert.Equal(t, "s2", got[1].ID)
	assert.Equal(t, library.StreamTypeAudio, got[1].StreamType)
	assert.Equal(t, 6, *got[1].Channels)
}

func TestMediaStreamRepo_CreateBatch_Empty(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	streamRepo := postgres.NewMediaStreamRepository(db)
	require.NoError(t, streamRepo.CreateBatch(context.Background(), nil))
}

func TestMediaStreamRepo_ListVideoLevelMissing_Scope(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("lvllib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	// A: h264 video, NULL level → candidate.
	itemA := newTestMediaItem("lvlA", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemA))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "a-v", MediaItemID: itemA.ID, StreamIndex: 0, StreamType: "video", Codec: "h264"},
		{ID: "a-a", MediaItemID: itemA.ID, StreamIndex: 1, StreamType: "audio", Codec: "aac"},
	}))
	// B: hevc video, NULL level → NOT a candidate (non-h264).
	itemB := newTestMediaItem("lvlB", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemB))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "b-v", MediaItemID: itemB.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc"},
	}))
	// C: h264 video with a level already set → NOT a candidate.
	itemC := newTestMediaItem("lvlC", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemC))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "c-v", MediaItemID: itemC.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", Level: new(40)},
	}))

	ids, err := streamRepo.ListVideoLevelMissing(ctx, 10, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{string(itemA.ID)}, ids, "only the h264 NULL-level item is a candidate")

	excluded, err := streamRepo.ListVideoLevelMissing(ctx, 10, []string{string(itemA.ID)})
	require.NoError(t, err)
	assert.Empty(t, excluded, "excluding the only candidate yields none")
}

func TestMediaStreamRepo_SetVideoLevels_FillsOnceNeverWipes(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("lvllib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("lvlfill", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "f-v", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "h264"},
		{ID: "f-a", MediaItemID: item.ID, StreamIndex: 1, StreamType: "audio", Codec: "aac"},
	}))

	// Fill from a "probe" (matched by stream_index). Audio carries no level.
	probe := []*library.MediaStream{
		{StreamType: "video", StreamIndex: 0, Codec: "h264", Level: new(40)},
		{StreamType: "audio", StreamIndex: 1, Codec: "aac"},
	}
	n, err := streamRepo.SetVideoLevels(ctx, item.ID, probe)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "exactly the one NULL-level video row is updated")

	got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 2, "SetVideoLevels must never delete streams")
	require.NotNil(t, got[0].Level)
	assert.Equal(t, 40, *got[0].Level)
	assert.Nil(t, got[1].Level, "audio level stays NULL")

	// Second call must NOT overwrite the existing level (level IS NULL guard).
	n2, err := streamRepo.SetVideoLevels(ctx, item.ID,
		[]*library.MediaStream{{StreamType: "video", StreamIndex: 0, Codec: "h264", Level: new(99)}})
	require.NoError(t, err)
	assert.Equal(t, 0, n2, "an already-set level is not overwritten")

	got2, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, got2[0].Level)
	assert.Equal(t, 40, *got2[0].Level, "level unchanged on the second call")
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
