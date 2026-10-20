package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/playback"
)

// seedShow seeds a series > season > episode chain.
func seedShow(seed func(*library.MediaItem)) {
	seed(&library.MediaItem{ID: "mi-series", Title: "Some Series", Type: library.Series})
	seed(&library.MediaItem{ID: "mi-season", Title: "Season 1", Type: library.Season, ParentID: "mi-series", IndexNumber: new(1)})
	seed(&library.MediaItem{ID: "mi-episode", Title: "Some Episode", Type: library.Episode, ParentID: "mi-season", IndexNumber: new(5)})
}

func TestFillAncestorTitles(t *testing.T) {
	byID := map[string]*library.MediaItem{
		"mi-series": {ID: "mi-series", Title: "Some Series", Type: library.Series},
		"mi-season": {ID: "mi-season", Title: "Season 1", Type: library.Season, ParentID: "mi-series", IndexNumber: new(2)},
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

	assert.Equal(t, "Some Series", episode.SeriesTitle)
	assert.Equal(t, "Season 1", episode.SeasonTitle)
	assert.Equal(t, int32(2), episode.SeasonIndexNumber)
	assert.Zero(t, season.SeasonIndexNumber)
	assert.Equal(t, "Some Series", season.SeriesTitle)
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
	assert.Equal(t, "Some Series", resp.Items[0].SeriesTitle)
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
	assert.Equal(t, "Some Episode", resp.Progress[0].Title)
	assert.Equal(t, "Some Series", resp.Progress[0].SeriesTitle)
	assert.Equal(t, int32(1), resp.Progress[0].ParentIndexNumber)
	assert.Equal(t, int32(5), resp.Progress[0].IndexNumber)
	// The declared-but-never-filled items list: the TV home row reads it.
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Some Episode", resp.Items[0].Title)
	assert.Equal(t, "Some Series", resp.Items[0].SeriesTitle)
}

func TestLibraryHandler_GetMediaItem_StampsAncestorTitles(t *testing.T) {
	h, _, mediaQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	seedShow(mediaQuery.Seed)
	item, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-episode"})
	require.NoError(t, err)
	assert.Equal(t, "Some Series", item.SeriesTitle)
	assert.Equal(t, "Season 1", item.SeasonTitle)
}

func TestLibraryHandler_SearchMedia_StampsSeriesTitle(t *testing.T) {
	h, _, mediaQuery, searchIdx, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	seedShow(mediaQuery.Seed)
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID: "mi-episode", Title: "Some Episode", Type: library.Episode,
	})

	resp, err := h.SearchMedia(ctx, &eveyv1.SearchMediaRequest{Query: "Some Episode", Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Some Series", resp.Items[0].SeriesTitle)
	assert.Equal(t, "mi-season", resp.Items[0].ParentId)
}

// Une SAISON trouvee par la recherche s'affichait « Season 2 » en anglais au
// milieu d'une page francaise. Cause : l'hydratation des resultats ne visait
// que les EPISODES, donc une saison repartait sans `index_number` (le document
// Meilisearch n'en porte pas) et `seasonLabel` cote client n'avait rien a
// deriver - il retombait sur le titre GELE par le scan. Meme cause pour le
// titre de serie manquant : `fillAncestorTitles` n'etait appele que s'il y
// avait au moins un episode dans la page.
func TestLibraryHandler_SearchMedia_HydratesSeasonNumberAndSeries(t *testing.T) {
	h, _, mediaQuery, searchIdx, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	seedShow(mediaQuery.Seed)
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID: "mi-season", Title: "Season 1", Type: library.Season,
	})

	resp, err := h.SearchMedia(ctx, &eveyv1.SearchMediaRequest{Query: "Season 1", Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, int32(1), resp.Items[0].IndexNumber, "sans le numero le client ne peut rien traduire")
	assert.Equal(t, "mi-series", resp.Items[0].ParentId)
	assert.Equal(t, "Some Series", resp.Items[0].SeriesTitle)
}
