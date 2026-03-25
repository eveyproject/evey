package library_test

import (
	"context"
	"testing"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type querySvcFull struct {
	svc       *applib.QueryService
	media     *libfakes.FakeMediaItemQueryRepo
	libraries *libfakes.FakeLibraryQueryRepo
	streams   *libfakes.FakeMediaStreamRepository
	search    *libfakes.FakeSearchIndex
}

func newQuerySvcFull() *querySvcFull {
	media := libfakes.NewFakeMediaItemQueryRepo()
	libraries := libfakes.NewFakeLibraryQueryRepo()
	streams := libfakes.NewFakeMediaStreamRepository()
	search := libfakes.NewFakeSearchIndex()
	svc := applib.NewQueryService(libraries, media, streams, search)
	return &querySvcFull{svc: svc, media: media, libraries: libraries, streams: streams, search: search}
}

func TestQueryService_CountMediaItems_Empty(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	count, err := f.svc.CountMediaItems(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestQueryService_CountMediaItems_WithItems(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	lib1 := "lib-1"
	lib2 := "lib-2"
	f.media.Seed(&library.MediaItem{ID: "m1", LibraryID: lib1})
	f.media.Seed(&library.MediaItem{ID: "m2", LibraryID: lib1})
	f.media.Seed(&library.MediaItem{ID: "m3", LibraryID: lib2})

	count, err := f.svc.CountMediaItems(context.Background(), library.ListMediaItemsFilter{LibraryID: &lib1})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestQueryService_CountMediaItems_FilterByType(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	movieType := "movie"
	f.media.Seed(&library.MediaItem{ID: "m1", Type: "movie"})
	f.media.Seed(&library.MediaItem{ID: "m2", Type: "movie"})
	f.media.Seed(&library.MediaItem{ID: "m3", Type: "series"})

	count, err := f.svc.CountMediaItems(context.Background(), library.ListMediaItemsFilter{Type: &movieType})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestQueryService_GetCastForItems_MultipleItems(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.media.SeedCast("item-1", []library.CastMember{
		{PersonID: "p1", PersonName: "Alice", RoleType: "actor", SortOrder: 0},
	})
	f.media.SeedCast("item-2", []library.CastMember{
		{PersonID: "p2", PersonName: "Bob", RoleType: "director", SortOrder: 0},
	})

	castMap, err := f.svc.GetCastForItems(context.Background(), []string{"item-1", "item-2"})
	require.NoError(t, err)
	require.Len(t, castMap, 2)
	assert.Equal(t, "Alice", castMap["item-1"][0].PersonName)
	assert.Equal(t, "Bob", castMap["item-2"][0].PersonName)
}

func TestQueryService_GetCastForItems_UnknownItem(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	castMap, err := f.svc.GetCastForItems(context.Background(), []string{"no-such-item"})
	require.NoError(t, err)
	assert.Empty(t, castMap)
}

func TestQueryService_GetCastForItems_EmptySlice(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	castMap, err := f.svc.GetCastForItems(context.Background(), []string{})
	require.NoError(t, err)
	assert.Empty(t, castMap)
}

func TestQueryService_GetMediaStreams_NoStreams(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	streams, err := f.svc.GetMediaStreams(context.Background(), "nonexistent-item")
	require.NoError(t, err)
	assert.Empty(t, streams)
}

func TestQueryService_GetMediaStreams_WithStreams(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.streams.CreateBatch(context.Background(), []*library.MediaStream{
		{ID: "s1", MediaItemID: "item-1", StreamType: "video", Codec: "h264"},
		{ID: "s2", MediaItemID: "item-1", StreamType: "audio", Codec: "aac"},
	})

	streams, err := f.svc.GetMediaStreams(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, streams, 2)
}

func TestQueryService_GetMediaStreams_IsolatedByItem(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.streams.CreateBatch(context.Background(), []*library.MediaStream{
		{ID: "s1", MediaItemID: "item-1", StreamType: "video", Codec: "h264"},
		{ID: "s2", MediaItemID: "item-2", StreamType: "audio", Codec: "aac"},
	})

	streams, err := f.svc.GetMediaStreams(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, streams, 1)
	assert.Equal(t, "h264", streams[0].Codec)
}

func TestQueryService_SearchMedia_Finds(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m1", Title: "Batman Begins"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m2", Title: "Batman Returns"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m3", Title: "Superman"}, nil)

	hits, total, err := f.svc.SearchMedia(context.Background(), "batman", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, hits, 2)
}

func TestQueryService_SearchMedia_LimitApplied(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m1", Title: "Batman Begins"}, nil)
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{ID: "m2", Title: "Batman Returns"}, nil)

	hits, _, err := f.svc.SearchMedia(context.Background(), "batman", nil, nil, 1)
	require.NoError(t, err)
	assert.Len(t, hits, 1)
}

func TestQueryService_SearchMedia_NoResults(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	hits, total, err := f.svc.SearchMedia(context.Background(), "xyzzy-notfound", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, hits)
}

func TestQueryService_GetLibrary_NotFound(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	_, err := f.svc.GetLibrary(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestQueryService_ListMediaItems_Pagination(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	for i := range 5 {
		id := "m" + string(rune('A'+i))
		f.media.Seed(&library.MediaItem{ID: id, Title: "Film " + id})
	}

	page1, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	page2, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, page2, 2)

	page3, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{Limit: 2, Offset: 4})
	require.NoError(t, err)
	assert.Len(t, page3, 1)
}

func TestQueryService_ListMediaItems_FilterByLibraryID(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	lib1 := "lib-1"
	lib2 := "lib-2"
	f.media.Seed(&library.MediaItem{ID: "m1", LibraryID: lib1, Title: "Film A"})
	f.media.Seed(&library.MediaItem{ID: "m2", LibraryID: lib2, Title: "Film B"})
	f.media.Seed(&library.MediaItem{ID: "m3", LibraryID: lib1, Title: "Film C"})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{LibraryID: &lib1})
	require.NoError(t, err)
	assert.Len(t, items, 2)
	for _, item := range items {
		assert.Equal(t, lib1, item.LibraryID)
	}
}

func TestQueryService_ListMediaItems_FilterByType(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	movieType := "movie"
	f.media.Seed(&library.MediaItem{ID: "m1", Type: "movie", Title: "Film A"})
	f.media.Seed(&library.MediaItem{ID: "m2", Type: "series", Title: "Show B"})
	f.media.Seed(&library.MediaItem{ID: "m3", Type: "movie", Title: "Film C"})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{Type: &movieType})
	require.NoError(t, err)
	assert.Len(t, items, 2)
	for _, item := range items {
		assert.Equal(t, "movie", item.Type)
	}
}

func TestQueryService_ListMediaItems_FilterByYearRange(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	y2000, y2010, y2020 := 2000, 2010, 2020
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "Old Film", Year: &y2000})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "Mid Film", Year: &y2010})
	f.media.Seed(&library.MediaItem{ID: "m3", Title: "New Film", Year: &y2020})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{YearFrom: 2005, YearTo: 2015})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "m2", items[0].ID)
}

