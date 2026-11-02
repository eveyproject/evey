package grpcapi

import (
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/feed"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
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
