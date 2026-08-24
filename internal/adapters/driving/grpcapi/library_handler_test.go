package grpcapi

import (
	"context"
	"errors"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/app/metadata"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	mkvfakes "github.com/evey-project/evey/internal/domain/mkv/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestLibraryHandler() (*libraryHandler, *libfakes.FakeLibraryQueryRepo, *libfakes.FakeMediaItemQueryRepo, *libfakes.FakeSearchIndex, *userfakes.FakeLibraryAccessRepository, *userfakes.FakeProfileRepository, *libfakes.FakeMediaStreamRepository) {
	libCmdRepo := libfakes.NewFakeLibraryCommandRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaCmdRepo := libfakes.NewFakeMediaItemCommandRepo()
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	searchIdx := libfakes.NewFakeSearchIndex()
	eventBus := sharedfakes.NewFakeEventBus()
	libraryAccess := userfakes.NewFakeLibraryAccessRepository()
	profiles := userfakes.NewFakeProfileRepository()

	cmdSvc := applibrary.NewCommandService(
		libCmdRepo, libQueryRepo, mediaCmdRepo, streamRepo,
		personRepo, mediaPersonRepo, scanner, prober, searchIdx, eventBus,
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(),
		"/tmp/test-trickplay", sharedfakes.NewFakeServerConfigRepository(),
	)
	querySvc := applibrary.NewQueryService(libQueryRepo, mediaQueryRepo, streamRepo, searchIdx)

	userRepo := userfakes.NewFakeUserRepository()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	userAccess := appuser.NewAccessService(
		userRepo, apiKeyRepo, inviteRepo, qcRepo,
		userfakes.NewFakePasswordHasher(), eventBus,
		appuser.WithLibraryAccess(libraryAccess),
	)
	userQuery := appuser.NewQueryService(userRepo, profiles, apiKeyRepo, inviteRepo, qcRepo)

	h := &libraryHandler{
		cmd:        cmdSvc,
		query:      querySvc,
		userAccess: userAccess,
		userQuery:  userQuery,
	}
	// The parental gate fails closed on an unresolvable profile, so the
	// non-parental tests need the default claimed profile to exist. Kids
	// tests overwrite it with their own settings.
	_ = profiles.Create(context.Background(), &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Adult"})
	return h, libQueryRepo, mediaQueryRepo, searchIdx, libraryAccess, profiles, streamRepo
}

func adminCtx() context.Context {
	return interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:    "user-1",
		Username:  "admin",
		IsAdmin:   true,
		ProfileID: "profile-1",
	})
}

func userCtx(userID, profileID string) context.Context {
	return interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:    userID,
		Username:  "viewer",
		IsAdmin:   false,
		ProfileID: profileID,
	})
}

func TestLibraryHandler_CreateLibrary(t *testing.T) {
	h, _, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	resp, err := h.CreateLibrary(ctx, &eveyv1.CreateLibraryRequest{
		Name:  "Movies",
		Type:  eveyv1.LibraryType_LIBRARY_TYPE_MOVIES,
		Paths: []string{"/media/movies"},
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Id)
	assert.Equal(t, "Movies", resp.Name)
	assert.Equal(t, eveyv1.LibraryType_LIBRARY_TYPE_MOVIES, resp.Type)
	assert.Equal(t, []string{"/media/movies"}, resp.Paths)
}

func TestLibraryHandler_GetLibrary_Success(t *testing.T) {
	h, libQuery, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "TV Shows", Type: "tvshow", Paths: []string{"/media/tv"}})

	resp, err := h.GetLibrary(ctx, &eveyv1.GetLibraryRequest{Id: "lib-1"})

	require.NoError(t, err)
	assert.Equal(t, "lib-1", resp.Id)
	assert.Equal(t, "TV Shows", resp.Name)
}