func TestQueryService_ListMediaItems_FilterByMinRating(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	low, high := 3.0, 8.5
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "Low Rated", CommunityRating: &low})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "High Rated", CommunityRating: &high})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{MinRating: 7.0})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "m2", items[0].ID)
}

func TestQueryService_GetLibrary_Found(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.libraries.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})

	lib, err := f.svc.GetLibrary(context.Background(), "lib-1")
	require.NoError(t, err)
	assert.Equal(t, "lib-1", lib.ID)
	assert.Equal(t, "Movies", lib.Name)
	assert.Equal(t, "movie", lib.Type)
}

func TestQueryService_ListLibraries_Empty(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	libs, err := f.svc.ListLibraries(context.Background())
	require.NoError(t, err)
	assert.Empty(t, libs)
}

func TestQueryService_ListLibraries_Multiple(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.libraries.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	f.libraries.Seed(&library.Library{ID: "lib-2", Name: "TV Shows", Type: "tvshow"})

	libs, err := f.svc.ListLibraries(context.Background())
	require.NoError(t, err)
	assert.Len(t, libs, 2)
	ids := []string{libs[0].ID, libs[1].ID}
	assert.Contains(t, ids, "lib-1")
	assert.Contains(t, ids, "lib-2")
}

func TestQueryService_GetMediaItem_Found(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	year := 2020
	rating := 8.5
	f.media.Seed(&library.MediaItem{
		ID: "m1", Title: "Inception", Type: "movie",
		LibraryID: "lib-1", Year: &year, CommunityRating: &rating,
		Genres: []string{"sci-fi", "thriller"},
	})

	item, err := f.svc.GetMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.Equal(t, "m1", item.ID)
	assert.Equal(t, "Inception", item.Title)
	assert.Equal(t, "movie", item.Type)
	assert.Equal(t, "lib-1", item.LibraryID)
	assert.Equal(t, 2020, *item.Year)
	assert.InEpsilon(t, 8.5, *item.CommunityRating, 1e-9)
	assert.Equal(t, []string{"sci-fi", "thriller"}, item.Genres)
}

