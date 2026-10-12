package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The 1-10 star-rating aggregate (the media_ratings table) was removed; thumbs
// (like/dislike) are the single rating model. GetRatingAggregate is kept as an
// empty stub for proto/client compatibility (no frontend caller today) until a
// deliberate proto cleanup drops the RPC.
func TestRatingHandler_GetRatingAggregate_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.seedMedia("m1", "lib-1")
	_ = f.libraryAccess.Grant(context.Background(), "user-1", "lib-1")

	callerCtx := authedCtx("user-1", "profile-1", false)
	resp, err := f.handler.GetRatingAggregate(callerCtx, &eveyv1.GetRatingAggregateRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.Zero(t, resp.Average)
	assert.Equal(t, int32(0), resp.Count)
}

func TestRatingHandler_GetRatingAggregate_Unauthenticated(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	_, err := f.handler.GetRatingAggregate(context.Background(), &eveyv1.GetRatingAggregateRequest{MediaItemId: "m1"})
	require.Error(t, err)
}