func TestLibraryHandler_GetLibrary_NotFound(t *testing.T) {
	h, _, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	_, err := h.GetLibrary(ctx, &eveyv1.GetLibraryRequest{Id: "nonexistent"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLibraryHandler_GetLibrary_NoAccess(t *testing.T) {
	h, libQuery, _, _, libraryAccess, _, _ := newTestLibraryHandler()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/media/movies"}})
	libraryAccess.Grant(context.Background(), "user-2", "lib-99")

	ctx := userCtx("user-2", "profile-2")
	_, err := h.GetLibrary(ctx, &eveyv1.GetLibraryRequest{Id: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLibraryHandler_GetLibrary_AdminBypassAccess(t *testing.T) {
	h, libQuery, _, _, libraryAccess, _, _ := newTestLibraryHandler()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/media/movies"}})
	libraryAccess.Grant(context.Background(), "user-2", "lib-99")

	ctx := adminCtx()
	resp, err := h.GetLibrary(ctx, &eveyv1.GetLibraryRequest{Id: "lib-1"})

	require.NoError(t, err)
	assert.Equal(t, "lib-1", resp.Id)
}

func TestLibraryHandler_ListLibraries(t *testing.T) {
	h, libQuery, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	libQuery.Seed(&library.Library{ID: "lib-2", Name: "TV Shows", Type: "tvshow"})

	resp, err := h.ListLibraries(ctx, &eveyv1.ListLibrariesRequest{})

	require.NoError(t, err)
	assert.Len(t, resp.Libraries, 2)
}

func TestLibraryHandler_ListLibraries_FilteredByAccess(t *testing.T) {
	h, libQuery, _, _, libraryAccess, _, _ := newTestLibraryHandler()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	libQuery.Seed(&library.Library{ID: "lib-2", Name: "TV Shows", Type: "tvshow"})
	libQuery.Seed(&library.Library{ID: "lib-3", Name: "Music", Type: "music"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	libraryAccess.Grant(context.Background(), "user-2", "lib-3")

	ctx := userCtx("user-2", "profile-2")
	resp, err := h.ListLibraries(ctx, &eveyv1.ListLibrariesRequest{})

	require.NoError(t, err)
	assert.Len(t, resp.Libraries, 2)
	ids := make(map[string]bool)
	for _, lib := range resp.Libraries {
		ids[lib.Id] = true
	}
	assert.True(t, ids["lib-1"])
	assert.True(t, ids["lib-3"])
	assert.False(t, ids["lib-2"])
}

func TestLibraryHandler_DeleteLibrary(t *testing.T) {
	h, _, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	createResp, err := h.CreateLibrary(ctx, &eveyv1.CreateLibraryRequest{
		Name:  "ToDelete",
		Type:  eveyv1.LibraryType_LIBRARY_TYPE_MOVIES,
		Paths: []string{"/tmp"},
	})
	require.NoError(t, err)

	_, err = h.DeleteLibrary(ctx, &eveyv1.DeleteLibraryRequest{Id: createResp.Id})
	require.NoError(t, err)
}

// Regression for the prod 500: deleting a library that no longer exists (stale
// admin list, double-click, already removed elsewhere) must be idempotent -
// success, not an internal error.
func TestLibraryHandler_DeleteLibrary_AlreadyAbsentIdempotent(t *testing.T) {
	h, _, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	_, err := h.DeleteLibrary(ctx, &eveyv1.DeleteLibraryRequest{Id: "lib-does-not-exist"})
	require.NoError(t, err)
}

func TestLibraryHandler_GetMediaItem_Success(t *testing.T) {
	h, _, mediaQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-1",
		LibraryID: "lib-1",
		Title:     "Some Movie",
		Type:      "movie",
	})

	resp, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-1"})

	require.NoError(t, err)
	assert.Equal(t, "mi-1", resp.Id)
	assert.Equal(t, "Some Movie", resp.Title)
	assert.Equal(t, eveyv1.MediaItemType_MEDIA_ITEM_TYPE_MOVIE, resp.Type)
}

func TestLibraryHandler_GetMediaItem_NotFound(t *testing.T) {
	h, _, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	_, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "nonexistent"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLibraryHandler_GetMediaItem_HydratesStreams(t *testing.T) {
	h, _, mediaQuery, _, _, _, streamRepo := newTestLibraryHandler()
	ctx := adminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-2",
		LibraryID: "lib-1",
		Title:     "Some Movie",
		Type:      "movie",
	})
	require.NoError(t, streamRepo.ReplaceBatch(ctx, "mi-2", []*library.MediaStream{
		{ID: "st-1", MediaItemID: "mi-2", StreamIndex: 0, StreamType: library.StreamTypeVideo, Codec: "h264", Language: "eng"},
		{ID: "st-2", MediaItemID: "mi-2", StreamIndex: 1, StreamType: library.StreamTypeAudio, Codec: "aac", Language: "fre"},
	}))

	resp, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-2"})

	require.NoError(t, err)
	require.Len(t, resp.Streams, 2)
	assert.Equal(t, eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_VIDEO, resp.Streams[0].Type)
	assert.Equal(t, "h264", resp.Streams[0].Codec)
	assert.Equal(t, eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_AUDIO, resp.Streams[1].Type)
	assert.Equal(t, "aac", resp.Streams[1].Codec)
}

// Grids and filter facets on every client read codecs/resolution off LIST
// rows - and an empty streams array is indistinguishable from "no streams",
// which is how the TV/mobile quality badges and the mobile/web facet filters
// shipped dead (2026-08-03).
func TestLibraryHandler_ListMediaItems_HydratesStreams(t *testing.T) {
	h, _, mediaQuery, _, _, _, streamRepo := newTestLibraryHandler()
	ctx := adminCtx()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-s1", LibraryID: "lib-1", Title: "Ep 1", Type: "episode"})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-s2", LibraryID: "lib-1", Title: "Ep 2", Type: "episode"})
	require.NoError(t, streamRepo.ReplaceBatch(ctx, "mi-s1", []*library.MediaStream{
		{ID: "st-l1", MediaItemID: "mi-s1", StreamIndex: 0, StreamType: library.StreamTypeVideo, Codec: "h264"},
		{ID: "st-l2", MediaItemID: "mi-s1", StreamIndex: 1, StreamType: library.StreamTypeAudio, Codec: "eac3"},
	}))

	resp, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{})

	require.NoError(t, err)
	byID := map[string]*eveyv1.MediaItem{}
	for _, it := range resp.Items {
		byID[it.Id] = it
	}
	require.NotNil(t, byID["mi-s1"])
	require.Len(t, byID["mi-s1"].Streams, 2)
	assert.Equal(t, "h264", byID["mi-s1"].Streams[0].Codec)
	assert.Equal(t, "eac3", byID["mi-s1"].Streams[1].Codec)
	require.NotNil(t, byID["mi-s2"])
	assert.Empty(t, byID["mi-s2"].Streams)
}

func TestLibraryHandler_FilterMediaItems_HydratesStreams(t *testing.T) {
	h, _, mediaQuery, _, _, _, streamRepo := newTestLibraryHandler()
	ctx := adminCtx()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-f1", LibraryID: "lib-1", Title: "Movie", Type: "movie"})
	require.NoError(t, streamRepo.ReplaceBatch(ctx, "mi-f1", []*library.MediaStream{
		{ID: "st-f1", MediaItemID: "mi-f1", StreamIndex: 0, StreamType: library.StreamTypeVideo, Codec: "hevc"},
	}))

	resp, err := h.FilterMediaItems(ctx, &eveyv1.FilterMediaItemsRequest{})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Len(t, resp.Items[0].Streams, 1)
	assert.Equal(t, "hevc", resp.Items[0].Streams[0].Codec)
}

func TestLibraryHandler_ListMediaItems_WithPagination(t *testing.T) {
	h, _, mediaQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	for i := range 5 {
		mediaQuery.Seed(&library.MediaItem{
			ID:        shared.MediaItemID("mi-" + string(rune('a'+i))),
			LibraryID: "lib-1",
			Title:     "Movie " + string(rune('A'+i)),
			Type:      "movie",
		})
	}

	resp, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
		Pagination: &eveyv1.PaginationRequest{Page: 1, PageSize: 3},
	})

	require.NoError(t, err)
	assert.Len(t, resp.Items, 3)
	assert.Equal(t, int32(5), resp.Pagination.TotalItems)
	assert.Equal(t, int32(1), resp.Pagination.Page)
	assert.Equal(t, int32(3), resp.Pagination.PageSize)
	// 5 items in pages of 3 = 2 pages. The field sat at zero for so long the
	// mobile library screen documented "the server NEVER fills totalPages".
	assert.Equal(t, int32(2), resp.Pagination.TotalPages)
}

