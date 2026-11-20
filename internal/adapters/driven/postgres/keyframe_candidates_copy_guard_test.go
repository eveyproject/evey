package postgres_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/app/transcoding"
	"github.com/eveyproject/evey/internal/domain/library"
)

// The keyframe backfill indexes what the video-copy rung can serve, and the
// rung copies only an indexed stream. A stream the copy guard accepts but the
// candidate query leaves out is never indexed, so it is transcoded whenever
// another viewer is playing (the on-demand index then stands aside). Sprite
// Fright was that stream: VP9 Profile 0, which ffprobe reports with no level
// and no transfer. The query and the guard must answer the same on each row.
func TestMediaStreamRepo_KeyframeCandidatesFollowTheCopyGuard(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("kfguard")
	require.NoError(t, libRepo.Create(ctx, lib))

	streams := map[string]library.MediaStream{
		"h264-leveled":          {Codec: "h264", Profile: "High", Level: new(40), Width: new(1920), Height: new(1080)},
		"h264-no-level":         {Codec: "h264", Profile: "High", Width: new(1920), Height: new(1080)},
		"hevc-probed":           {Codec: "hevc", Profile: "Main 10", Level: new(153), ColorTransfer: new("smpte2084"), Width: new(3840), Height: new(2160)},
		"hevc-no-transfer":      {Codec: "hevc", Profile: "Main 10", Level: new(153), Width: new(3840), Height: new(2160)},
		"vp9-p0-as-ffprobe":     {Codec: "vp9", Profile: "Profile 0", Width: new(2048), Height: new(858)},
		"vp9-p0-leveled":        {Codec: "vp9", Profile: "Profile 0", Level: new(40), ColorTransfer: new("bt709"), Width: new(1920), Height: new(1080)},
		"vp9-p2-no-transfer":    {Codec: "vp9", Profile: "Profile 2", Width: new(3840), Height: new(2160)},
		"vp9-p2-pq":             {Codec: "vp9", Profile: "Profile 2", ColorTransfer: new("smpte2084"), Width: new(3840), Height: new(2160)},
		"vp9-p0-no-size":        {Codec: "vp9", Profile: "Profile 0"},
		"vp9-p0-padded-profile": {Codec: "vp9", Profile: " profile 0 ", Width: new(1280), Height: new(720)},
	}
	for id, s := range streams {
		item := newTestMediaItem(id, lib.ID)
		require.NoError(t, itemRepo.Create(ctx, item))
		row := s
		row.ID, row.MediaItemID, row.StreamType = id+"-v", item.ID, "video"
		require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{&row}))
	}

	ids, err := streamRepo.ListVideoKeyframesMissing(ctx, 100, nil, 1)
	require.NoError(t, err)

	for id, s := range streams {
		src := transcoding.CopySource{Codec: s.Codec, Profile: s.Profile, Level: s.Level, ColorTransfer: s.ColorTransfer}
		if s.Width != nil {
			src.Width, src.Height = *s.Width, *s.Height
		}
		assert.Equal(t, transcoding.CopyRungEligible(src), slices.Contains(ids, id),
			"%s: copy guard and keyframe candidate query disagree", id)
	}
	assert.Contains(t, ids, "vp9-p0-as-ffprobe", "the Sprite Fright shape is indexed")
	assert.NotContains(t, ids, "vp9-p2-no-transfer", "a VP9 that may be HDR stays out")

	cov, err := streamRepo.KeyframesCoverage(ctx, 1)
	require.NoError(t, err)
	assertCoveragePending(t, cov, len(ids))
}
