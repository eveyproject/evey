package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func facetVideo(id string, item shared.MediaItemID, idx int, codec string, width, height int) *library.MediaStream {
	return &library.MediaStream{ID: id, MediaItemID: item, StreamIndex: idx, StreamType: library.StreamTypeVideo,
		Codec: codec, Width: &width, Height: &height}
}

func facetLang(id string, item shared.MediaItemID, idx int, kind library.StreamType, lang string) *library.MediaStream {
	return &library.MediaStream{ID: id, MediaItemID: item, StreamIndex: idx, StreamType: kind, Codec: "x", Language: lang}
}

// One pass over a library's streams returns its distinct values: scoped to the
// library, narrowed by type and by the parental pre-filter the grid applies,
// and restricted to an explicit item set when one is given.
func TestMediaItemQueryRepoAdapter_ListStreamFacets(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	streamRepo := postgres.NewMediaStreamRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	lib := newTestLibrary("fq-lib")
	require.NoError(t, libRepo.Create(ctx, lib))
	other := newTestLibrary("fq-other")
	require.NoError(t, libRepo.Create(ctx, other))

	pg := newTestMediaItem("fq-pg", lib.ID)
	pg.Classification = &library.MediaClassification{OfficialRating: "PG"}
	require.NoError(t, itemRepo.Create(ctx, pg))
	r := newTestMediaItem("fq-r", lib.ID)
	r.Classification = &library.MediaClassification{OfficialRating: "R"}
	require.NoError(t, itemRepo.Create(ctx, r))
	ep := newTestMediaItem("fq-ep", lib.ID)
	ep.Type = library.Episode
	require.NoError(t, itemRepo.Create(ctx, ep))
	leak := newTestMediaItem("fq-leak", other.ID)
	require.NoError(t, itemRepo.Create(ctx, leak))

	require.NoError(t, streamRepo.CreateBatch(ctx, []*library.MediaStream{
		facetVideo("fq-pg-v", pg.ID, 0, "h264", 1920, 800),
		facetLang("fq-pg-a1", pg.ID, 1, library.StreamTypeAudio, "fre"),
		facetLang("fq-pg-a2", pg.ID, 2, library.StreamTypeAudio, "und"),
		facetLang("fq-pg-s1", pg.ID, 3, library.StreamTypeSubtitle, "eng"),
		facetLang("fq-pg-s2", pg.ID, 4, library.StreamTypeSubtitle, ""),
		facetVideo("fq-r-v", r.ID, 0, "hevc", 3840, 2160),
		facetLang("fq-r-a", r.ID, 1, library.StreamTypeAudio, "fre"),
		facetLang("fq-r-s", r.ID, 2, library.StreamTypeSubtitle, "jpn"),
		facetVideo("fq-ep-v", ep.ID, 0, "h264", 1920, 800),
		facetVideo("fq-ep-v0", ep.ID, 1, "", 0, 0),
		facetLang("fq-ep-a", ep.ID, 2, library.StreamTypeAudio, "kor"),
		facetVideo("fq-leak-v", leak.ID, 0, "av1", 1280, 720),
		facetLang("fq-leak-a", leak.ID, 1, library.StreamTypeAudio, "rus"),
	}))

	libID := lib.ID
	all, err := queryRepo.ListStreamFacets(ctx, library.ListMediaItemsFilter{LibraryID: &libID})
	require.NoError(t, err)
	assert.Equal(t, []string{"h264", "hevc"}, all.VideoCodecs, "distinct, sorted, no empty codec, no other library")
	assert.Equal(t, []string{"fre", "kor"}, all.AudioLanguages, "und is not a language")
	assert.Equal(t, []string{"eng", "jpn"}, all.SubtitleLanguages)
	assert.ElementsMatch(t, []library.VideoSize{{Width: 1920, Height: 800}, {Width: 3840, Height: 2160}}, all.VideoSizes,
		"one row per distinct size, a stream without a height left out")
	assert.Equal(t, []int{2160, 1080}, all.ResolutionTiers())

	movie := string(library.MovieItem)
	movies, err := queryRepo.ListStreamFacets(ctx, library.ListMediaItemsFilter{LibraryID: &libID, Type: &movie})
	require.NoError(t, err)
	assert.Equal(t, []string{"fre"}, movies.AudioLanguages, "the episode's Korean audio is not a movie's")

	// The SQL pre-filter the grid applies for a 13-year-old: R is out.
	kids, err := queryRepo.ListStreamFacets(ctx, library.ListMediaItemsFilter{
		LibraryID: &libID, AllowedRatings: []string{"G", "PG", "PG-13"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"h264"}, kids.VideoCodecs)
	assert.Equal(t, []string{"eng"}, kids.SubtitleLanguages)

	// The whole parental rule: the unrated episode the pre-filter let through
	// is hidden too, so its Korean audio is no chip for a kids profile.
	strict, err := queryRepo.ListStreamFacets(ctx, library.ListMediaItemsFilter{
		LibraryID: &libID, Parental: &library.ParentalRule{MaxAge: 13, Region: "US"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"h264"}, strict.VideoCodecs)
	assert.Equal(t, []string{"fre"}, strict.AudioLanguages)
	assert.Equal(t, []string{"eng"}, strict.SubtitleLanguages)
}

// The grouped count answers, per library, exactly what Count answers for that
// library alone, in one query.
func TestMediaItemQueryRepoAdapter_CountPerLibrary(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	ctx := context.Background()
	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	queryRepo := postgres.NewMediaItemQueryRepoAdapter(db)

	a := newTestLibrary("cpl-a")
	require.NoError(t, libRepo.Create(ctx, a))
	b := newTestLibrary("cpl-b")
	require.NoError(t, libRepo.Create(ctx, b))
	empty := newTestLibrary("cpl-empty")
	require.NoError(t, libRepo.Create(ctx, empty))
	unlisted := newTestLibrary("cpl-unlisted")
	require.NoError(t, libRepo.Create(ctx, unlisted))

	for i, rating := range []string{"PG", "R", ""} {
		it := newTestMediaItem("cpl-a-"+string(rune('0'+i)), a.ID)
		if rating != "" {
			it.Classification = &library.MediaClassification{OfficialRating: rating}
		}
		require.NoError(t, itemRepo.Create(ctx, it))
	}
	series := newTestMediaItem("cpl-b-series", b.ID)
	series.Type = library.Series
	require.NoError(t, itemRepo.Create(ctx, series))
	episode := newTestMediaItem("cpl-b-episode", b.ID)
	episode.Type = library.Episode
	require.NoError(t, itemRepo.Create(ctx, episode))
	require.NoError(t, itemRepo.Create(ctx, newTestMediaItem("cpl-unlisted-1", unlisted.ID)))

	ids := []shared.LibraryID{a.ID, b.ID, empty.ID}
	for _, allowed := range [][]string{nil, {"G", "PG", "PG-13"}} {
		counts, err := queryRepo.CountPerLibrary(ctx, library.ListMediaItemsFilter{LibraryIDs: ids, AllowedRatings: allowed})
		require.NoError(t, err)
		for _, id := range ids {
			libID := id
			want, err := queryRepo.Count(ctx, library.ListMediaItemsFilter{LibraryID: &libID, AllowedRatings: allowed})
			require.NoError(t, err)
			assert.Equal(t, want, counts[id], "allowed=%v %s", allowed, id)
		}
		_, listed := counts[unlisted.ID]
		assert.False(t, listed, "a library not asked for is not counted")
		_, hasEmpty := counts[empty.ID]
		assert.False(t, hasEmpty, "a library with no item is absent, read as 0")
	}

	counts, err := queryRepo.CountPerLibrary(ctx, library.ListMediaItemsFilter{})
	require.NoError(t, err)
	assert.Empty(t, counts, "no library asked for counts nothing, not every library")
}
