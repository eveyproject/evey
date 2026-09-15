package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appmediarequest "github.com/evey-project/evey/internal/app/mediarequest"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// cannedProvider answers every search with the same results and records the
// year it was asked for.
type cannedProvider struct {
	results  []metadata.MetadataResult
	searched int
	lastYear *int
}

func (p *cannedProvider) Name() string { return "tmdb" }
func (p *cannedProvider) Search(_ context.Context, _ string, year *int, _ string) ([]metadata.MetadataResult, error) {
	p.searched++
	p.lastYear = year
	return p.results, nil
}
func (p *cannedProvider) GetByProviderID(context.Context, string, string) (*metadata.MetadataResult, error) {
	return nil, nil
}

func newSearchHandler(access *fakeMediaRequestAccess, provider *cannedProvider) *mediaRequestHandler {
	svc := appmediarequest.NewService(newFakeMediaRequestRepo(), access, []metadata.MetadataProvider{provider}, sharedfakes.NewFakeEventBus())
	return &mediaRequestHandler{svc: svc}
}

func TestMediaRequestHandler_SearchTitles(t *testing.T) {
	year := 1999
	provider := &cannedProvider{results: []metadata.MetadataResult{{
		Title: "Some Movie", Year: &year, Overview: "plot",
		ProviderIDs: map[string]string{"tmdb": "603"},
		Artwork:     []metadata.ArtworkResult{{URL: "https://image.tmdb.org/p.jpg"}},
	}}}
	h := newSearchHandler(newFakeMediaRequestAccess(true), provider)
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.SearchTitles(ctx, &eveyv1.SearchTitlesRequest{MediaType: "movie", Query: "some", Year: 1999})
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	got := resp.Results[0]
	assert.Equal(t, "tmdb", got.Provider)
	assert.Equal(t, "603", got.ProviderId)
	assert.Equal(t, "Some Movie", got.Title)
	assert.EqualValues(t, 1999, got.Year)
	assert.Equal(t, "https://image.tmdb.org/p.jpg", got.PosterUrl)
	assert.Equal(t, "plot", got.Overview)
	require.NotNil(t, provider.lastYear)
	assert.Equal(t, 1999, *provider.lastYear)

	// Year 0 means "no year filter".
	_, err = h.SearchTitles(ctx, &eveyv1.SearchTitlesRequest{MediaType: "movie", Query: "some"})
	require.NoError(t, err)
	assert.Nil(t, provider.lastYear)

	// Refusals: bad media type, denied user (the provider is never reached), no claims.
	_, err = h.SearchTitles(ctx, &eveyv1.SearchTitlesRequest{MediaType: "podcast", Query: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	denied := newSearchHandler(newFakeMediaRequestAccess(false), provider)
	before := provider.searched
	_, err = denied.SearchTitles(ctx, &eveyv1.SearchTitlesRequest{MediaType: "movie", Query: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, before, provider.searched, "a denied user must not reach the provider")

	_, err = h.SearchTitles(context.Background(), &eveyv1.SearchTitlesRequest{MediaType: "movie", Query: "x"})
	require.Error(t, err)
}

func TestMediaRequestHandler_GetRequestPolicy(t *testing.T) {
	access := newFakeMediaRequestAccess(false)
	access.overrides["user-1"] = true
	h, _, _ := newTestMediaRequestHandlers(access)

	resp, err := h.GetRequestPolicy(ctxWithClaims("user-1", "profile-1", false), &eveyv1.GetRequestPolicyRequest{})
	require.NoError(t, err)
	assert.True(t, resp.CanRequest, "the override enables user-1")

	resp, err = h.GetRequestPolicy(ctxWithClaims("user-2", "profile-2", false), &eveyv1.GetRequestPolicyRequest{})
	require.NoError(t, err)
	assert.False(t, resp.CanRequest, "user-2 follows the disabled default")

	_, err = h.GetRequestPolicy(context.Background(), &eveyv1.GetRequestPolicyRequest{})
	require.Error(t, err)
}

func TestAdminMediaRequestHandler_SetAccessDefault(t *testing.T) {
	access := newFakeMediaRequestAccess(false)
	_, admin, _ := newTestMediaRequestHandlers(access)
	ctx := ctxWithClaims("admin-1", "profile-admin", true)

	_, err := admin.SetAccessDefault(ctx, &eveyv1.SetAccessDefaultRequest{Enabled: true})
	require.NoError(t, err)
	assert.True(t, access.global)
	_, err = admin.SetAccessDefault(ctx, &eveyv1.SetAccessDefaultRequest{Enabled: false})
	require.NoError(t, err)
	assert.False(t, access.global)
}

// Every domain refusal has its own code; anything else is an internal error
// that never leaks its text.
func TestMapMediaRequestError(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		err  error
		want codes.Code
	}{
		{mediarequest.ErrDisabled, codes.PermissionDenied},
		{mediarequest.ErrDuplicate, codes.AlreadyExists},
		{mediarequest.ErrAlreadyHeld, codes.AlreadyExists},
		{mediarequest.ErrNotFound, codes.NotFound},
		{assert.AnError, codes.Internal},
	} {
		got := mapMediaRequestError(ctx, "op", tc.err)
		assert.Equal(t, tc.want, status.Code(got), "%v", tc.err)
	}
	assert.NotContains(t, mapMediaRequestError(ctx, "op", assert.AnError).Error(), assert.AnError.Error())
}
