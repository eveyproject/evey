package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
)

// A metadata refresh writes the probe's own columns and nothing else: what
// the backfills wrote on the row - level and its version, idet, frame-rate
// mode and its algorithm version, the keyframe index - stays, so none of
// their workers lists the file again. A column the probe did not read keeps
// its value; another stream set writes nothing.
func TestMediaStreamRepo_RefreshProbedFields_KeepsEveryBackfill(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("refreshlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("refreshmi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, repo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "v", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "hevc", BitDepth: new(10)},
		{ID: "a", MediaItemID: item.ID, StreamIndex: 1, StreamType: "audio", Codec: "eac3", Bitrate: new(int64(640000))},
	}))
	_, err := repo.SetVideoLevels(ctx, item.ID, []*library.MediaStream{{StreamIndex: 0, StreamType: "video", Level: new(153)}})
	require.NoError(t, err)
	_, err = repo.MarkVideoLevelProbed(ctx, item.ID, library.LevelProbeVersion)
	require.NoError(t, err)
	_, err = repo.SetIdetInterlaced(ctx, item.ID, false)
	require.NoError(t, err)
	_, err = repo.SetKeyframes(ctx, item.ID, []int64{0, 2002, 4004})
	require.NoError(t, err)
	_, err = repo.SetFrameRateMode(ctx, item.ID, "cfr", 3)
	require.NoError(t, err)
	backfills := func() (row [6]any) {
		t.Helper()
		var level, levelV, algo int
		var idet bool
		var mode string
		var keyframes bool
		require.NoError(t, db.QueryRowContext(ctx, `SELECT level, level_probe_version, idet_interlaced, frame_rate_mode,
			frame_rate_algo_version, keyframes_ms IS NOT NULL FROM media_streams WHERE id = 'v'`).
			Scan(&level, &levelV, &idet, &mode, &algo, &keyframes))
		return [6]any{level, levelV, idet, mode, algo, keyframes}
	}
	before := backfills()

	probed := []*library.MediaStream{
		{StreamIndex: 0, StreamType: "video", Codec: "hevc", Framerate: new(23.976), ColorTransfer: new("smpte2084")},
		{StreamIndex: 1, StreamType: "audio", Codec: "eac3"},
	}
	changed, err := repo.RefreshProbedFields(ctx, item.ID, probed)
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "only the video row learnt something")

	got, err := repo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "v", got[0].ID, "refreshed in place, not replaced")
	assert.InDelta(t, 23.976, *got[0].Framerate, 1e-9)
	assert.Equal(t, "smpte2084", *got[0].ColorTransfer)
	assert.Equal(t, 10, *got[0].BitDepth, "a column the probe did not read keeps its value")
	assert.Equal(t, int64(640000), *got[1].Bitrate)
	assert.Equal(t, before, backfills(), "every backfill stays")

	changed, err = repo.RefreshProbedFields(ctx, item.ID, probed)
	require.NoError(t, err)
	assert.Zero(t, changed, "nothing new, nothing written")

	// The probe now sees a third stream: no row to write to, nothing written.
	other := append(probed, &library.MediaStream{StreamIndex: 2, StreamType: "subtitle", Codec: "subrip"})
	other[0] = &library.MediaStream{StreamIndex: 0, StreamType: "video", Codec: "hevc", Framerate: new(25.0)}
	_, err = repo.RefreshProbedFields(ctx, item.ID, other)
	require.ErrorIs(t, err, library.ErrStreamSetChanged)
	got, err = repo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.InDelta(t, 23.976, *got[0].Framerate, 1e-9, "a refused refresh writes nothing")
}
