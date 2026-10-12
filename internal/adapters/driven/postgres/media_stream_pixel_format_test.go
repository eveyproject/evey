package postgres_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/require"
)

// pixel_format was the 37th column of a bulk INSERT whose placeholders are
// enumerated by hand ($1..$36 per row). An off-by-one there does not fail: it
// writes every value one column to the left, silently, for every stream of
// every scanned file. This asserts the value comes back on the field it went in
// on, and that a stream without one stays nil rather than borrowing a neighbour.
func TestMediaStreamRepo_PixelFormat_RoundTrips(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("pxlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("pxmi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	width, height, depth, channels := 3840, 2160, 10, 2
	pixFmt, space, transfer := "yuv420p10le", "bt2020nc", "smpte2084"
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{
			ID: "px1", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video",
			Codec: "hevc", Width: &width, Height: &height,
			BitDepth: &depth, PixelFormat: &pixFmt,
			ColorSpace: &space, ColorTransfer: &transfer,
		},
		{
			ID: "px2", MediaItemID: item.ID, StreamIndex: 1, StreamType: "audio",
			Codec: "aac", Channels: &channels,
		},
	}))

	got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)

	byID := map[string]*library.MediaStream{}
	for _, s := range got {
		byID[s.ID] = s
	}

	video := byID["px1"]
	require.NotNil(t, video.PixelFormat)
	require.Equal(t, "yuv420p10le", *video.PixelFormat)
	// The neighbours on either side must be untouched - that is what an
	// off-by-one would break first.
	require.NotNil(t, video.BitDepth)
	require.Equal(t, 10, *video.BitDepth)
	require.NotNil(t, video.ColorSpace)
	require.Equal(t, "bt2020nc", *video.ColorSpace)

	require.Nil(t, byID["px2"].PixelFormat, "a stream with no pixel format must stay nil")
}
