package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/feed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFeedHandler_ResumePlayback_Success(t *testing.T) {
	h, _, _, querier := newFeedHandler()
	querier.ResumePlaybackItems = []*feed.FeedItem{
		{MediaItemID: "m1", Title: "Wednesday"},
		{MediaItemID: "m2", Title: "The Bear"},
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.ResumePlayback(ctx, &eveyv1.ResumePlaybackRequest{Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
}

func TestFeedHandler_ResumePlayback_Unauthenticated(t *testing.T) {
	h, _, _, _ := newFeedHandler()
	_, err := h.ResumePlayback(context.Background(), &eveyv1.ResumePlaybackRequest{})
	require.Error(t, err)
}

// limit <= 0 defaults to 20.
func TestFeedHandler_ResumePlayback_DefaultLimit(t *testing.T) {
	h, _, _, querier := newFeedHandler()
	querier.ResumePlaybackItems = []*feed.FeedItem{{MediaItemID: "m1"}}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.ResumePlayback(ctx, &eveyv1.ResumePlaybackRequest{Limit: 0})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
}

func TestFeedHandler_NextEpisode_Success(t *testing.T) {
	h, _, _, querier := newFeedHandler()
	querier.NextEpisodeItems = []*feed.FeedItem{
		{MediaItemID: "e1", Title: "S01E05"},
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.NextEpisode(ctx, &eveyv1.NextEpisodeRequest{Limit: 5})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
}

func TestFeedHandler_NextEpisode_Unauthenticated(t *testing.T) {
	h, _, _, _ := newFeedHandler()
	_, err := h.NextEpisode(context.Background(), &eveyv1.NextEpisodeRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
