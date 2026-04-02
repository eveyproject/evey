package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/app/metadata"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
	metafakes "github.com/gravity-zero/evey/internal/domain/metadata/fakes"
	mkvfakes "github.com/gravity-zero/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestLibraryHandler() (*libraryHandler, *libfakes.FakeLibraryQueryRepo, *libfakes.FakeMediaItemQueryRepo, *libfakes.FakeSearchIndex, *userfakes.FakeLibraryAccessRepository, *userfakes.FakeProfileRepository) {
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

	h := &libraryHandler{
		cmd:           cmdSvc,
		query:         querySvc,
		libraryAccess: libraryAccess,
		profiles:      profiles,
	}
	return h, libQueryRepo, mediaQueryRepo, searchIdx, libraryAccess, profiles
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
	h, _, _, _, _, _ := newTestLibraryHandler()
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
	h, libQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "TV Shows", Type: "tvshow", Paths: []string{"/media/tv"}})

	resp, err := h.GetLibrary(ctx, &eveyv1.GetLibraryRequest{Id: "lib-1"})

	require.NoError(t, err)
	assert.Equal(t, "lib-1", resp.Id)
	assert.Equal(t, "TV Shows", resp.Name)
}

func TestLibraryHandler_GetLibrary_NotFound(t *testing.T) {
	h, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	_, err := h.GetLibrary(ctx, &eveyv1.GetLibraryRequest{Id: "nonexistent"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLibraryHandler_ListLibraries(t *testing.T) {
	h, libQuery, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	libQuery.Seed(&library.Library{ID: "lib-1", Name: "Movies", Type: "movie"})
	libQuery.Seed(&library.Library{ID: "lib-2", Name: "TV Shows", Type: "tvshow"})

	resp, err := h.ListLibraries(ctx, &eveyv1.ListLibrariesRequest{})

	require.NoError(t, err)
	assert.Len(t, resp.Libraries, 2)
}

func TestLibraryHandler_ListLibraries_FilteredByAccess(t *testing.T) {
	h, libQuery, _, _, libraryAccess, _ := newTestLibraryHandler()

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
	h, _, _, _, _, _ := newTestLibraryHandler()
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

func TestLibraryHandler_GetMediaItem_Success(t *testing.T) {
	h, _, mediaQuery, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	mediaQuery.Seed(&library.MediaItem{
		ID:        "mi-1",
		LibraryID: "lib-1",
		Title:     "Inception",
		Type:      "movie",
	})

	resp, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "mi-1"})

	require.NoError(t, err)
	assert.Equal(t, "mi-1", resp.Id)
	assert.Equal(t, "Inception", resp.Title)
	assert.Equal(t, eveyv1.MediaItemType_MEDIA_ITEM_TYPE_MOVIE, resp.Type)
}

func TestLibraryHandler_GetMediaItem_NotFound(t *testing.T) {
	h, _, _, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	_, err := h.GetMediaItem(ctx, &eveyv1.GetMediaItemRequest{Id: "nonexistent"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestLibraryHandler_ListMediaItems_WithPagination(t *testing.T) {
	h, _, mediaQuery, _, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	for i := range 5 {
		mediaQuery.Seed(&library.MediaItem{
			ID:        "mi-" + string(rune('a'+i)),
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
}

func TestLibraryHandler_SearchMedia(t *testing.T) {
	h, _, _, searchIdx, _, _ := newTestLibraryHandler()
	ctx := adminCtx()

	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID:    "mi-1",
		Title: "The Matrix",
		Type:  "movie",
	}, nil)
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID:    "mi-2",
		Title: "Matrix Reloaded",
		Type:  "movie",
	}, nil)
	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID:    "mi-3",
		Title: "Inception",
		Type:  "movie",
	}, nil)

	resp, err := h.SearchMedia(ctx, &eveyv1.SearchMediaRequest{
		Query: "Matrix",
		Limit: 10,
	})

	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
	for _, item := range resp.Items {
		assert.Contains(t, item.Title, "Matrix")
	}
}

func TestApplyOverridesToItem(t *testing.T) {
	year := 2010
	rating := 7.5
	item := &library.MediaItem{
		ID:              "mi-1",
		Title:           "Original Title",
		OriginalTitle:   "Titre Original",
		Overview:        "Old overview",
		Year:            &year,
		CommunityRating: &rating,
		Genres:          []string{"Drama"},
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
	assert.Equal(t, 2024, *item.Year)
	assert.InDelta(t, 9.1, *item.CommunityRating, 0.01)
	assert.Equal(t, []string{"Action", "Sci-Fi"}, item.Genres)
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

func TestLibraryHandler_GetMediaItem_WithOverrides(t *testing.T) {
	h, _, mediaQuery, _, _, _ := newTestLibraryHandler()
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
