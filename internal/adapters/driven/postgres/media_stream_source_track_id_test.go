package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// The file's own track number survives every write path, and an unknown one
// comes back nil rather than 0: track numbers start at 1 in Matroska but a
// zero would still pair with a rendition that claims it.
func TestMediaStreamRepo_SourceTrackID_RoundTrips(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	repo := postgres.NewMediaStreamRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("trackidlib")
	require.NoError(t, libRepo.Create(ctx, lib))
	item := newTestMediaItem("trackidmi", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, item))
	other := newTestMediaItem("trackidmi2", lib.ID)
	require.NoError(t, itemRepo.Create(ctx, other))

	byID := func(streams []*library.MediaStream) map[string]*library.MediaStream {
		m := map[string]*library.MediaStream{}
		for _, s := range streams {
			m[s.ID] = s
		}
		return m
	}

	// CreateBatch, read back by both readers.
	require.NoError(t, repo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "t1", MediaItemID: item.ID, StreamIndex: 0, StreamType: "video", Codec: "h264", SourceTrackID: new(1)},
		{ID: "t2", MediaItemID: item.ID, StreamIndex: 1, StreamType: "audio", Codec: "aac", SourceTrackID: new(3)},
		{ID: "t3", MediaItemID: item.ID, StreamIndex: 2, StreamType: "subtitle", Codec: "subrip"},
	}))
	got, err := repo.GetByMediaItemID(ctx, item.ID)
	require.NoError(t, err)
	m := byID(got)
	require.NotNil(t, m["t1"].SourceTrackID)
	assert.Equal(t, 1, *m["t1"].SourceTrackID)
	require.NotNil(t, m["t2"].SourceTrackID)
	assert.Equal(t, 3, *m["t2"].SourceTrackID)
	assert.Nil(t, m["t3"].SourceTrackID, "an unknown track number stays NULL")

	many, err := repo.GetByMediaItemIDs(ctx, []shared.MediaItemID{item.ID})
	require.NoError(t, err)
	m = byID(many[item.ID])
	require.NotNil(t, m["t2"].SourceTrackID)
	assert.Equal(t, 3, *m["t2"].SourceTrackID)
	assert.Nil(t, m["t3"].SourceTrackID)

	// ReplaceBatch.
	require.NoError(t, repo.ReplaceBatch(ctx, other.ID, []*library.MediaStream{
		{ID: "r1", MediaItemID: other.ID, StreamIndex: 0, StreamType: "audio", Codec: "ac3", SourceTrackID: new(7)},
		{ID: "r2", MediaItemID: other.ID, StreamIndex: 1, StreamType: "audio", Codec: "aac"},
	}))
	got, err = repo.GetByMediaItemID(ctx, other.ID)
	require.NoError(t, err)
	m = byID(got)
	require.NotNil(t, m["r1"].SourceTrackID)
	assert.Equal(t, 7, *m["r1"].SourceTrackID)
	assert.Nil(t, m["r2"].SourceTrackID)

	// A refresh by a probe that reads the number fills a row stored without
	// it; a probe that does not read it keeps the stored one.
	res, err := repo.RefreshProbedFields(ctx, other.ID, []*library.MediaStream{
		{StreamIndex: 0, StreamType: "audio", Codec: "ac3"},
		{StreamIndex: 1, StreamType: "audio", Codec: "aac", SourceTrackID: new(9)},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Changed, "only the row that learnt its number is written")
	got, err = repo.GetByMediaItemID(ctx, other.ID)
	require.NoError(t, err)
	m = byID(got)
	require.NotNil(t, m["r1"].SourceTrackID)
	assert.Equal(t, 7, *m["r1"].SourceTrackID, "a probe without the number keeps the stored one")
	require.NotNil(t, m["r2"].SourceTrackID)
	assert.Equal(t, 9, *m["r2"].SourceTrackID)
}
