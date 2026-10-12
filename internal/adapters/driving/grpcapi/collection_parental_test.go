package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appcollection "github.com/eveyproject/evey/internal/app/collection"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	libraryfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
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
	cmdSvc, querySvc := appcollection.NewCommandService(cmdRepo, queryRepo, mediaRepo), appcollection.NewQueryService(queryRepo, smartQ)

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

	h := &collectionHandler{cmd: cmdSvc, query: querySvc, userQuery: uq}

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
	cmdSvc, querySvc := appcollection.NewCommandService(cmdRepo, queryRepo, mediaRepo), appcollection.NewQueryService(queryRepo, smartQ)

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
	h := &collectionHandler{cmd: cmdSvc, query: querySvc, userQuery: uq, userAccess: newPermissiveTestUserAccess()}

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

// The collection reads a kids profile reaches each filter what its parental
// rules hide. Neutralising any of these three filters left the package green
// (sabotage sweep, 2026-09-28): only ListCollections had a test.
//
// col-clean holds a clean title, col-blocked only a title tagged "violence",
// col-mixed both. Both titles carry a US "G" rating so the age rule passes and
// the blocked tag is the only difference.
func newKidsCollectionFixture(t *testing.T) (*collectionHandler, context.Context) {
	t.Helper()
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemStore()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	cmdSvc := appcollection.NewCommandService(cmdRepo, queryRepo, mediaRepo)
	querySvc := appcollection.NewQueryService(queryRepo, libraryfakes.NewFakeCollectionSmartQuerier())

	maxAge := 7
	kid := &user.Profile{
		ID: "prf-kid", UserID: "usr-kid", Name: "Kid", IsKids: true,
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &maxAge, BlockedTags: []string{"violence"}},
	}
	profileRepo := userfakes.NewFakeProfileRepository()
	if err := profileRepo.Create(context.Background(), kid); err != nil {
		t.Fatal(err)
	}
	userQuery := appuser.NewQueryService(userfakes.NewFakeUserRepository(), profileRepo,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(), userfakes.NewFakeQuickConnectRepository())
	h := &collectionHandler{cmd: cmdSvc, query: querySvc, userQuery: userQuery, userAccess: newPermissiveTestUserAccess()}

	g := map[string]string{"US": "G"}
	_ = mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m-clean", LibraryID: "lib-1", Title: "Safe", Type: "movie",
		Catalog: &library.MediaCatalog{Tags: []string{}}, Classification: &library.MediaClassification{CertificationsByCountry: g}})
	_ = mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m-violent", LibraryID: "lib-1", Title: "Action", Type: "movie",
		Catalog: &library.MediaCatalog{Tags: []string{"violence"}}, Classification: &library.MediaClassification{CertificationsByCountry: g}})
	seedCollection(cmdRepo, "col-clean", "Clean")
	seedCollection(cmdRepo, "col-blocked", "Blocked")
	seedCollection(cmdRepo, "col-mixed", "Mixed")
	_ = cmdRepo.LinkItem(context.Background(), "col-clean", "m-clean", 0)
	_ = cmdRepo.LinkItem(context.Background(), "col-blocked", "m-violent", 0)
	_ = cmdRepo.LinkItem(context.Background(), "col-mixed", "m-clean", 0)
	_ = cmdRepo.LinkItem(context.Background(), "col-mixed", "m-violent", 1)

	ctx := interceptors.ContextWithClaims(context.Background(), &user.Claims{UserID: "usr-kid", Username: "kid", ProfileID: string(kid.ID)})
	return h, ctx
}

func TestCollectionGetCollection_ParentalHidesAWhollyBlockedCollection(t *testing.T) {
	h, kidCtx := newKidsCollectionFixture(t)
	if _, err := h.GetCollection(kidCtx, &eveyv1.GetCollectionRequest{Id: "col-blocked"}); status.Code(err) != codes.NotFound {
		t.Errorf("a collection of hidden titles must read as not found, got %v", err)
	}
	if _, err := h.GetCollection(kidCtx, &eveyv1.GetCollectionRequest{Id: "col-clean"}); err != nil {
		t.Errorf("control: a clean collection must open: %v", err)
	}
}

func TestCollectionListCollectionsForMediaItem_ParentalSkipsBlockedCollections(t *testing.T) {
	h, kidCtx := newKidsCollectionFixture(t)
	resp, err := h.ListCollectionsForMediaItem(kidCtx, &eveyv1.ListCollectionsForMediaItemRequest{MediaItemId: "m-violent"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range resp.Collections {
		if c.Id == "col-blocked" {
			t.Errorf("a collection whose every title is hidden must not be listed, got %v", c.Id)
		}
	}
	// Control: the clean title's collections are listed.
	clean, err := h.ListCollectionsForMediaItem(kidCtx, &eveyv1.ListCollectionsForMediaItemRequest{MediaItemId: "m-clean"})
	if err != nil || len(clean.Collections) == 0 {
		t.Errorf("control: the clean title's collections must be listed, got %v (%v)", clean, err)
	}
}

func TestCollectionGetCollectionItems_ParentalDropsBlockedTitles(t *testing.T) {
	h, kidCtx := newKidsCollectionFixture(t)
	resp, err := h.GetCollectionItems(kidCtx, &eveyv1.GetCollectionItemsRequest{Id: "col-mixed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range resp.MediaItemIds {
		if id == "m-violent" {
			t.Errorf("a hidden title must not be among the items, got %v", resp.MediaItemIds)
		}
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1 (the hidden title does not count)", resp.Total)
	}
}
