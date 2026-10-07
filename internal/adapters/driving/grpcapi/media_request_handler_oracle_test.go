package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmediarequest "github.com/evey-project/evey/internal/app/mediarequest"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/metadata"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
)

// heldLookup answers every provider key with the same media item id.
type heldLookup struct{ itemID string }

func (l heldLookup) FindByProviderIDs(_ context.Context, _ string, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, k := range keys {
		out[k] = l.itemID
	}
	return out, nil
}

// oracleFixture wires a search handler whose provider result is matched to
// the given library item, with the access ports server.go supplies.
func oracleFixture(t *testing.T, item *library.MediaItem) (*mediaRequestHandler, *userfakes.FakeLibraryAccessRepository, *userfakes.FakeProfileRepository) {
	t.Helper()
	provider := &cannedProvider{results: []metadata.MetadataResult{{
		Title: "Some Movie", ProviderIDs: map[string]string{"tmdb": "603"},
	}}}
	svc := appmediarequest.NewService(newFakeMediaRequestRepo(), newFakeMediaRequestAccess(true),
		[]metadata.MetadataProvider{provider}, sharedfakes.NewFakeEventBus()).
		WithLibraryLookup(heldLookup{itemID: string(item.ID)})

	mediaRepo := libfakes.NewFakeMediaItemQueryRepo()
	mediaRepo.Seed(item)
	userRepo := userfakes.NewFakeUserRepository()
	apiKeyRepo := userfakes.NewFakeAPIKeyRepository()
	inviteRepo := userfakes.NewFakeInviteLinkRepository()
	qcRepo := userfakes.NewFakeQuickConnectRepository()
	grants := userfakes.NewFakeLibraryAccessRepository()
	profiles := userfakes.NewFakeProfileRepository()
	return &mediaRequestHandler{
		svc: svc,
		libQuery: applibrary.NewQueryService(libfakes.NewFakeLibraryQueryRepo(), mediaRepo,
			libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakeSearchIndex()),
		userAccess: appuser.NewAccessService(userRepo, apiKeyRepo, inviteRepo, qcRepo,
			userfakes.NewFakePasswordHasher(), sharedfakes.NewFakeEventBus(), appuser.WithLibraryAccess(grants)),
		userQuery: appuser.NewQueryService(userRepo, profiles, apiKeyRepo, inviteRepo, qcRepo),
	}, grants, profiles
}

func searchOne(ctx context.Context, t *testing.T, h *mediaRequestHandler) *eveyv1.TitleCandidate {
	t.Helper()
	resp, err := h.SearchTitles(ctx, &eveyv1.SearchTitlesRequest{MediaType: "movie", Query: "some"})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	return resp.Results[0]
}

func TestMediaRequestHandler_SearchTitles_NoLibraryAccessHidesMatch(t *testing.T) {
	h, grants, profiles := oracleFixture(t, &library.MediaItem{ID: "mi-1", LibraryID: "lib-private", Title: "Some Movie", Type: "movie"})
	ctx := context.Background()
	_ = profiles.Create(ctx, &user.Profile{ID: "profile-1", UserID: "user-1", Name: "Plain"})
	require.NoError(t, grants.Grant(ctx, "user-1", "lib-other"))

	got := searchOne(userCtx("user-1", "profile-1"), t, h)
	assert.False(t, got.InLibrary, "a user without access to the library must not learn the title is held")
	assert.Empty(t, got.MediaItemId)
}

// The other half of the oracle: creating a request for a held title used to
// answer "already held" to everyone. To a profile that cannot see the title it
// is not on the server, so the request goes through; to one that can, it is
// still refused.
func TestMediaRequestHandler_CreateRequest_HeldTitleInvisibleToKidsIsRequestable(t *testing.T) {
	h, grants, profiles := oracleFixture(t, &library.MediaItem{ID: "mi-r", LibraryID: "lib-1", Title: "Adult Movie", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}})
	ctx := context.Background()
	maxAge := 13
	_ = profiles.Create(ctx, &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})
	_ = profiles.Create(ctx, &user.Profile{ID: "profile-1", UserID: "user-1", Name: "Plain"})
	require.NoError(t, grants.Grant(ctx, "user-1", "lib-1"))
	req := &eveyv1.CreateRequestRequest{MediaType: "movie", Provider: "tmdb", ProviderId: "603", Title: "Some Movie"}

	created, err := h.CreateRequest(userCtx("user-2", "profile-2"), req)
	require.NoError(t, err, "a title the kids profile cannot see is not 'already held' to it")
	assert.NotEmpty(t, created.Id)

	_, err = h.CreateRequest(userCtx("user-1", "profile-1"), req)
	require.Error(t, err, "a caller who may see the title is still told it is held")
	assert.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestMediaRequestHandler_SearchTitles_KidsProfileHidesAdultMatch(t *testing.T) {
	h, _, profiles := oracleFixture(t, &library.MediaItem{ID: "mi-r", LibraryID: "lib-1", Title: "Adult Movie", Type: "movie",
		Classification: &library.MediaClassification{OfficialRating: "R"}})
	maxAge := 13
	_ = profiles.Create(context.Background(), &user.Profile{ID: "profile-2", UserID: "user-2", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{MaxAge: &maxAge, Enabled: true}})

	got := searchOne(userCtx("user-2", "profile-2"), t, h)
	assert.False(t, got.InLibrary, "a kids profile must not learn an R-rated title is held")
	assert.Empty(t, got.MediaItemId)
}

func TestMediaRequestHandler_SearchTitles_AccessibleMatchStaysVisible(t *testing.T) {
	h, grants, profiles := oracleFixture(t, &library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Title: "Some Movie", Type: "movie"})
	ctx := context.Background()
	_ = profiles.Create(ctx, &user.Profile{ID: "profile-1", UserID: "user-1", Name: "Plain"})
	require.NoError(t, grants.Grant(ctx, "user-1", "lib-1"))

	got := searchOne(userCtx("user-1", "profile-1"), t, h)
	assert.True(t, got.InLibrary)
	assert.Equal(t, "mi-1", got.MediaItemId)

	admin := searchOne(ctxWithClaims("admin-1", "profile-a", true), t, h)
	assert.True(t, admin.InLibrary, "an admin is never gated")
}
