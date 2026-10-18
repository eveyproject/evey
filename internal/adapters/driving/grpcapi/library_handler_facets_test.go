package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

func videoStreamOf(codec string, width, height int) *library.MediaStream {
	return &library.MediaStream{StreamType: library.StreamTypeVideo, Codec: codec, Width: &width, Height: &height}
}

func langStream(kind library.StreamType, lang string) *library.MediaStream {
	return &library.MediaStream{StreamType: kind, Language: lang}
}

// seedFacetItem seeds one item and the streams the facet query reads off it.
func seedFacetItem(repo *libfakes.FakeMediaItemQueryRepo, item *library.MediaItem, streams ...*library.MediaStream) {
	repo.Seed(item)
	repo.SeedStreams(string(item.ID), streams)
}

func seedKidsProfile(t *testing.T, profiles *userfakes.FakeProfileRepository, libraryAccess *userfakes.FakeLibraryAccessRepository) {
	t.Helper()
	ctx := context.Background()
	maxAge := 13
	require.NoError(t, profiles.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}}))
	require.NoError(t, libraryAccess.Grant(ctx, "user-2", "lib-1"))
}

// The panel's chips are the library's, not a sample's: every item counts,
// values are distinct and sorted, "und" and empty tags are not languages, and
// a scope 1920x800 file is a 1080p tier, not a 720p one.
func TestLibraryHandler_GetLibraryFilterFacets_WholeLibraryDistinctValues(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-1"))

	seedFacetItem(mediaQuery, &library.MediaItem{ID: "m-1", LibraryID: "lib-1", Type: "movie", Title: "A"},
		videoStreamOf("hevc", 3840, 2160),
		langStream(library.StreamTypeAudio, "fre"),
		langStream(library.StreamTypeAudio, "und"),
		langStream(library.StreamTypeSubtitle, "eng"))
	seedFacetItem(mediaQuery, &library.MediaItem{ID: "m-2", LibraryID: "lib-1", Type: "movie", Title: "B"},
		videoStreamOf("h264", 1920, 800),
		langStream(library.StreamTypeAudio, "jpn"),
		langStream(library.StreamTypeAudio, "fre"),
		langStream(library.StreamTypeSubtitle, ""))
	seedFacetItem(mediaQuery, &library.MediaItem{ID: "m-3", LibraryID: "lib-1", Type: "movie", Title: "C"},
		videoStreamOf("h264", 720, 576))
	// Another library's values never show up in this one's panel.
	seedFacetItem(mediaQuery, &library.MediaItem{ID: "o-1", LibraryID: "lib-2", Type: "movie", Title: "Other"},
		videoStreamOf("av1", 1280, 720), langStream(library.StreamTypeAudio, "kor"))

	resp, err := h.GetLibraryFilterFacets(userCtx("user-2", "profile-2"), &eveyv1.GetLibraryFilterFacetsRequest{LibraryId: "lib-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"h264", "hevc"}, resp.VideoCodecs)
	assert.Equal(t, []string{"fre", "jpn"}, resp.AudioLanguages)
	assert.Equal(t, []string{"eng"}, resp.SubtitleLanguages)
	assert.Equal(t, []int32{2160, 1080, 0}, resp.ResolutionTiers, "width-aware tiers, highest first, 0 below 720p")
}

// The type narrows the facets the way it narrows the list the panel filters.
func TestLibraryHandler_GetLibraryFilterFacets_TypeNarrows(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-1"))

	seedFacetItem(mediaQuery, &library.MediaItem{ID: "s-1", LibraryID: "lib-1", Type: "series", Title: "Show"})
	seedFacetItem(mediaQuery, &library.MediaItem{ID: "e-1", LibraryID: "lib-1", Type: "episode", Title: "Ep", ParentID: "s-1"},
		videoStreamOf("h264", 1920, 1080), langStream(library.StreamTypeAudio, "eng"))

	resp, err := h.GetLibraryFilterFacets(userCtx("user-2", "profile-2"), &eveyv1.GetLibraryFilterFacetsRequest{
		LibraryId: "lib-1", Type: eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SERIES,
	})
	require.NoError(t, err)
	assert.Empty(t, resp.VideoCodecs, "series rows carry no streams: the series grid has nothing to facet on")

	resp, err = h.GetLibraryFilterFacets(userCtx("user-2", "profile-2"), &eveyv1.GetLibraryFilterFacetsRequest{
		LibraryId: "lib-1", Type: eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"h264"}, resp.VideoCodecs)
	assert.Equal(t, []string{"eng"}, resp.AudioLanguages)
}

// IDOR: a library the caller holds no grant on answers an empty set, like
// ListGenres, and reveals nothing of what it holds.
func TestLibraryHandler_GetLibraryFilterFacets_IDOR_Denied(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-1"))
	seedFacetItem(mediaQuery, &library.MediaItem{ID: "x-1", LibraryID: "lib-secret", Type: "movie", Title: "Secret"},
		videoStreamOf("hevc", 3840, 2160), langStream(library.StreamTypeAudio, "rus"))

	resp, err := h.GetLibraryFilterFacets(userCtx("user-2", "profile-2"), &eveyv1.GetLibraryFilterFacetsRequest{LibraryId: "lib-secret"})
	require.NoError(t, err)
	assert.Empty(t, resp.VideoCodecs)
	assert.Empty(t, resp.AudioLanguages)
	assert.Empty(t, resp.SubtitleLanguages)
	assert.Empty(t, resp.ResolutionTiers)
}

func TestLibraryHandler_GetLibraryFilterFacets_MissingLibraryID(t *testing.T) {
	h, _, _, _, _, _, _ := newTestLibraryHandler()
	_, err := h.GetLibraryFilterFacets(userCtx("user-2", "profile-2"), &eveyv1.GetLibraryFilterFacetsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// A kids profile sees the facets of the titles it may list, after BOTH
// parental passes. The unrated item is the case the SQL pre-filter lets
// through (empty official_rating) and only the strict pass blocks: its
// Japanese audio must not appear as a chip that would then select nothing.
func TestLibraryHandler_GetLibraryFilterFacets_KidsSeesOnlyVisibleTitles(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, profiles, _ := newTestLibraryHandler()
	seedKidsProfile(t, profiles, libraryAccess)

	seedFacetItem(mediaQuery, &library.MediaItem{ID: "k-pg", LibraryID: "lib-1", Type: "movie", Title: "PG",
		Classification: &library.MediaClassification{OfficialRating: "PG"}},
		videoStreamOf("h264", 1920, 1080), langStream(library.StreamTypeAudio, "fre"))
	seedFacetItem(mediaQuery, &library.MediaItem{ID: "k-r", LibraryID: "lib-1", Type: "movie", Title: "R",
		Classification: &library.MediaClassification{OfficialRating: "R"}},
		videoStreamOf("hevc", 3840, 2160), langStream(library.StreamTypeAudio, "kor"))
	seedFacetItem(mediaQuery, &library.MediaItem{ID: "k-unrated", LibraryID: "lib-1", Type: "movie", Title: "Unrated"},
		videoStreamOf("av1", 1280, 720), langStream(library.StreamTypeAudio, "jpn"),
		langStream(library.StreamTypeSubtitle, "jpn"))

	resp, err := h.GetLibraryFilterFacets(userCtx("user-2", "profile-2"), &eveyv1.GetLibraryFilterFacetsRequest{LibraryId: "lib-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"h264"}, resp.VideoCodecs)
	assert.Equal(t, []string{"fre"}, resp.AudioLanguages)
	assert.Empty(t, resp.SubtitleLanguages)
	assert.Equal(t, []int32{1080}, resp.ResolutionTiers)

	// The same profile's list agrees: what the chips offer is what it can see.
	list, err := h.ListMediaItems(userCtx("user-2", "profile-2"), &eveyv1.ListMediaItemsRequest{LibraryId: "lib-1"})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	assert.Equal(t, "k-pg", list.Items[0].Id)
}

// A profile that cannot be read sees nothing (the parental resolver fails
// closed), so it has no facet either.
func TestLibraryHandler_GetLibraryFilterFacets_UnreadableProfileFailsClosed(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-1"))
	seedFacetItem(mediaQuery, &library.MediaItem{ID: "u-1", LibraryID: "lib-1", Type: "movie", Title: "G",
		Classification: &library.MediaClassification{OfficialRating: "G"}},
		videoStreamOf("h264", 1920, 1080), langStream(library.StreamTypeAudio, "fre"))

	resp, err := h.GetLibraryFilterFacets(userCtx("user-2", "profile-missing"), &eveyv1.GetLibraryFilterFacetsRequest{LibraryId: "lib-1"})
	require.NoError(t, err)
	assert.Empty(t, resp.VideoCodecs)
	assert.Empty(t, resp.AudioLanguages)
}

// item_count is the number the phone used to fetch once per card: the
// total_items of ListMediaItems for that library_id alone, for the SAME caller.
// Checked on an adult and on a kids profile, whose rating pre-filter the total
// carries.
func TestLibraryHandler_ListLibraries_ItemCountMatchesListMediaItemsTotal(t *testing.T) {
	for _, kids := range []bool{false, true} {
		h, libQuery, mediaQuery, _, libraryAccess, profiles, _ := newTestLibraryHandler()
		libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
		libQuery.Seed(&library.Library{ID: "lib-2", Name: "Shows", Type: "tvshow"})
		libQuery.Seed(&library.Library{ID: "lib-3", Name: "Empty", Type: "movie"})
		require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-1"))
		require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-2"))
		require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-3"))
		if kids {
			seedKidsProfile(t, profiles, libraryAccess)
		}

		mediaQuery.Seed(&library.MediaItem{ID: "c-pg", LibraryID: "lib-1", Type: "movie", Title: "PG",
			Classification: &library.MediaClassification{OfficialRating: "PG"}})
		mediaQuery.Seed(&library.MediaItem{ID: "c-r", LibraryID: "lib-1", Type: "movie", Title: "R",
			Classification: &library.MediaClassification{OfficialRating: "R"}})
		// Every item type counts, as it does in the per-library total.
		mediaQuery.Seed(&library.MediaItem{ID: "c-s", LibraryID: "lib-2", Type: "series", Title: "Show"})
		mediaQuery.Seed(&library.MediaItem{ID: "c-se", LibraryID: "lib-2", Type: "season", Title: "S1", ParentID: "c-s"})
		mediaQuery.Seed(&library.MediaItem{ID: "c-e", LibraryID: "lib-2", Type: "episode", Title: "E1", ParentID: "c-se"})
		// A library the caller cannot open is neither listed nor counted.
		mediaQuery.Seed(&library.MediaItem{ID: "c-x", LibraryID: "lib-secret", Type: "movie", Title: "X"})

		ctx := userCtx("user-2", "profile-2")
		resp, err := h.ListLibraries(ctx, &eveyv1.ListLibrariesRequest{})
		require.NoError(t, err)
		require.Len(t, resp.Libraries, 3)
		for _, lib := range resp.Libraries {
			list, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
				LibraryId: lib.Id, Pagination: &eveyv1.PaginationRequest{Page: 1, PageSize: 1},
			})
			require.NoError(t, err)
			require.NotNil(t, lib.ItemCount, "kids=%v %s carries a count", kids, lib.Id)
			assert.Equal(t, list.Pagination.TotalItems, lib.GetItemCount(), "kids=%v %s", kids, lib.Id)
		}
	}
}

// A count that cannot be read leaves the field absent (no counter beats a
// wrong "0 items") and never fails the library list itself.
func TestLibraryHandler_ListLibraries_ItemCountAbsentWhenCountFails(t *testing.T) {
	h, libQuery, mediaQuery, _, _, _, _ := newTestLibraryHandler()
	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	mediaQuery.Err = assert.AnError

	resp, err := h.ListLibraries(adminCtx(), &eveyv1.ListLibrariesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Libraries, 1)
	assert.Nil(t, resp.Libraries[0].ItemCount)
}
