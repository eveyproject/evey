package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appfavorite "github.com/gravity-zero/evey/internal/app/favorite"
	favfakes "github.com/gravity-zero/evey/internal/domain/favorite/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestFavoriteHandler() (*favoriteHandler, *favfakes.FakeRepository) {
	repo := favfakes.NewFakeRepository()
	svc := appfavorite.NewService(repo)
	return &favoriteHandler{svc: svc}, repo
}

func TestFavoriteHandler_ListFavorites_Unauthenticated(t *testing.T) {
	h, _ := newTestFavoriteHandler()

	_, err := h.ListFavorites(context.Background(), &eveyv1.ListFavoritesRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestFavoriteHandler_ListFavorites_Empty(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{})

	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

func TestFavoriteHandler_AddFavorite_Toggle(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.True(t, resp.IsFavorite)

	// Toggle off
	resp2, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.False(t, resp2.IsFavorite)
}

func TestFavoriteHandler_AddFavorite_Unauthenticated(t *testing.T) {
	h, _ := newTestFavoriteHandler()

	_, err := h.AddFavorite(context.Background(), &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})

	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestFavoriteHandler_AddFavorite_RepoError(t *testing.T) {
	h, repo := newTestFavoriteHandler()
	repo.Err = errors.New("db error")
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestFavoriteHandler_ListFavorites_WithItems(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	_, err = h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m2"})
	require.NoError(t, err)

	resp, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Items, 2)
}

func TestFavoriteHandler_RemoveFavorite_NotFavorited(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.RemoveFavorite(ctx, &eveyv1.RemoveFavoriteRequest{MediaItemId: "nonexistent"})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestFavoriteHandler_RemoveFavorite_Success(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)

	_, err = h.RemoveFavorite(ctx, &eveyv1.RemoveFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)

	list, err := h.ListFavorites(ctx, &eveyv1.ListFavoritesRequest{})
	require.NoError(t, err)
	assert.Empty(t, list.Items)
}

func TestFavoriteHandler_IsFavorite_False(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	resp, err := h.IsFavorite(ctx, &eveyv1.IsFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.False(t, resp.IsFavorite)
}

func TestFavoriteHandler_IsFavorite_True(t *testing.T) {
	h, _ := newTestFavoriteHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)

	_, err := h.AddFavorite(ctx, &eveyv1.AddFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)

	resp, err := h.IsFavorite(ctx, &eveyv1.IsFavoriteRequest{MediaItemId: "m1"})
	require.NoError(t, err)
	assert.True(t, resp.IsFavorite)
}

func TestFavoriteHandler_IsFavorite_Unauthenticated(t *testing.T) {
	h, _ := newTestFavoriteHandler()

	_, err := h.IsFavorite(context.Background(), &eveyv1.IsFavoriteRequest{MediaItemId: "m1"})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
