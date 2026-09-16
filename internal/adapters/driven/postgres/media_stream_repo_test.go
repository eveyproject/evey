package postgres_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
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

func TestMediaStreamRepo_GetByMediaItemIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("mslib2")
	require.NoError(t, libRepo.Create(ctx, lib))
	item1 := newTestMediaItem("msmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item1))
	item2 := newTestMediaItem("msmi3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item2))
	item3 := newTestMediaItem("msmi4", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item3))

	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "b1", MediaItemID: item1.ID, StreamIndex: 1, StreamType: "audio", Codec: "eac3", Language: "fr", Channels: new(6)},
		{ID: "b0", MediaItemID: item1.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", Width: new(1920), Height: new(1080)},
		{ID: "b2", MediaItemID: item2.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Width: new(3840), Height: new(2160)},
	}))

	got, err := streamRepo.GetByMediaItemIDs(ctx, []shared.MediaItemID{item1.ID, item2.ID, item3.ID})
	require.NoError(t, err)
	require.Len(t, got, 2)

	require.Len(t, got[item1.ID], 2)
	assert.Equal(t, "b0", got[item1.ID][0].ID, "streams must come back in stream_index order")
	assert.Equal(t, "h264", got[item1.ID][0].Codec)
	assert.Equal(t, 1080, *got[item1.ID][0].Height)
	assert.Equal(t, "eac3", got[item1.ID][1].Codec)
	assert.Equal(t, 6, *got[item1.ID][1].Channels)

	require.Len(t, got[item2.ID], 1)
	assert.Equal(t, "hevc", got[item2.ID][0].Codec)

	_, has := got[item3.ID]
	assert.False(t, has, "an item with no streams is absent from the map")

	empty, err := streamRepo.GetByMediaItemIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
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
		{ID: "e-v", MediaItemID: itemE.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153), ColorTransfer: new("smpte2084")},
	}))
	// F: vp9, NULL everything → candidate too since B4 (av1/vp9 join the
	// HDR-capable copy scope, same ground-truth needs as hevc).
	itemF := newTestMediaItem("lvlF", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemF))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "f-v", MediaItemID: itemF.ID, StreamIndex: 0, StreamType: "video", Codec: "vp9"},
	}))
	// G: mpeg2, idet_interlaced NULL → candidate via the idet branch (B3.1).
	itemG := newTestMediaItem("lvlG", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemG))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "g-v", MediaItemID: itemG.ID, StreamIndex: 0, StreamType: "video", Codec: "mpeg2video"},
	}))
	// H: mpeg2 with the idet verdict already set → NOT a candidate.
	itemH := newTestMediaItem("lvlH", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemH))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "h-v", MediaItemID: itemH.ID, StreamIndex: 0, StreamType: "video", Codec: "mpeg2video", IdetInterlaced: new(false)},
	}))
	// I: AV1 codec but out of legacy scope → not an idet candidate (only the
	// HDR-truth branch applies, already covered by F-style logic; here it's
	// fully probed so NOT a candidate).
	itemI := newTestMediaItem("lvlI", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemI))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "i-v", MediaItemID: itemI.ID, StreamIndex: 0, StreamType: "video", Codec: "av1", Level: new(8), ColorTransfer: new("bt709")},
	}))

	ids, err := streamRepo.ListVideoLevelMissing(ctx, 10, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{string(itemA.ID), string(itemB.ID), string(itemD.ID), string(itemF.ID), string(itemG.ID)}, ids,
		"candidates = h264 NULL-level + hevc/av1/vp9 missing HDR truth + legacy codec missing idet verdict")

	excluded, err := streamRepo.ListVideoLevelMissing(ctx, 10,
		[]string{string(itemA.ID), string(itemB.ID), string(itemD.ID), string(itemF.ID), string(itemG.ID)})
	require.NoError(t, err)
	assert.Empty(t, excluded, "excluding every candidate yields none")
}