func TestLibraryHandler_SearchMedia(t *testing.T) {
	h, _, _, searchIdx, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID:    "mi-1",
		Title: "The Cascade",
		Type:  "movie",
	})
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID:    "mi-2",
		Title: "Cascade Reloaded",
		Type:  "movie",
	})
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID:    "mi-3",
		Title: "Some Movie",
		Type:  "movie",
	})

	resp, err := h.SearchMedia(ctx, &eveyv1.SearchMediaRequest{
		Query: "Cascade",
		Limit: 10,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	for _, item := range resp.Items {
		assert.Contains(t, item.Title, "Cascade")
	}
}

// TestLibraryHandler_SearchMedia_HugeLimitClamped proves a limit far above
// maxPageSize is capped before it reaches the search index, so a client cannot
// request an unbounded result set (memory + query-load DoS).
func TestLibraryHandler_SearchMedia_HugeLimitClamped(t *testing.T) {
	h, _, _, searchIdx, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	_, err := h.SearchMedia(ctx, &eveyv1.SearchMediaRequest{
		Query: "anything",
		Limit: 1_000_000,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, searchIdx.LastSearchLimit.Load(), int64(maxPageSize),
		"huge limit must be clamped to at most maxPageSize before reaching the index")
}

func TestApplyOverridesToItem(t *testing.T) {
	year := 2010
	rating := 7.5
	item := &library.MediaItem{
		ID:             "mi-1",
		Title:          "Original Title",
		OriginalTitle:  "Titre Original",
		Overview:       "Old overview",
		Catalog:        &library.MediaCatalog{Year: &year, Genres: []string{"Drama"}},
		Classification: &library.MediaClassification{CommunityRating: &rating},
	}

	overrides := []*domainmetadata.MetadataOverride{
		{FieldName: "title", CustomValue: "New Title"},
		{FieldName: "original_title", CustomValue: "Nouveau Titre"},
		{FieldName: "overview", CustomValue: "New overview"},
		{FieldName: "year", CustomValue: "2024"},
		{FieldName: "community_rating", CustomValue: "9.1"},
		{FieldName: "genres", CustomValue: "Action, Sci-Fi"},
	}

	applyOverridesToItem(item, overrides)

	assert.Equal(t, "New Title", item.Title)
	assert.Equal(t, "Nouveau Titre", item.OriginalTitle)
	assert.Equal(t, "New overview", item.Overview)
	require.NotNil(t, item.Catalog)
	assert.Equal(t, 2024, *item.Catalog.Year)
	require.NotNil(t, item.Classification)
	assert.InDelta(t, 9.1, *item.Classification.CommunityRating, 0.01)
	assert.Equal(t, []string{"Action", "Sci-Fi"}, item.Catalog.Genres)
}

func TestApplyOverridesToItem_EmptyValueSkipped(t *testing.T) {
	item := &library.MediaItem{Title: "Keep This"}
	overrides := []*domainmetadata.MetadataOverride{
		{FieldName: "title", CustomValue: ""},
	}
	applyOverridesToItem(item, overrides)
	assert.Equal(t, "Keep This", item.Title)
}

func TestApplyOverridesToItem_NilOverrides(t *testing.T) {
	item := &library.MediaItem{Title: "Keep This"}
	applyOverridesToItem(item, nil)
	assert.Equal(t, "Keep This", item.Title)
}

// --- IDOR / authorization tests ---

func TestLibraryHandler_GetLibrary_AccessDenied(t *testing.T) {
	h, libQuery, _, _, libraryAccess, _, _ := newTestLibraryHandler()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	libQuery.Seed(&library.Library{ID: "lib-2", Name: "Private", Type: "movie"})

	// user-2 only has access to lib-1
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	// GetLibrary denies access to lib-2
	_, err := h.GetLibrary(ctx, &eveyv1.GetLibraryRequest{Id: "lib-2"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLibraryHandler_GetMediaItem_IDOR_Denied(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-secret", Title: "Secret Movie", Type: "movie"})

	// user-2 has access to lib-1 only (creates restriction)
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	_, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-1"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLibraryHandler_GetMediaItem_IDOR_Allowed(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Title: "Allowed Movie", Type: "movie"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-1"})
	require.NoError(t, err)
	assert.Equal(t, "Allowed Movie", resp.Title)
}

func TestLibraryHandler_GetMediaItem_ParentalFailsClosedOnProfileLookupError(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Title: "Movie", Type: "movie"})

	// library access granted, but the claimed profile cannot be resolved -
	// the parental gate must fail closed, not fall back to "no restriction".
	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-missing")

	_, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-1"})
	require.Error(t, err)
	// Fail-closed still blocks; the client sees NotFound (existence hidden), not
	// PermissionDenied - same opacity as the library-access path.
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLibraryHandler_GetMediaItem_AdminBypassesAccess(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-secret", Title: "Secret", Type: "movie"})

	libraryAccess.Grant(context.Background(), "user-1", "lib-other")
	ctx := adminCtx()

	resp, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-1"})
	require.NoError(t, err)
	assert.Equal(t, "Secret", resp.Title)
}

func TestLibraryHandler_ListGenres_IDOR_Denied(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "g-secret", LibraryID: "lib-secret", Type: "movie",
		Catalog: &library.MediaCatalog{Genres: []string{"Horror"}}})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.ListGenres(ctx, &eveyv1.ListGenresRequest{LibraryId: "lib-secret"})
	require.NoError(t, err)
	assert.Empty(t, resp.Genres, "no access to lib-secret must leak no genres")
}

