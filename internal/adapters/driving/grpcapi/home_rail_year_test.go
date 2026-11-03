package grpcapi

import (
	"errors"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recommended / similar / recently-added rails answer with FeedItem, not a
// full media item: the year has to ride on it or the card prints no year.
func TestFeedItemToProto_CarriesYear(t *testing.T) {
	t.Parallel()
	pb := feedItemToProto(&feed.FeedItem{MediaItemID: "mi-1", Title: "Film", Type: "movie", Year: 1999})
	assert.Equal(t, int32(1999), pb.Year)
	assert.Zero(t, feedItemToProto(&feed.FeedItem{MediaItemID: "mi-2"}).Year, "unknown year stays 0")
}

// The popular row resolves the media items anyway; their catalog year must
// reach the payload, and a missing catalog must not panic.
func TestActivityHandler_GetPopularMedia_CarriesYear(t *testing.T) {
	h, _, stats, _, _ := newActivityHandler()
	stats.MostPopular = []activity.MediaPlayCount{
		{MediaItemID: "mi-dated", Title: "Dated", PlayCount: 9},
		{MediaItemID: "mi-undated", Title: "Undated", PlayCount: 5},
	}
	year := 2004
	mediaRepo := libfakes.NewFakeMediaItemQueryRepo()
	mediaRepo.Seed(&library.MediaItem{ID: "mi-dated", LibraryID: "lib-1", Type: library.MovieItem, Title: "Dated",
		Catalog: &library.MediaCatalog{Year: &year}})
	mediaRepo.Seed(&library.MediaItem{ID: "mi-undated", LibraryID: "lib-1", Type: library.MovieItem, Title: "Undated"})
	h.libQuery = applibrary.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), mediaRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())

	resp, err := h.GetPopularMedia(ctxWithClaims("user-1", "profile-1", true), &eveyv1.GetPopularMediaRequest{Limit: 10})
	require.NoError(t, err)
	got := map[string]int32{}
	for _, it := range resp.Items {
		got[it.MediaItemId] = it.Year
	}
	assert.Equal(t, map[string]int32{"mi-dated": 2004, "mi-undated": 0}, got)
}

// The rating rides on the FeedItem for the same reason as the year: the
// recommended rail's card prints it and has nothing else to read it from.
func TestFeedItemToProto_CarriesCommunityRating(t *testing.T) {
	t.Parallel()
	pb := feedItemToProto(&feed.FeedItem{MediaItemID: "mi-1", Title: "Film", Type: "movie", CommunityRating: 7.4})
	assert.InDelta(t, 7.4, pb.CommunityRating, 1e-9)
	assert.Zero(t, feedItemToProto(&feed.FeedItem{MediaItemID: "mi-2"}).CommunityRating, "unknown rating stays 0")
}

// The popular rail draws the shared home card: it needs the poster
// placeholder and the rating like every other rail, from one batched artwork
// fetch, and an artwork failure only drops the placeholder.
func TestActivityHandler_GetPopularMedia_CarriesBlurhashAndRating(t *testing.T) {
	h, _, stats, _, _ := newActivityHandler()
	stats.MostPopular = []activity.MediaPlayCount{
		{MediaItemID: "mi-rated", Title: "Rated", PlayCount: 9},
		{MediaItemID: "mi-bare", Title: "Bare", PlayCount: 5},
	}
	mediaRepo := libfakes.NewFakeMediaItemQueryRepo()
	mediaRepo.Seed(&library.MediaItem{ID: "mi-rated", LibraryID: "lib-1", Type: library.MovieItem, Title: "Rated",
		Classification: &library.MediaClassification{CommunityRating: new(8.1)}})
	mediaRepo.Seed(&library.MediaItem{ID: "mi-bare", LibraryID: "lib-1", Type: library.MovieItem, Title: "Bare"})
	h.libQuery = applibrary.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), mediaRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex())
	artwork := &fakeArtworkBatch{records: map[string][]*metadata.ArtworkRecord{
		"mi-rated": {
			{Type: "backdrop", Blurhash: new("HASH-backdrop")},
			{Type: "poster", Blurhash: new("HASH-poster")},
		},
	}}
	h.artwork = artwork

	resp, err := h.GetPopularMedia(ctxWithClaims("user-1", "profile-1", true), &eveyv1.GetPopularMediaRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, "HASH-poster", resp.Items[0].PrimaryBlurhash)
	assert.InDelta(t, 8.1, resp.Items[0].CommunityRating, 1e-9)
	assert.Empty(t, resp.Items[1].PrimaryBlurhash)
	assert.Zero(t, resp.Items[1].CommunityRating)
	assert.Equal(t, 1, artwork.calls, "one batched fetch for the whole rail")

	artwork.err = errors.New("artwork down")
	resp, err = h.GetPopularMedia(ctxWithClaims("user-1", "profile-1", true), &eveyv1.GetPopularMediaRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Empty(t, resp.Items[0].PrimaryBlurhash)
	assert.InDelta(t, 8.1, resp.Items[0].CommunityRating, 1e-9)
}