// B3 legacy-codec persistence: SetIdetInterlaced's definitive verdict +
// no-overwrite guard, and the field_order/sample_aspect_ratio round-trip and
// backfill. Both share ONE testcontainer (subtests) to keep the postgres
// package's total testcontainer spin-up under the suite timeout.
func TestMediaStreamRepo_LegacyCodecPersistence(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("legacylib")
	require.NoError(t, libRepo.Create(ctx, lib))

	t.Run("SetIdetInterlaced definitive + no overwrite", func(t *testing.T) {
		item := newTestMediaItem("idet1", lib.ID)
		require.NoError(t, itemRepo.Create(ctx, item))
		require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
			{ID: "id-v", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "mpeg2video"},
		}))

		n, err := streamRepo.SetIdetInterlaced(ctx, item.ID, true)
		require.NoError(t, err)
		assert.Equal(t, 1, n)

		got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.NotNil(t, got[0].IdetInterlaced)
		assert.True(t, *got[0].IdetInterlaced)

		// Second call must NOT overwrite (idet_interlaced IS NULL guard).
		n2, err := streamRepo.SetIdetInterlaced(ctx, item.ID, false)
		require.NoError(t, err)
		assert.Equal(t, 0, n2, "an existing verdict is not overwritten")
		got2, err := streamRepo.GetByMediaItemID(ctx, item.ID)
		require.NoError(t, err)
		assert.True(t, *got2[0].IdetInterlaced, "verdict unchanged on the second call")
	})

	t.Run("field_order + SAR round-trip and backfill", func(t *testing.T) {
		item := newTestMediaItem("legacy1", lib.ID)
		require.NoError(t, itemRepo.Create(ctx, item))
		require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
			{ID: "lg-v", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "mpeg2video",
				FieldOrder: new("tt"), SampleAspectRatio: new("64:45")},
		}))

		got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.NotNil(t, got[0].FieldOrder)
		assert.Equal(t, "tt", *got[0].FieldOrder)
		require.NotNil(t, got[0].SampleAspectRatio)
		assert.Equal(t, "64:45", *got[0].SampleAspectRatio)

		// A backfill probe fills both on a stream that had them NULL.
		item2 := newTestMediaItem("legacy2", lib.ID)
		require.NoError(t, itemRepo.Create(ctx, item2))
		require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
			{ID: "lg2-v", MediaItemID: item2.ID, StreamIndex: 0, StreamType: "video", Codec: "mpeg4"},
		}))
		probe := []*library.MediaStream{
			{StreamType: "video", StreamIndex: 0, Codec: "mpeg4", FieldOrder: new("progressive"), SampleAspectRatio: new("16:11")},
		}
		upd, err := streamRepo.SetVideoLevels(ctx, item2.ID, probe)
		require.NoError(t, err)
		assert.Equal(t, 1, upd)
		got2, err := streamRepo.GetByMediaItemID(ctx, item2.ID)
		require.NoError(t, err)
		assert.Equal(t, "progressive", *got2[0].FieldOrder)
		assert.Equal(t, "16:11", *got2[0].SampleAspectRatio)
	})
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
			ColorTransfer: new("smpte2084"), DoViProfile: new(8), DoViBLCompat: new(1)},
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
			ColorTransfer: new("bt709"), DoViProfile: new(5)}})
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
		{ID: "c-v", MediaItemID: itemC.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153), ColorTransfer: new("smpte2084")},
	}))
	// D: hevc DoVi profile 5 → NEVER copied, so indexing its keyframes is waste.
	itemD := newTestMediaItem("kfD", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemD))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "d-v", MediaItemID: itemD.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153), ColorTransfer: new("smpte2084"), DoViProfile: new(5)},
	}))
	// E: hevc DoVi profile 8.1 (HDR10-compatible base) → copy-able, candidate.
	itemE := newTestMediaItem("kfE", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemE))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "e-v", MediaItemID: itemE.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153), ColorTransfer: new("smpte2084"), DoViProfile: new(8), DoViBLCompat: new(1)},
	}))
	// F: hevc with level but color_transfer NULL → HDR truth unknown, NOT a candidate.
	itemF := newTestMediaItem("kfF", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemF))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "kf-f-v", MediaItemID: itemF.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", Level: new(153)},
	}))

	ids, err := streamRepo.ListVideoKeyframesMissing(ctx, 10, nil, 1)
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

	after, err := streamRepo.ListVideoKeyframesMissing(ctx, 10, nil, 1)
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