func TestLibraryHandler_ListGenres_Allowed(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "g-a", LibraryID: "lib-1", Type: "movie",
		Catalog: &library.MediaCatalog{Genres: []string{"Drama", "Comedy"}}})
	mediaQuery.Seed(&library.MediaItem{ID: "g-b", LibraryID: "lib-1", Type: "movie",
		Catalog: &library.MediaCatalog{Genres: []string{"Comedy", "Action"}}})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.ListGenres(ctx, &eveyv1.ListGenresRequest{LibraryId: "lib-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"Action", "Comedy", "Drama"}, resp.Genres, "distinct + sorted")
}

func TestLibraryHandler_ListGenres_MissingLibraryID(t *testing.T) {
	h, _, _, _, _, _, _ := newTestLibraryHandler()
	ctx := userCtx("user-2", "profile-2")
	_, err := h.ListGenres(ctx, &eveyv1.ListGenresRequest{LibraryId: ""})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestLibraryHandler_FilterMediaItems_MultiGenre(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "f-a", LibraryID: "lib-1", Type: "movie", Title: "A",
		Catalog: &library.MediaCatalog{Genres: []string{"Action"}}})
	mediaQuery.Seed(&library.MediaItem{ID: "f-d", LibraryID: "lib-1", Type: "movie", Title: "D",
		Catalog: &library.MediaCatalog{Genres: []string{"Drama"}}})
	mediaQuery.Seed(&library.MediaItem{ID: "f-h", LibraryID: "lib-1", Type: "movie", Title: "H",
		Catalog: &library.MediaCatalog{Genres: []string{"Horror"}}})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.FilterMediaItems(ctx, &eveyv1.FilterMediaItemsRequest{
		LibraryId: "lib-1",
		Genres:    []string{"Action", "Drama"},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Total, "OR match on Action or Drama, Horror excluded")
	ids := map[string]bool{}
	for _, it := range resp.Items {
		ids[it.Id] = true
	}
	assert.True(t, ids["f-a"])
	assert.True(t, ids["f-d"])
	assert.False(t, ids["f-h"])
}

func TestLibraryHandler_ListMediaItems_IDOR_Denied(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-secret", Title: "Secret", Type: "movie"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
		LibraryId:  "lib-secret",
		Pagination: &eveyv1.PaginationRequest{Page: 1, PageSize: 10},
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

func TestLibraryHandler_ListMediaItems_IDOR_Allowed(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Title: "Allowed", Type: "movie"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
		LibraryId:  "lib-1",
		Pagination: &eveyv1.PaginationRequest{Page: 1, PageSize: 10},
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, "Allowed", resp.Items[0].Title)
}

func TestLibraryHandler_ListMediaItems_NoLibraryID_FiltersAccess(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Title: "Visible", Type: "movie"})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-2", LibraryID: "lib-secret", Title: "Hidden", Type: "movie"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{
		Pagination: &eveyv1.PaginationRequest{Page: 1, PageSize: 10},
	})
	require.NoError(t, err)
	for _, item := range resp.Items {
		assert.NotEqual(t, "lib-secret", item.LibraryId, "should not see items from restricted library")
	}
}

