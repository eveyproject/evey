package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/playback"
)

func intp(v int) *int { return &v }

// seedShow seeds a series > season > episode chain and returns their IDs.
func seedShow(seed func(*library.MediaItem)) {
	seed(&library.MediaItem{ID: "mi-series", Title: "Lioness", Type: library.Series})
	seed(&library.MediaItem{ID: "mi-season", Title: "Season 1", Type: library.Season, ParentID: "mi-series", IndexNumber: intp(1)})
	seed(&library.MediaItem{ID: "mi-episode", Title: "The Spider and the Fly", Type: library.Episode, ParentID: "mi-season", IndexNumber: intp(5)})
}

func TestFillAncestorTitles(t *testing.T) {
	byID := map[string]*library.MediaItem{
		"mi-series": {ID: "mi-series", Title: "Lioness", Type: library.Series},
		"mi-season": {ID: "mi-season", Title: "Season 1", Type: library.Season, ParentID: "mi-series"},
	}
	fetch := func(_ context.Context, ids []string) ([]*library.MediaItem, error) {
		out := make([]*library.MediaItem, 0, len(ids))
		for _, id := range ids {
			if mi := byID[id]; mi != nil {
				out = append(out, mi)
			}
		}
		return out, nil
	}

	episode := &eveyv1.MediaItem{Id: "mi-episode", Type: eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE, ParentId: "mi-season"}
	season := &eveyv1.MediaItem{Id: "mi-season", Type: eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SEASON, ParentId: "mi-series"}
	movie := &eveyv1.MediaItem{Id: "mi-movie", Type: eveyv1.MediaItemType_MEDIA_ITEM_TYPE_MOVIE}
	fillAncestorTitles(context.Background(), fetch, []*eveyv1.MediaItem{episode, season, movie, nil})

	assert.Equal(t, "Lioness", episode.SeriesTitle)
	assert.Equal(t, "Season 1", episode.SeasonTitle)
	assert.Equal(t, "Lioness", season.SeriesTitle)
	assert.Empty(t, season.SeasonTitle)
	assert.Empty(t, movie.SeriesTitle)
}

func TestFillAncestorTitles_FetchErrorLeavesTitlesEmpty(t *testing.T) {
	fetch := func(context.Context, []string) ([]*library.MediaItem, error) {
		return nil, errors.New("db down")
	}
	episode := &eveyv1.MediaItem{Id: "mi-episode", Type: eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE, ParentId: "mi-season"}
	fillAncestorTitles(context.Background(), fetch, []*eveyv1.MediaItem{episode})
	assert.Empty(t, episode.SeriesTitle)
	assert.Empty(t, episode.SeasonTitle)
}

func TestPlaybackHandler_GetNextUp_StampsAncestorTitles(t *testing.T) {
	h, _, progressRepo, mediaQuery, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	seedShow(mediaQuery.Seed)
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-1", ProfileID: "profile-1", MediaItemID: "mi-episode",
		Position: mustPlaybackDurationMs(t, 30_000), Completed: true, LastPlayedAt: time.Now(),
	})

	resp, err := h.GetNextUp(ctx, &eveyv1.GetNextUpRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Lioness", resp.Items[0].SeriesTitle)
	assert.Equal(t, "Season 1", resp.Items[0].SeasonTitle)
}

func TestPlaybackHandler_GetResumeItems_StampsEpisodeDisplay(t *testing.T) {
	h, _, progressRepo, mediaQuery, _, _ := newTestPlaybackHandler()
	ctx := pbAdminCtx()

	seedShow(mediaQuery.Seed)
	progressRepo.Upsert(context.Background(), &playback.WatchProgress{
		ID: "wp-1", ProfileID: "profile-1", MediaItemID: "mi-episode",
		Position: mustPlaybackDurationMs(t, 30_000), LastPlayedAt: time.Now(),
	})

	resp, err := h.GetResumeItems(ctx, &eveyv1.GetResumeItemsRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Progress, 1)
	assert.Equal(t, "The Spider and the Fly", resp.Progress[0].Title)
	assert.Equal(t, "Lioness", resp.Progress[0].SeriesTitle)
	assert.Equal(t, int32(1), resp.Progress[0].ParentIndexNumber)
	assert.Equal(t, int32(5), resp.Progress[0].IndexNumber)
	// The declared-but-never-filled items list: the TV home row reads it.
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "The Spider and the Fly", resp.Items[0].Title)
	assert.Equal(t, "Lioness", resp.Items[0].SeriesTitle)
}

func TestLibraryHandler_GetMediaItem_StampsAncestorTitles(t *testing.T) {
	h, _, mediaQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	seedShow(mediaQuery.Seed)
	item, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-episode"})
	require.NoError(t, err)
	assert.Equal(t, "Lioness", item.SeriesTitle)
	assert.Equal(t, "Season 1", item.SeasonTitle)
}

func TestLibraryHandler_SearchMedia_StampsSeriesTitle(t *testing.T) {
	h, _, mediaQuery, searchIdx, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	seedShow(mediaQuery.Seed)
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID: "mi-episode", Title: "The Spider and the Fly", Type: library.Episode,
	}, nil)

	resp, err := h.SearchMedia(ctx, &eveyv1.SearchMediaRequest{Query: "Spider", Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Lioness", resp.Items[0].SeriesTitle)
	assert.Equal(t, "mi-season", resp.Items[0].ParentId)
}