func TestQueryService_GetMediaItem_NotFound(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	_, err := f.svc.GetMediaItem(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestQueryService_GetCast_SingleItem(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	charName := "Bruce Wayne"
	f.media.SeedCast("item-1", []library.CastMember{
		{PersonID: "p1", PersonName: "Christian Bale", RoleType: "actor", CharacterName: &charName, SortOrder: 0},
		{PersonID: "p2", PersonName: "Christopher Nolan", RoleType: "director", SortOrder: 1},
	})

	cast, err := f.svc.GetCast(context.Background(), "item-1")
	require.NoError(t, err)
	assert.Len(t, cast, 2)
	assert.Equal(t, "Christian Bale", cast[0].PersonName)
	assert.Equal(t, "actor", cast[0].RoleType)
	assert.Equal(t, "Bruce Wayne", *cast[0].CharacterName)
	assert.Equal(t, "Christopher Nolan", cast[1].PersonName)
	assert.Equal(t, "director", cast[1].RoleType)
}

func TestQueryService_GetCast_NoCast(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	cast, err := f.svc.GetCast(context.Background(), "no-item")
	require.NoError(t, err)
	assert.Empty(t, cast)
}

func TestQueryService_GetMediaStreams_AssertStreamContent(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	width, height := 1920, 1080
	channels := 6
	f.streams.CreateBatch(context.Background(), []*library.MediaStream{
		{ID: "s1", MediaItemID: "item-1", StreamType: "video", Codec: "hevc", Width: &width, Height: &height},
		{ID: "s2", MediaItemID: "item-1", StreamType: "audio", Codec: "aac", Channels: &channels},
	})

	streams, err := f.svc.GetMediaStreams(context.Background(), "item-1")
	require.NoError(t, err)
	require.Len(t, streams, 2)
	var video, audio *library.MediaStream
	for _, s := range streams {
		if s.StreamType == "video" {
			video = s
		} else {
			audio = s
		}
	}
	require.NotNil(t, video)
	assert.Equal(t, "hevc", video.Codec)
	assert.Equal(t, 1920, *video.Width)
	assert.Equal(t, 1080, *video.Height)
	require.NotNil(t, audio)
	assert.Equal(t, "aac", audio.Codec)
	assert.Equal(t, 6, *audio.Channels)
}

func TestQueryService_SearchMedia_HitContent(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	year := 2005
	f.search.IndexMediaItem(context.Background(), &library.MediaItem{
		ID: "m1", Title: "Batman Begins", Type: "movie",
		Year: &year, Genres: []string{"action"}, OfficialRating: "PG-13",
	}, nil)

	hits, total, err := f.svc.SearchMedia(context.Background(), "batman", nil, nil, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, hits, 1)
	assert.Equal(t, "m1", hits[0].ID)
	assert.Equal(t, "Batman Begins", hits[0].Title)
	assert.Equal(t, "movie", hits[0].Type)
	assert.Equal(t, 2005, *hits[0].Year)
	assert.Equal(t, []string{"action"}, hits[0].Genres)
	assert.Equal(t, "PG-13", hits[0].OfficialRating)
}

func TestQueryService_ListMediaItems_AllNoFilter(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	f.media.Seed(&library.MediaItem{ID: "m1", Title: "A"})
	f.media.Seed(&library.MediaItem{ID: "m2", Title: "B"})

	items, err := f.svc.ListMediaItems(context.Background(), library.ListMediaItemsFilter{})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestQueryService_CountMediaItems_FilterByParentID(t *testing.T) {
	t.Parallel()
	f := newQuerySvcFull()
	parent := "series-1"
	f.media.Seed(&library.MediaItem{ID: "ep1", ParentID: parent, Type: "episode"})
	f.media.Seed(&library.MediaItem{ID: "ep2", ParentID: parent, Type: "episode"})
	f.media.Seed(&library.MediaItem{ID: "ep3", ParentID: "series-2", Type: "episode"})

	count, err := f.svc.CountMediaItems(context.Background(), library.ListMediaItemsFilter{ParentID: &parent})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