func TestLibraryHandler_SearchMedia_IDOR_FiltersAccess(t *testing.T) {
	h, _, _, searchIdx, libraryAccess, _, _ := newTestLibraryHandler()
	ctx := context.Background()

	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID: "mi-1", LibraryID: "lib-1", Title: "Allowed Cascade", Type: "movie",
	})
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID: "mi-2", LibraryID: "lib-secret", Title: "Secret Cascade", Type: "movie",
	})

	libraryAccess.Grant(ctx, "user-2", "lib-1")
	userCtx := userCtx("user-2", "profile-2")

	resp, err := h.SearchMedia(userCtx, &eveyv1.SearchMediaRequest{Query: "Cascade", Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, "mi-1", resp.Items[0].Id)
	// TotalHits came straight from the backend count, so it still announced the
	// hit in lib-secret: the item list was filtered but the tally told the user
	// something matched in a library they cannot see. Same existence oracle the
	// stream routes avoid by answering 404 rather than 403.
	assert.EqualValues(t, 1, resp.TotalHits, "the tally must not count hits the caller may not see")
}

// The search index carries no per-country certifications (and often no rating
// at all), so a kids profile's filter must resolve the full items and apply the
// fail-closed parental check - not trust the indexed OfficialRating.
func TestLibraryHandler_SearchMedia_ParentalFailClosed(t *testing.T) {
	h, _, mediaQuery, searchIdx, libraryAccess, profiles, _ := newTestLibraryHandler()
	ctx := context.Background()

	// Hits returned by the index - note they carry no rating, like the real one.
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{ID: "mi-r", LibraryID: "lib-1", Title: "Searchword R", Type: "movie"})
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{ID: "mi-pg", LibraryID: "lib-1", Title: "Searchword PG", Type: "movie"})
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{ID: "mi-unrated", LibraryID: "lib-1", Title: "Searchword Unrated", Type: "movie"})

	// Full items (with classification) the handler resolves from the DB.
	mediaQuery.Seed(&library.MediaItem{ID: "mi-r", LibraryID: "lib-1", Title: "Searchword R", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}}) // age 17 → blocked
	mediaQuery.Seed(&library.MediaItem{ID: "mi-pg", LibraryID: "lib-1", Title: "Searchword PG", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}}) // age 10 → allowed
	mediaQuery.Seed(&library.MediaItem{ID: "mi-unrated", LibraryID: "lib-1", Title: "Searchword Unrated", Type: "movie"}) // no rating → fail closed

	maxAge := 13
	profiles.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})
	libraryAccess.Grant(ctx, "user-2", "lib-1")

	resp, err := h.SearchMedia(userCtx("user-2", "profile-2"), &eveyv1.SearchMediaRequest{Query: "Searchword", Limit: 10})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1, "only the PG item passes; R is over-age and the unrated item fails closed")
	assert.Equal(t, "mi-pg", resp.Items[0].Id)
}