func TestMediaStreamRepo_KeyframeCandidates_StampedUnindexableDropOut(t *testing.T) {
	// A probe that settles on "no index possible" stamps the row at the current
	// prober version; the candidate query must stop re-serving it (it used to
	// come back on every boot, forever - prod 2026-07-26, 68 avi files). A
	// version bump re-opens every stamped row: the supported "the prober
	// learned this container" path, mirroring frame_rate_algo_version.
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("kflib3")
	require.NoError(t, libRepo.Create(ctx, lib))
	itemA := newTestMediaItem("kfM1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemA))
	itemB := newTestMediaItem("kfM2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, itemB))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "kfm1-v", MediaItemID: itemA.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", Level: new(40)},
		{ID: "kfm2-v", MediaItemID: itemB.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", Level: new(40)},
	}))

	const version int16 = 1
	ids, err := streamRepo.ListVideoKeyframesMissing(ctx, 10, nil, version)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{string(itemA.ID), string(itemB.ID)}, ids)

	n, err := streamRepo.MarkKeyframesUnindexable(ctx, itemA.ID, version)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	ids, err = streamRepo.ListVideoKeyframesMissing(ctx, 10, nil, version)
	require.NoError(t, err)
	assert.Equal(t, []string{string(itemB.ID)}, ids, "a stamped row must drop out of the candidates")

	ids, err = streamRepo.ListVideoKeyframesMissing(ctx, 10, nil, version+1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{string(itemA.ID), string(itemB.ID)}, ids,
		"a prober version bump re-opens rows stamped by the older prober")
}

func TestMediaStreamRepo_MarkKeyframesUnindexable_NeverTouchesIndexedRows(t *testing.T) {
	// The stamp only concerns rows the probe could NOT index; a row holding a
	// real keyframe index already dropped out through keyframes_ms IS NOT NULL
	// and must never be restamped.
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("kflib4")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("kfM3", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "kfm3-v", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", Level: new(40)},
	}))

	n, err := streamRepo.SetKeyframes(ctx, item.ID, []int64{0, 2000, 4000})
	require.NoError(t, err)
	require.Equal(t, 1, n)

	n, err = streamRepo.MarkKeyframesUnindexable(ctx, item.ID, 1)
	require.NoError(t, err)
	assert.Zero(t, n, "an indexed row is out of scope for the unindexable stamp")
}

// TestMediaStreamRepo_FrameRateMode_RoundTrip verifies frame_rate_mode
// defaults to ” (unknown) for existing rows, persists a cfr/vfr
// classification, and never overwrites a value already set - mirroring
// SetKeyframes' idempotency but keyed on the column's default rather than SQL
// NULL (frame_rate_mode is NOT NULL DEFAULT ”).
func TestMediaStreamRepo_FrameRateMode_RoundTrip(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("frmlib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("frmmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "frm-v", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", Level: new(40)},
	}))

	got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].FrameRateMode, "unanalyzed rows default to '' (unknown)")

	// Stamped at version 0: what every row written before the classifier was
	// versioned looks like, i.e. the rows carrying the false 'vfr' verdicts.
	n, err := streamRepo.SetFrameRateMode(ctx, item.ID, "vfr", 0)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err = streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "vfr", got[0].FrameRateMode, "the classification round-trips through the TEXT column")

	// Never overwrite: a later probe (e.g. a race with a background re-run)
	// must not clobber the persisted classification.
	n2, err := streamRepo.SetFrameRateMode(ctx, item.ID, "cfr", 1)
	require.NoError(t, err)
	assert.Equal(t, 0, n2, "an already-classified stream is not overwritten")

	got, err = streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "vfr", got[0].FrameRateMode, "the original classification is preserved")

	// The stale-verdict drain finds it, because its stamp predates version 1.
	stale, err := streamRepo.ListVideoFrameRateStale(ctx, 10, 1, nil)
	require.NoError(t, err)
	assert.Contains(t, stale, string(item.ID), "a verdict from an older classifier is listed as stale")

	excluded, err := streamRepo.ListVideoFrameRateStale(ctx, 10, 1, []string{string(item.ID)})
	require.NoError(t, err)
	assert.NotContains(t, excluded, string(item.ID),
		"the exclude list keeps a failed item from pinning the queue")

	// Reclassify is the ONLY write that may overwrite a verdict - this is the
	// whole point: SetFrameRateMode above refused the very same correction.
	n3, err := streamRepo.ReclassifyFrameRateMode(ctx, item.ID, "cfr", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, n3)

	got, err = streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "cfr", got[0].FrameRateMode, "the stale verdict is corrected")

	// Restamped at the current version, so the drain is self-limiting: the item
	// disappears from the candidate set instead of being re-analyzed forever.
	stale, err = streamRepo.ListVideoFrameRateStale(ctx, 10, 1, nil)
	require.NoError(t, err)
	assert.NotContains(t, stale, string(item.ID), "a restamped row is no longer stale")

	// Idempotent: a second drain at the same version cannot walk it backwards.
	n4, err := streamRepo.ReclassifyFrameRateMode(ctx, item.ID, "vfr", 1)
	require.NoError(t, err)
	assert.Equal(t, 0, n4, "an up-to-date verdict is not rewritten")

	// An "unknown" answer from a NEWER classifier stamps the version but must
	// not drop the verdict - the analyzer failing to speak is not evidence that
	// the stored answer was wrong.
	n5, err := streamRepo.ReclassifyFrameRateMode(ctx, item.ID, "", 2)
	require.NoError(t, err)
	assert.Equal(t, 1, n5)

	got, err = streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "cfr", got[0].FrameRateMode, "an unknown answer never erases a stored verdict")

	// ...but it DID stamp, so the row converges instead of being re-listed on
	// every drain forever.
	stale, err = streamRepo.ListVideoFrameRateStale(ctx, 10, 2, nil)
	require.NoError(t, err)
	assert.NotContains(t, stale, string(item.ID), "the stamp takes it out of the candidate set")
}

