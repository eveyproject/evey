package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libraryfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
)

// GetCollectionItems must apply the active profile's parental filter per item.
// Without it a kid profile could enumerate adult titles through this endpoint
// even though GetCollection/ListCollections hide the collection (mutation/
// pentest 2026-05-25). Unrated items fail closed for kids, so a kid sees none
// while an admin (parental bypass) sees all.
func TestCollectionGetCollectionItems_ParentalFilter(t *testing.T) {
	ctx := context.Background()
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemStore()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libraryfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)

	maxAge := 6
	kid := &user.Profile{
		ID: "prf-kid", UserID: "usr-1", IsKids: true,
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &maxAge},
	}
	profiles := userfakes.NewFakeProfileRepository()
	require.NoError(t, profiles.Create(ctx, kid))
	uq := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profiles,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository())

	h := &collectionHandler{svc: svc, userQuery: uq}

	seedCollection(cmdRepo, "col-p", "Mixed")
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "Adult A", Type: "movie"})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m2", LibraryID: "lib-1", Title: "Adult B", Type: "movie"})
	cmdRepo.LinkItem(ctx, "col-p", "m1", 0)
	cmdRepo.LinkItem(ctx, "col-p", "m2", 1)

	kidCtx := interceptors.ContextWithClaims(ctx, &user.Claims{UserID: "usr-1", IsAdmin: false, ProfileID: "prf-kid"})
	kidResp, err := h.GetCollectionItems(kidCtx, &eveyv1.GetCollectionItemsRequest{Id: "col-p", Limit: 100})
	require.NoError(t, err)
	assert.Empty(t, kidResp.Items, "a kid profile must not see unrated (fail-closed) collection items")

	adminCtx := interceptors.ContextWithClaims(ctx, &user.Claims{UserID: "usr-1", IsAdmin: true, ProfileID: "prf-kid"})
	adminResp, err := h.GetCollectionItems(adminCtx, &eveyv1.GetCollectionItemsRequest{Id: "col-p", Limit: 100})
	require.NoError(t, err)
	assert.Len(t, adminResp.Items, 2, "admin bypasses the parental filter")
}

// The detail response carries the members now, so it owes the same per-item
// filter as the listing: handing them back here would reopen exactly the
// enumeration the guard above closes.
func TestCollectionGetCollection_ParentalFilterOnItems(t *testing.T) {
	ctx := context.Background()
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemStore()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libraryfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)

	maxAge := 6
	kid := &user.Profile{
		ID: "prf-kid", UserID: "usr-1", IsKids: true,
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &maxAge},
	}
	profiles := userfakes.NewFakeProfileRepository()
	require.NoError(t, profiles.Create(ctx, kid))
	uq := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(), profiles,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository())

	// Without a permissive access repo a non-admin is denied outright and the
	// parental path is never reached.
	h := &collectionHandler{svc: svc, userQuery: uq, userAccess: newPermissiveTestUserAccess()}

	// A MIXED collection is the case that matters: with only blocked titles the
	// collection-level guard hides the whole thing and the per-item filter is
	// never reached, so a test built that way proves nothing.
	seedCollection(cmdRepo, "col-p", "Mixed")
	mediaRepo.Create(ctx, &library.MediaItem{
		ID: "m-safe", LibraryID: "lib-1", Title: "For everyone", Type: "movie",
		Catalog:        &library.MediaCatalog{Tags: []string{}},
		Classification: &library.MediaClassification{CertificationsByCountry: map[string]string{"US": "G"}},
	})
	mediaRepo.Create(ctx, &library.MediaItem{ID: "m-adult", LibraryID: "lib-1", Title: "Unrated", Type: "movie"})
	cmdRepo.LinkItem(ctx, "col-p", "m-safe", 0)
	cmdRepo.LinkItem(ctx, "col-p", "m-adult", 1)

	adminCtx := interceptors.ContextWithClaims(ctx, &user.Claims{UserID: "usr-1", IsAdmin: true, ProfileID: "prf-kid"})
	adminResp, err := h.GetCollection(adminCtx, &eveyv1.GetCollectionRequest{Id: "col-p"})
	require.NoError(t, err)
	assert.Len(t, adminResp.Items, 2, "admin bypasses the parental filter")
	assert.EqualValues(t, 2, adminResp.Total)

	// The kid keeps the collection - one title is fine for them - but must not
	// receive the other, and the count must agree: announcing titles it cannot
	// show is the contradiction item_count exists to avoid.
	kidCtx := interceptors.ContextWithClaims(ctx, &user.Claims{UserID: "usr-1", IsAdmin: false, ProfileID: "prf-kid"})
	kidResp, err := h.GetCollection(kidCtx, &eveyv1.GetCollectionRequest{Id: "col-p"})
	require.NoError(t, err)
	require.Len(t, kidResp.Items, 1, "the unrated title must not reach a kid profile")
	assert.Equal(t, "For everyone", kidResp.Items[0].Title)
	assert.EqualValues(t, 1, kidResp.Total)
	assert.EqualValues(t, 1, kidResp.Collection.ItemCount)
}