// When the item resolution fails for a kids profile, every hit is withheld
// rather than leaked.
func TestLibraryHandler_SearchMedia_ParentalFetchError_FailsClosed(t *testing.T) {
	h, _, mediaQuery, searchIdx, libraryAccess, profiles, _ := newTestLibraryHandler()
	ctx := context.Background()

	searchIdx.IndexMediaItem(ctx, &library.MediaItem{ID: "mi-pg", LibraryID: "lib-1", Title: "Searchword PG", Type: "movie"})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-pg", LibraryID: "lib-1", Title: "Searchword PG", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}})

	maxAge := 13
	profiles.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})
	libraryAccess.Grant(ctx, "user-2", "lib-1")
	mediaQuery.Err = errors.New("db unavailable") // item fetch fails

	resp, err := h.SearchMedia(userCtx("user-2", "profile-2"), &eveyv1.SearchMediaRequest{Query: "Searchword", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, resp.Items, "an unresolvable kids search must fail closed")
}

// A non-kids profile is unaffected: no extra fetch, all hits returned.
func TestLibraryHandler_SearchMedia_NonKids_Unfiltered(t *testing.T) {
	h, _, _, searchIdx, libraryAccess, _, _ := newTestLibraryHandler()
	ctx := context.Background()

	searchIdx.IndexMediaItem(ctx, &library.MediaItem{ID: "mi-r", LibraryID: "lib-1", Title: "Searchword R", Type: "movie"})
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{ID: "mi-pg", LibraryID: "lib-1", Title: "Searchword PG", Type: "movie"})
	libraryAccess.Grant(ctx, "user-2", "lib-1")

	resp, err := h.SearchMedia(userCtx("user-2", "profile-2"), &eveyv1.SearchMediaRequest{Query: "Searchword", Limit: 10})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
}

func TestLibraryHandler_FilterMediaItems_IDOR_Denied(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-secret", Title: "Secret", Type: "movie"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.FilterMediaItems(ctx, &eveyv1.FilterMediaItemsRequest{
		LibraryId: "lib-secret",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

func TestLibraryHandler_FilterMediaItems_IDOR_Allowed(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Title: "Allowed", Type: "movie"})

	libraryAccess.Grant(context.Background(), "user-2", "lib-1")
	ctx := userCtx("user-2", "profile-2")

	resp, err := h.FilterMediaItems(ctx, &eveyv1.FilterMediaItemsRequest{
		LibraryId: "lib-1",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 1)
}

