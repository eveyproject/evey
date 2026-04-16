package rating_test

import (
	"context"
	"testing"

	apprating "github.com/evey-project/evey/internal/app/rating"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	ratingfakes "github.com/evey-project/evey/internal/domain/rating/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UpsertThumb + GetThumb cycle with WithThumbRating wired.
func TestService_UpsertAndGetThumb(t *testing.T) {
	t.Parallel()
	thumbRepo := libfakes.NewFakeUserRatingRepository()
	repo := ratingfakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apprating.NewService(repo, bus, apprating.WithThumbRating(thumbRepo))

	ctx := context.Background()
	require.NoError(t, svc.UpsertThumb(ctx, &library.UserRating{
		ProfileID: "p1", MediaItemID: "m1", Value: 1,
	}))

	got, err := svc.GetThumb(ctx, "p1", "m1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, got.Value)
}

// Upserting the same profile+item overwrites the previous value.
func TestService_UpsertThumb_Overwrite(t *testing.T) {
	t.Parallel()
	thumbRepo := libfakes.NewFakeUserRatingRepository()
	repo := ratingfakes.NewFakeRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apprating.NewService(repo, bus, apprating.WithThumbRating(thumbRepo))

	ctx := context.Background()
	require.NoError(t, svc.UpsertThumb(ctx, &library.UserRating{ProfileID: "p", MediaItemID: "m", Value: 1}))
	require.NoError(t, svc.UpsertThumb(ctx, &library.UserRating{ProfileID: "p", MediaItemID: "m", Value: -1}))

	got, err := svc.GetThumb(ctx, "p", "m")
	require.NoError(t, err)
	assert.Equal(t, -1, got.Value)
}