// A row that was NEVER classified is a candidate too: ” fails open at the copy
// gate, so a genuinely VFR source sitting there is served as copy and drifts
// A/V - and it never self-heals, because classification only ever rode along a
// keyframe write.
func TestMediaStreamRepo_FrameRateStale_IncludesNeverClassified(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("frlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("frmi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		{
			ID: "frs1", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video",
			Codec: "h264", Language: "und",
		},
	}))

	// No level yet ⇒ not copy-eligible ⇒ analyzing it would change no decision.
	stale, err := streamRepo.ListVideoFrameRateStale(ctx, 10, 1, nil)
	require.NoError(t, err)
	assert.NotContains(t, stale, string(item.ID),
		"a source that cannot be served as copy is not worth a full-file walk")

	// The level backfill fills it in; the row becomes a candidate on its own,
	// which is why excluding it above loses nothing.
	_, err = streamRepo.SetVideoLevels(ctx, item.ID, []*library.MediaStream{
		{StreamType: "video", StreamIndex: 0, Codec: "h264", Level: new(40)},
	})
	require.NoError(t, err)

	stale, err = streamRepo.ListVideoFrameRateStale(ctx, 10, 1, nil)
	require.NoError(t, err)
	assert.Contains(t, stale, string(item.ID),
		"a never-classified row (frame_rate_mode = '') is a candidate once copy-eligible")

	n, err := streamRepo.ReclassifyFrameRateMode(ctx, item.ID, "vfr", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "vfr", got[0].FrameRateMode, "the first real verdict lands on a '' row")
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