// FilterMediaItems must apply the same parental gate as ListMediaItems -
// otherwise the filter screen leaks restricted content to a kids profile.
func TestLibraryHandler_FilterMediaItems_ParentalFailClosed(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, profiles, _ := newTestLibraryHandler()
	ctx := context.Background()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-r", LibraryID: "lib-1", Title: "Adult R", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}}) // age 17 → blocked
	mediaQuery.Seed(&library.MediaItem{ID: "mi-pg", LibraryID: "lib-1", Title: "Kid PG", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}}) // age 10 → allowed
	mediaQuery.Seed(&library.MediaItem{ID: "mi-unrated", LibraryID: "lib-1", Title: "Unrated", Type: "movie"}) // fail closed

	maxAge := 13
	profiles.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})
	libraryAccess.Grant(ctx, "user-2", "lib-1")

	resp, err := h.FilterMediaItems(userCtx("user-2", "profile-2"), &eveyv1.FilterMediaItemsRequest{
		LibraryId: "lib-1",
		Limit:     10,
	})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1, "only the PG item passes; R is over-age and the unrated item fails closed")
	assert.Equal(t, "mi-pg", resp.Items[0].Id)
}

// A non-kids profile is unaffected: FilterMediaItems returns everything.
func TestLibraryHandler_FilterMediaItems_NonKids_Unfiltered(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()
	ctx := context.Background()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-r", LibraryID: "lib-1", Title: "Adult R", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-pg", LibraryID: "lib-1", Title: "Kid PG", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "PG"}})
	libraryAccess.Grant(ctx, "user-2", "lib-1")

	resp, err := h.FilterMediaItems(userCtx("user-2", "profile-2"), &eveyv1.FilterMediaItemsRequest{
		LibraryId: "lib-1",
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
}

func TestLibraryHandler_GetMediaItem_WithOverrides(t *testing.T) {
	h, _, mediaQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-1",
		LibraryID: "lib-1",
		Title:     "Original",
		Type:      "movie",
	})

	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	metaSvc := metadata.NewQueryService(overrideRepo, artworkRepo, nil)
	h.metadataSvc = metaSvc

	overrideRepo.Set(ctx, &domainmetadata.MetadataOverride{
		ID: "mo-1", MediaItemID: "mi-1", FieldName: "title", CustomValue: "Overridden Title",
	})

	resp, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-1"})

	require.NoError(t, err)
	assert.Equal(t, "Overridden Title", resp.Title)
}

