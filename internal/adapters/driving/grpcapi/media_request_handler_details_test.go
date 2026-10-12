package grpcapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/metadata"
)

// The release date crosses the wire as the provider's own YYYY-MM-DD, in both
// directions; the rating as a plain number. A malformed date from a client is
// dropped, not refused - it is decoration copied from a search result.
func TestMediaRequestHandler_DetailsRoundTrip(t *testing.T) {
	premiere := time.Date(1999, 3, 31, 0, 0, 0, 0, time.UTC)
	score := 8.7
	provider := &cannedProvider{results: []metadata.MetadataResult{{
		Title: "Some Movie", Overview: "plot", PremiereDate: &premiere, CommunityRating: &score,
		ProviderIDs: map[string]string{"tmdb": "603"},
	}}}
	h := newSearchHandler(newFakeMediaRequestAccess(true), provider)
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.SearchTitles(ctx, &eveyv1.SearchTitlesRequest{MediaType: "movie", Query: "some"})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "1999-03-31", resp.Results[0].ReleaseDate)
	assert.InDelta(t, 8.7, resp.Results[0].Rating, 1e-9)

	created, err := h.CreateRequest(ctx, &eveyv1.CreateRequestRequest{
		MediaType: "movie", Provider: "tmdb", ProviderId: "603", Title: "Some Movie",
		Overview: "plot", ReleaseDate: "1999-03-31", Rating: 8.7,
	})
	require.NoError(t, err)
	assert.Equal(t, "plot", created.Overview)
	assert.Equal(t, "1999-03-31", created.ReleaseDate)
	assert.InDelta(t, 8.7, created.Rating, 1e-9)

	listed, err := h.ListMyRequests(ctx, &eveyv1.ListMyRequestsRequest{})
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	assert.Equal(t, "1999-03-31", listed.Items[0].ReleaseDate)

	malformed, err := h.CreateRequest(ctx, &eveyv1.CreateRequestRequest{
		MediaType: "tv", Provider: "tmdb", ProviderId: "1399", Title: "Other", ReleaseDate: "31/03/1999",
	})
	require.NoError(t, err)
	assert.Empty(t, malformed.ReleaseDate, "a date the server cannot read is dropped, not stored")
	assert.InDelta(t, 0.0, malformed.Rating, 0)
}
