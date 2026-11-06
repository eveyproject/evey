package fakes_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/library/fakes"
)

// The fake keeps the Postgres refresh contract for the file's track number:
// a probe that reads it fills it, one that does not keeps the stored value.
func TestFakeMediaStreamRepo_RefreshProbedFields_SourceTrackID(t *testing.T) {
	repo := fakes.NewFakeMediaStreamRepository()
	ctx := context.Background()
	require.NoError(t, repo.CreateBatch(ctx, []*library.MediaStream{
		{ID: "a", MediaItemID: "mi", StreamIndex: 0, StreamType: "audio", Codec: "ac3", SourceTrackID: new(2)},
		{ID: "b", MediaItemID: "mi", StreamIndex: 1, StreamType: "audio", Codec: "aac"},
	}))

	res, err := repo.RefreshProbedFields(ctx, "mi", []*library.MediaStream{
		{StreamIndex: 0, StreamType: "audio", Codec: "ac3"},
		{StreamIndex: 1, StreamType: "audio", Codec: "aac", SourceTrackID: new(4)},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Changed)

	got, err := repo.GetByMediaItemID(ctx, "mi")
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.NotNil(t, got[0].SourceTrackID)
	assert.Equal(t, 2, *got[0].SourceTrackID)
	require.NotNil(t, got[1].SourceTrackID)
	assert.Equal(t, 4, *got[1].SourceTrackID)
}