// Defense-in-depth IDOR: UpdateLibrary must reject non-admin callers without
// access to the target library, even though RBAC already gates this RPC to
// admins. Guards against future RBAC misconfiguration.
func TestLibraryHandler_UpdateLibrary_IDOR_DeniesNonAdminWithoutAccess(t *testing.T) {
	h, libQuery, _, _, libraryAccess, _, _ := newTestLibraryHandler()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/media/movies"}})
	libraryAccess.Grant(context.Background(), "user-2", "lib-99")

	ctx := userCtx("user-2", "profile-2")
	newName := "Hijacked"
	_, err := h.UpdateLibrary(ctx, &eveyv1.UpdateLibraryRequest{Id: "lib-1", Name: &newName})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// Defense-in-depth IDOR: DeleteLibrary must reject non-admin callers without
// access to the target library, even though RBAC already gates this RPC to
// admins.
func TestLibraryHandler_DeleteLibrary_IDOR_DeniesNonAdminWithoutAccess(t *testing.T) {
	h, libQuery, _, _, libraryAccess, _, _ := newTestLibraryHandler()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie", Paths: []string{"/media/movies"}})
	libraryAccess.Grant(context.Background(), "user-2", "lib-99")

	ctx := userCtx("user-2", "profile-2")
	_, err := h.DeleteLibrary(ctx, &eveyv1.DeleteLibraryRequest{Id: "lib-1"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// Two doors, one answer: FilterMediaItems and ListMediaItems serve the SAME
// cards to the SAME screen - the filter panel just narrows the set. Filter
// hydrated streams only, so turning on a genre/year/rating filter silently
// dropped the operator's manual title override, the locale overlay and the
// artwork placeholder. Same family as the season labels found on 2026-08-24:
// a handler that rebuilds a partial item by hand instead of going through the
// shared path. Nothing crashes, nothing turns red - the title just reverts.
func TestLibraryHandler_FilterMediaItems_AppliesOverridesLikeTheListPath(t *testing.T) {
	h, _, mediaQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-ov1", LibraryID: "lib-1", Title: "Original", Type: "movie"})

	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	artworkRepo := metafakes.NewFakeArtworkRepository()
	h.metadataSvc = metadata.NewQueryService(overrideRepo, artworkRepo, nil)
	overrideRepo.Set(ctx, &domainmetadata.MetadataOverride{
		ID: "mo-ov1", MediaItemID: "mi-ov1", FieldName: "title", CustomValue: "Overridden Title",
	})

	// Filter FIRST, deliberately: the fake query repo hands out pointers to the
	// seeded items, so a ListMediaItems call run before this one would overlay
	// the override onto the shared object and leave the filter path looking
	// correct for the wrong reason.
	filtered, err := h.FilterMediaItems(ctx, &eveyv1.FilterMediaItemsRequest{})
	require.NoError(t, err)
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, "Overridden Title", filtered.Items[0].Title,
		"a filtered card must carry the same title as an unfiltered one")
}

// stubMediaTranslations serves one locale's translations from a map.
type stubMediaTranslations struct {
	byID map[string]*library.MediaItemTranslation
}

func (s *stubMediaTranslations) GetByMediaItemAndLocale(_ context.Context, mediaItemID, _ string) (*library.MediaItemTranslation, error) {
	return s.byID[mediaItemID], nil
}
func (s *stubMediaTranslations) GetByMediaItem(_ context.Context, _ string) ([]*library.MediaItemTranslation, error) {
	return nil, nil
}
func (s *stubMediaTranslations) GetByMediaItemsAndLocale(_ context.Context, ids []string, _ string) (map[string]*library.MediaItemTranslation, error) {
	out := make(map[string]*library.MediaItemTranslation, len(ids))
	for _, id := range ids {
		if t := s.byID[id]; t != nil {
			out[id] = t
		}
	}
	return out, nil
}

// The operator's correction WINS over the auto-translation.
//
// Both overlays write the same field, in sequence: overlayMediaItemsLocale
// then overlayMediaItemsOverrides. The precedence is stated in the code's own
// comment ("These win over the auto-translation too") and asserted nowhere -
// no test in this package ever switched the locale branch ON before applying
// an override, so the two had never met. Swap the two calls and nothing fails:
// a title an admin fixed by hand silently reverts to the machine translation
// on the next refresh, in exactly one locale, which is the hardest kind of
// report to get back from a user.
func TestLibraryHandler_ListMediaItems_OverrideBeatsAutoTranslation(t *testing.T) {
	libCmdRepo := libfakes.NewFakeLibraryCommandRepo()
	libQueryRepo := libfakes.NewFakeLibraryQueryRepo()
	mediaQueryRepo := libfakes.NewFakeMediaItemQueryRepo()
	trans := &stubMediaTranslations{byID: map[string]*library.MediaItemTranslation{
		"mi-both": {MediaItemID: "mi-both", Locale: "fr", Title: "Titre traduit automatiquement"},
		"mi-tr":   {MediaItemID: "mi-tr", Locale: "fr", Title: "Traduction seule"},
	}}
	querySvc := applibrary.NewQueryService(
		libQueryRepo, mediaQueryRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex(),
		applibrary.WithTranslations(trans, nil),
	)
	_ = libCmdRepo

	overrideRepo := metafakes.NewFakeMetadataOverrideRepository()
	metaSvc := metadata.NewQueryService(overrideRepo, metafakes.NewFakeArtworkRepository(), nil)
	h := &libraryHandler{query: querySvc, metadataSvc: metaSvc}

	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-both", LibraryID: "lib-1", Title: "Stored Title", Type: "movie"})
	mediaQueryRepo.Seed(&library.MediaItem{ID: "mi-tr", LibraryID: "lib-1", Title: "Stored Title Two", Type: "movie"})
	ctx := eveyi18n.ContextWithLocale(adminCtx(), "fr")
	overrideRepo.Set(ctx, &domainmetadata.MetadataOverride{
		ID: "mo-both", MediaItemID: "mi-both", FieldName: "title", CustomValue: "Titre corrige a la main",
	})

	resp, err := h.ListMediaItems(ctx, &eveyv1.ListMediaItemsRequest{})
	require.NoError(t, err)
	byID := map[string]string{}
	for _, it := range resp.Items {
		byID[it.Id] = it.Title
	}
	assert.Equal(t, "Titre corrige a la main", byID["mi-both"],
		"an operator override must beat the auto-translation on the same field")
	assert.Equal(t, "Traduction seule", byID["mi-tr"],
		"without an override the auto-translation still applies - or the test would pass on a handler that ignores locale entirely")
}
