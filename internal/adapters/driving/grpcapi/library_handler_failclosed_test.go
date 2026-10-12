package grpcapi

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
)

// libraryHandler.getAllowedLibraryIDs used to return nil on error, and nil
// reads as "unrestricted" everywhere downstream. Four call sites (SearchMedia,
// ListMediaItems, FilterMediaItems, ListPublicMediaItems) discarded or blanked
// the error, so a grant-store outage WIDENED what the caller could see - the
// same fail-open playbackHandler already fixed. These tests pin the deny.
// Note the query layer treats an EMPTY filter.LibraryIDs as "no filter", so
// the SQL-filter sites must short-circuit instead of narrowing.

func TestLibraryHandler_SearchMedia_GrantLookupFailure_Denies(t *testing.T) {
	h, _, _, searchIdx, libraryAccess, _, _ := newTestLibraryHandler()
	ctx := userCtx("user-2", "profile-2")

	searchIdx.IndexMediaItem(ctx, &library.MediaItem{
		ID: "mi-fc-1", LibraryID: "lib-granted", Title: "Cascade One", Type: "movie",
	})
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-granted"))

	// The grant store goes down AFTER the grant exists.
	libraryAccess.Err = errors.New("library_access unavailable")

	resp, err := h.SearchMedia(ctx, &eveyv1.SearchMediaRequest{Query: "Cascade", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, resp.Items,
		"the grant lookup failed, so the caller's allowed set is UNKNOWN - "+
			"returning hits means an outage widened what the user can see")
}

func TestLibraryHandler_ListMediaItems_GrantLookupFailure_Denies(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-fc-2", LibraryID: "lib-granted", Type: "movie", Title: "Some Movie"})
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-granted"))
	libraryAccess.Err = errors.New("library_access unavailable")

	resp, err := h.ListMediaItems(userCtx("user-2", "profile-2"), &eveyv1.ListMediaItemsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

func TestLibraryHandler_FilterMediaItems_GrantLookupFailure_Denies(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-fc-3", LibraryID: "lib-granted", Type: "movie", Title: "Some Movie"})
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-granted"))
	libraryAccess.Err = errors.New("library_access unavailable")

	resp, err := h.FilterMediaItems(userCtx("user-2", "profile-2"), &eveyv1.FilterMediaItemsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

func TestLibraryHandler_ListPublicMediaItems_GrantLookupFailure_Denies(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-fc-4", LibraryID: "lib-granted", Type: "movie", Title: "Some Movie"})
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-granted"))
	libraryAccess.Err = errors.New("library_access unavailable")

	resp, err := h.ListPublicMediaItems(userCtx("user-2", "profile-2"), &eveyv1.ListPublicMediaItemsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

// A healthy grant store keeps narrowing (not denying): the restricted caller
// still sees their granted library. Pins that the fail-closed rework did not
// break legitimate restricted access.
func TestLibraryHandler_ListMediaItems_GrantedLibraryStillVisible(t *testing.T) {
	h, _, mediaQuery, _, libraryAccess, _, _ := newTestLibraryHandler()

	mediaQuery.Seed(&library.MediaItem{ID: "mi-fc-5", LibraryID: "lib-granted", Type: "movie", Title: "Some Movie"})
	mediaQuery.Seed(&library.MediaItem{ID: "mi-fc-6", LibraryID: "lib-other", Type: "movie", Title: "Some Other Movie"})
	require.NoError(t, libraryAccess.Grant(context.Background(), "user-2", "lib-granted"))

	resp, err := h.ListMediaItems(userCtx("user-2", "profile-2"), &eveyv1.ListMediaItemsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "mi-fc-5", resp.Items[0].Id)
}
