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
	// B: hevc video, NULL level + NULL color_transfer → candidate (HEVC needs
	// both for the copy rung's HDR ground truth).
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
	// D: hevc with level but NULL color_transfer → still a candidate (probed
	// before the HDR-truth columns existed → must re-probe for DoVi).
	itemD := newTestMediaItem("lvlD", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemD))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "d-v", MediaItemID: itemD.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153)},
	}))
	// E: hevc with level AND color_transfer → fully probed, NOT a candidate.
	itemE := newTestMediaItem("lvlE", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemE))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "e-v", MediaItemID: itemE.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153), ColorTransfer: stringPtr("smpte2084")},
	}))
	// F: vp9, NULL everything → NOT a candidate (out of copy-rung scope).
	itemF := newTestMediaItem("lvlF", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemF))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "f-v", MediaItemID: itemF.ID, StreamIndex: 0, StreamType: "video", Codec: "vp9"},
	}))

	ids, err := streamRepo.ListVideoLevelMissing(ctx, 10, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{string(itemA.ID), string(itemB.ID), string(itemD.ID)}, ids,
		"candidates = h264 NULL-level + hevc missing level or color_transfer")

	excluded, err := streamRepo.ListVideoLevelMissing(ctx, 10, []string{string(itemA.ID), string(itemB.ID), string(itemD.ID)})
	require.NoError(t, err)
	assert.Empty(t, excluded, "excluding every candidate yields none")
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

// The HEVC variant of the backfill: a fresh probe fills the HDR ground truth
// (color_transfer + DoVi record) alongside the level, each column only where
// NULL, and the filled item drops out of the candidate query.
func TestMediaStreamRepo_SetVideoLevels_FillsHDRTruth(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("lvllib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("hdrfill", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "h-v", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc"},
	}))

	probe := []*library.MediaStream{
		{StreamType: "video", StreamIndex: 0, Codec: "hevc", Level: new(153),
			ColorTransfer: stringPtr("smpte2084"), DoViProfile: new(8), DoViBLCompat: new(1)},
	}
	n, err := streamRepo.SetVideoLevels(ctx, item.ID, probe)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Level)
	assert.Equal(t, 153, *got[0].Level)
	require.NotNil(t, got[0].ColorTransfer)
	assert.Equal(t, "smpte2084", *got[0].ColorTransfer)
	require.NotNil(t, got[0].DoViProfile)
	assert.Equal(t, 8, *got[0].DoViProfile)
	require.NotNil(t, got[0].DoViBLCompat)
	assert.Equal(t, 1, *got[0].DoViBLCompat)

	ids, err := streamRepo.ListVideoLevelMissing(ctx, 10, nil)
	require.NoError(t, err)
	assert.Empty(t, ids, "a fully probed hevc item is no longer a candidate")

	// A second probe must not overwrite anything (per-column NULL guards).
	n2, err := streamRepo.SetVideoLevels(ctx, item.ID,
		[]*library.MediaStream{{StreamType: "video", StreamIndex: 0, Codec: "hevc", Level: new(99),
			ColorTransfer: stringPtr("bt709"), DoViProfile: new(5)}})
	require.NoError(t, err)
	assert.Equal(t, 0, n2, "already-filled metadata is not overwritten")

	got2, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, 153, *got2[0].Level)
	assert.Equal(t, "smpte2084", *got2[0].ColorTransfer)
	assert.Equal(t, 8, *got2[0].DoViProfile)
}

func TestMediaStreamRepo_KeyframesScopeFillRoundtrip(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("kflib1")
	require.NoError(t, libRepo.Create(ctx, lib))

	// A: h264 video WITH level, keyframes NULL → candidate.
	itemA := newTestMediaItem("kfA", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemA))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "a-v", MediaItemID: itemA.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", Level: new(40)},
	}))
	// B: h264 video WITHOUT level → NOT a candidate (keyframes only matter once copy-eligible).
	itemB := newTestMediaItem("kfB", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemB))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "b-v", MediaItemID: itemB.ID, StreamIndex: 0, StreamType: "video", Codec: "h264"},
	}))
	// C: hevc fully probed (level + transfer), no DoVi → candidate.
	itemC := newTestMediaItem("kfC", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemC))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "c-v", MediaItemID: itemC.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153), ColorTransfer: stringPtr("smpte2084")},
	}))
	// D: hevc DoVi profile 5 → NEVER copied, so indexing its keyframes is waste.
	itemD := newTestMediaItem("kfD", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemD))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "d-v", MediaItemID: itemD.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153), ColorTransfer: stringPtr("smpte2084"), DoViProfile: new(5)},
	}))
	// E: hevc DoVi profile 8.1 (HDR10-compatible base) → copy-able, candidate.
	itemE := newTestMediaItem("kfE", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemE))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "e-v", MediaItemID: itemE.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153), ColorTransfer: stringPtr("smpte2084"), DoViProfile: new(8), DoViBLCompat: new(1)},
	}))
	// F: hevc with level but color_transfer NULL → HDR truth unknown, NOT a candidate.
	itemF := newTestMediaItem("kfF", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemF))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "kf-f-v", MediaItemID: itemF.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153)},
	}))

	ids, err := streamRepo.ListVideoKeyframesMissing(ctx, 10, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{string(itemA.ID), string(itemC.ID), string(itemE.ID)}, ids,
		"candidates = copy-eligible streams only (h264 leveled; hevc probed, not DoVi-incompatible)")

	// Fill A's keyframes; it must drop out of the candidate query and read back.
	kf := []int64{0, 3253, 6548, 13138}
	n, err := streamRepo.SetKeyframes(ctx, itemA.ID, kf)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := streamRepo.GetByMediaItemID(ctx, itemA.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, kf, got[0].Keyframes, "keyframes round-trip through the bigint[] column")

	after, err := streamRepo.ListVideoKeyframesMissing(ctx, 10, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{string(itemC.ID), string(itemE.ID)}, after,
		"an indexed item is no longer a candidate; the hevc candidates remain")

	// Never overwrite; empty slice is a no-op.
	n2, err := streamRepo.SetKeyframes(ctx, itemA.ID, []int64{999})
	require.NoError(t, err)
	assert.Equal(t, 0, n2, "existing keyframes are not overwritten")
	n3, err := streamRepo.SetKeyframes(ctx, itemB.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, n3, "empty keyframes write nothing")
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