// TestMediaStreamRepo_ExtendedDispositions_RoundTrip verifies that the 5 extended
// disposition columns (commentary / hearing_impaired / visual_impaired /
// text_descriptions / original) are persisted and read back correctly.
func TestMediaStreamRepo_ExtendedDispositions_RoundTrip(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("displib1")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("dispmi1", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))

	streams := []*library.MediaStream{
		{
			ID: "disp-v", MediaItemID: item.ID, StreamIndex: 0,
			StreamType: "video", Codec: "h264",
			// No flags set - zero values for the extended booleans.
		},
		{
			ID: "disp-a1", MediaItemID: item.ID, StreamIndex: 1,
			StreamType: "audio", Codec: "aac", Language: "eng",
			IsDefault: true, Commentary: true,
		},
		{
			ID: "disp-a2", MediaItemID: item.ID, StreamIndex: 2,
			StreamType: "audio", Codec: "aac", Language: "eng",
			HearingImpaired: true,
		},
		{
			ID: "disp-s", MediaItemID: item.ID, StreamIndex: 3,
			StreamType: "subtitle", Codec: "subrip", Language: "eng",
			VisualImpaired: true, TextDescriptions: true, Original: true,
		},
	}
	require.NoError(t, streamRepo.CreateBatch(ctx, streams))

	got, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 4)

	video := got[0]
	assert.False(t, video.Commentary)
	assert.False(t, video.HearingImpaired)
	assert.False(t, video.VisualImpaired)
	assert.False(t, video.TextDescriptions)
	assert.False(t, video.Original)

	audioCommentary := got[1]
	assert.True(t, audioCommentary.Commentary, "commentary must round-trip")
	assert.False(t, audioCommentary.HearingImpaired)
	assert.False(t, audioCommentary.VisualImpaired)
	assert.False(t, audioCommentary.TextDescriptions)
	assert.False(t, audioCommentary.Original)

	audioHI := got[2]
	assert.False(t, audioHI.Commentary)
	assert.True(t, audioHI.HearingImpaired, "hearing_impaired must round-trip")

	sub := got[3]
	assert.False(t, sub.Commentary)
	assert.False(t, sub.HearingImpaired)
	assert.True(t, sub.VisualImpaired, "visual_impaired must round-trip")
	assert.True(t, sub.TextDescriptions, "text_descriptions must round-trip")
	assert.True(t, sub.Original, "original must round-trip")

	// Also verify that ReplaceBatch correctly persists the flags.
	replacement := []*library.MediaStream{
		{
			ID: "disp-r", MediaItemID: item.ID, StreamIndex: 0,
			StreamType: "audio", Codec: "eac3", Language: "eng",
			Commentary: true, HearingImpaired: true,
			VisualImpaired: true, TextDescriptions: true, Original: true,
		},
	}
	require.NoError(t, streamRepo.ReplaceBatch(ctx, item.ID, replacement))

	got2, err := streamRepo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got2, 1)

	r := got2[0]
	assert.True(t, r.Commentary)
	assert.True(t, r.HearingImpaired)
	assert.True(t, r.VisualImpaired)
	assert.True(t, r.TextDescriptions)
	assert.True(t, r.Original)
}

// VideoDims serves the complexity worker the resolution the scan already
// stored (no ffprobe fork): the first video stream with both dimensions,
// (0,0) without one. The idet verdicts land on video streams only.
func TestMediaStreamRepo_VideoDims_AndIdetVerdicts(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	lib := newTestLibrary("msdimlib")
	require.NoError(t, postgres.NewLibraryRepository(db).Create(ctx, lib))
	itemRepo := postgres.NewMediaItemRepository(db)
	video, audioOnly := newTestMediaItem("msdim-v", lib.ID), newTestMediaItem("msdim-a", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, video))
	require.NoError(t, itemRepo.Create(ctx, audioOnly))
	streamRepo := postgres.NewMediaStreamRepository(db)
	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		// A cover-art "video" stream without dimensions must not answer.
		{ID: "d0", MediaItemID: video.ID, StreamIndex: 0, StreamType: "video", Codec: "mjpeg"},
		{ID: "d1", MediaItemID: video.ID, StreamIndex: 1, StreamType: "video", Codec: "hevc", Width: new(3840), Height: new(2160)},
		{ID: "d2", MediaItemID: video.ID, StreamIndex: 2, StreamType: "audio", Codec: "eac3"},
		{ID: "d3", MediaItemID: audioOnly.ID, StreamIndex: 0, StreamType: "audio", Codec: "flac"},
	}))

	w, h, err := streamRepo.VideoDims(ctx, video.ID)
	require.NoError(t, err)
	assert.Equal(t, [2]int{3840, 2160}, [2]int{w, h})
	w, h, err = streamRepo.VideoDims(ctx, audioOnly.ID)
	require.NoError(t, err)
	assert.Zero(t, w+h, "no video stream is (0,0), not an error")

	n, err := streamRepo.SetIdetTelecine(ctx, video.ID, true)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "every video stream of the item, never the audio")
	n, err = streamRepo.SetIdetTelecine(ctx, audioOnly.ID, true)
	require.NoError(t, err)
	assert.Zero(t, n)
	streams, err := streamRepo.GetByMediaItemID(ctx, video.ID)
	require.NoError(t, err)
	for _, s := range streams {
		if s.StreamType == library.StreamTypeVideo {
			require.NotNil(t, s.IdetTelecine, s.ID)
			assert.True(t, *s.IdetTelecine)
		} else {
			assert.Nil(t, s.IdetTelecine, "audio carries no idet verdict")
		}
	}
}
