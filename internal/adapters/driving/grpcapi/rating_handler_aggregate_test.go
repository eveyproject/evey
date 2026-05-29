package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/rating"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustRatingValue(v int) rating.RatingValue {
	rv, err := rating.NewRatingValue(v)
	if err != nil {
		panic(err)
	}
	return rv
}

// (Star rating RPC was removed when we ripped the star UI — only thumbs
// up/down remain. Aggregate read still serves /admin and series detail.)

// GetRatingAggregate returns the average/count for a media item.
func TestRatingHandler_GetRatingAggregate_Success(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	f.mediaItems.Seed(&library.MediaItem{ID: "m1", LibraryID: "lib-1"})
	// Seed 2 ratings for aggregation.
	ctx := context.Background()
	_ = f.starRepo.Upsert(ctx, rating.Rating{MediaItemID: "m1", ProfileID: "p1", Value: mustRatingValue(5)})
	_ = f.starRepo.Upsert(ctx, rating.Rating{MediaItemID: "m1", ProfileID: "p2", Value: mustRatingValue(3)})

	callerCtx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := f.handler.GetRatingAggregate(callerCtx, &eveyv1.GetRatingAggregateRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, resp.Average, 3.0)
	assert.LessOrEqual(t, resp.Average, 5.0)
}

func TestRatingHandler_GetRatingAggregate_Unauthenticated(t *testing.T) {
	t.Parallel()
	f := newRatingFixture()
	_, err := f.handler.GetRatingAggregate(context.Background(), &eveyv1.GetRatingAggregateRequest{MediaItemId: "m1"})
	require.Error(t, err)
}
