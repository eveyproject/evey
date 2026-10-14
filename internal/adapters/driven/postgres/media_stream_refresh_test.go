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
		{ID: "a", MediaItemID: item.ID, StreamIndex: 1, StreamType: "audio", Codec: "ac3", Bitrate: new(int64(640000)),
			Channels: new(6), ChannelLayout: new("5.1")},
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
		// Muxed as 6 channels, the stream says stereo.
		{StreamIndex: 1, StreamType: "audio", Codec: "ac3", Channels: new(2), ChannelLayout: new("stereo")},
	}
	res, err := repo.RefreshProbedFields(ctx, item.ID, probed)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Changed, "the video and the audio rows learnt something")
	assert.Empty(t, res.Kept)

	got, err := repo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "v", got[0].ID, "refreshed in place, not replaced")
	assert.InDelta(t, 23.976, *got[0].Framerate, 1e-9)
	assert.Equal(t, "smpte2084", *got[0].ColorTransfer)
	assert.Equal(t, 10, *got[0].BitDepth, "a column the probe did not read keeps its value")
	assert.Equal(t, int64(640000), *got[1].Bitrate)
	assert.Equal(t, 2, *got[1].Channels)
	assert.Equal(t, "stereo", *got[1].ChannelLayout)
	assert.Equal(t, before, backfills(), "every backfill stays")

	res, err = repo.RefreshProbedFields(ctx, item.ID, probed)
	require.NoError(t, err)
	assert.Zero(t, res.Changed, "nothing new, nothing written")

	// The probe now sees a third stream: no row to write to, nothing written.
	other := append(probed, &library.MediaStream{StreamIndex: 2, StreamType: "subtitle", Codec: "subrip"})
	other[0] = &library.MediaStream{StreamIndex: 0, StreamType: "video", Codec: "hevc", Framerate: new(25.0)}
	_, err = repo.RefreshProbedFields(ctx, item.ID, other)
	require.ErrorIs(t, err, library.ErrStreamSetChanged)
	got, err = repo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	assert.InDelta(t, 23.976, *got[0].Framerate, 1e-9, "a refused refresh writes nothing")
}

// Field by field: one field the refresh may not take (another codec, a title
// someone set) is kept and listed, and the rest of the file is refreshed all
// the same. An MP4 timed-text "subrip" becomes "mov_text"; a title that is the
// track's MP4 handler name - what an older probe stored - is cleared.
func TestMediaStreamRepo_RefreshProbedFields_FieldByField(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("fieldlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("fieldmi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	require.NoError(t, repo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "v", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", Title: "video handler"},
		{ID: "a", MediaItemID: item.ID, StreamIndex: 1, StreamType: "audio", Codec: "aac", Title: "Commentaire"},
		{ID: "s", MediaItemID: item.ID, StreamIndex: 2, StreamType: "subtitle", Codec: "subrip", Title: "srt@GPAC0.9.0"},
		{ID: "t", MediaItemID: item.ID, StreamIndex: 3, StreamType: "subtitle", Codec: "ass", Title: "Forced"},
	}))

	probed := []*library.MediaStream{
		// The handler name the old probe took for a title: cleared.
		{StreamIndex: 0, StreamType: "video", Codec: "h264", Framerate: new(23.976), HandlerName: "video handler"},
		// A title that is not the handler name (an edit, a real name): kept.
		{StreamIndex: 1, StreamType: "audio", Codec: "aac", Channels: new(2), HandlerName: "SoundHandler"},
		// The one codec rename the refresh takes, and its handler title.
		{StreamIndex: 2, StreamType: "subtitle", Codec: "mov_text", HandlerName: "srt@GPAC0.9.0"},
		// Another codec: kept and listed, the row still refreshed.
		{StreamIndex: 3, StreamType: "subtitle", Codec: "subrip", Title: "Forced"},
	}
	res, err := repo.RefreshProbedFields(ctx, item.ID, probed)
	require.NoError(t, err, "a field in disagreement no longer refuses the file")
	assert.Equal(t, 3, res.Changed)
	assert.ElementsMatch(t, []string{
		`stream 1 title: stored "Commentaire", probe ""`,
		`stream 3 codec: stored "ass", probe "subrip"`,
	}, res.Kept)

	got, err := repo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, got, 4)
	assert.Empty(t, got[0].Title)
	assert.InDelta(t, 23.976, *got[0].Framerate, 1e-9)
	assert.Equal(t, "Commentaire", got[1].Title)
	assert.Equal(t, 2, *got[1].Channels, "the rest of a row with a kept field is refreshed")
	assert.Equal(t, "mov_text", got[2].Codec)
	assert.Empty(t, got[2].Title)
	assert.Equal(t, "ass", got[3].Codec)
	assert.Equal(t, "Forced", got[3].Title)

	// Idempotent: the same probe again writes nothing and lists the same.
	again, err := repo.RefreshProbedFields(ctx, item.ID, probed)
	require.NoError(t, err)
	assert.Zero(t, again.Changed)
	assert.ElementsMatch(t, res.Kept, again.Kept)
}
