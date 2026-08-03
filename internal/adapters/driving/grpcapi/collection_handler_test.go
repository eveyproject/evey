package grpcapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libraryfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestCollectionHandler() (*collectionHandler, *libraryfakes.FakeCollectionCommandRepo) {
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemRepository()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libraryfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)
	return &collectionHandler{svc: svc, userAccess: newPermissiveTestUserAccess()}, cmdRepo
}

var tmdbCounter int

func seedCollection(cmdRepo *libraryfakes.FakeCollectionCommandRepo, id, name string) {
	tmdbCounter++
	tmdb := tmdbCounter
	cmdRepo.Upsert(context.Background(), &library.Collection{
		ID:        id,
		Name:      name,
		TmdbID:    &tmdb,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
}

func TestCollectionListCollections_Empty(t *testing.T) {
	h, _ := newTestCollectionHandler()
	resp, err := h.ListCollections(context.Background(), &eveyv1.ListCollectionsRequest{})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 0 {
		t.Errorf("expected 0 collections, got %d", len(resp.Collections))
	}
}

func TestCollectionListCollections_WithData(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-1", "Marvel Cinematic Universe")
	seedCollection(cmdRepo, "col-2", "DC Extended Universe")
	// Real sagas hold titles: an empty auto saga is a stub the listing hides.
	for i, col := range []string{"col-1", "col-2"} {
		for j := range 2 {
			id := shared.MediaItemID(fmt.Sprintf("wd%d%d", i, j))
			mediaRepo.Create(context.Background(), &library.MediaItem{ID: id, LibraryID: "lib-1", Title: string(id), Type: "movie"})
			cmdRepo.LinkItem(context.Background(), col, string(id), 0)
		}
	}

	resp, err := h.ListCollections(adminCtx(), &eveyv1.ListCollectionsRequest{MinItems: 1})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 2 {
		t.Errorf("expected 2 collections, got %d", len(resp.Collections))
	}
}

func TestCollectionGetCollection_Found(t *testing.T) {
	h, cmdRepo := newTestCollectionHandler()
	seedCollection(cmdRepo, "col-1", "Avengers")

	resp, err := h.GetCollection(context.Background(), &eveyv1.GetCollectionRequest{Id: "col-1"})
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if resp.Collection.Id != "col-1" {
		t.Errorf("expected id 'col-1', got %q", resp.Collection.Id)
	}
	if resp.Collection.Name != "Avengers" {
		t.Errorf("expected name 'Avengers', got %q", resp.Collection.Name)
	}
}

func TestCollectionGetCollection_NotFound(t *testing.T) {
	h, _ := newTestCollectionHandler()
	_, err := h.GetCollection(context.Background(), &eveyv1.GetCollectionRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent collection")
	}
}

// TestCollectionGetCollection_UserAccessUnwired_DeniesNonAdmin proves the
// hardening: a handler built with userAccess left nil (a wiring regression,
// never true in production since wire.go always constructs it) must deny a
// non-admin caller, not silently show every collection like the old
// userAccess==nil fail-open guard did.
func TestCollectionGetCollection_UserAccessUnwired_DeniesNonAdmin(t *testing.T) {
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemRepository()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libraryfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)
	h := &collectionHandler{svc: svc} // userAccess intentionally left nil

	seedCollection(cmdRepo, "col-1", "Some Collection")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-1", "m1", 0)

	_, err := h.GetCollection(nonAdminCtx("user-1"), &eveyv1.GetCollectionRequest{Id: "col-1"})
	if err == nil {
		t.Fatal("expected NotFound: an unwired userAccess port must deny, not return everything")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code: want NotFound, got %v", got)
	}
}

func TestCollectionDeleteCollection_Success(t *testing.T) {
	h, cmdRepo := newTestCollectionHandler()
	seedCollection(cmdRepo, "col-1", "Avengers")

	_, err := h.DeleteCollection(context.Background(), &eveyv1.DeleteCollectionRequest{Id: "col-1"})
	if err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}

	_, err = h.GetCollection(context.Background(), &eveyv1.GetCollectionRequest{Id: "col-1"})
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func newTestCollectionHandlerWithAccess() (*collectionHandler, *libraryfakes.FakeCollectionCommandRepo, *libraryfakes.FakeMediaItemRepository, *userfakes.FakeLibraryAccessRepository) {
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemRepository()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libraryfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)
	libAccessRepo := userfakes.NewFakeLibraryAccessRepository()
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccessRepo),
	)
	return &collectionHandler{svc: svc, userAccess: userAccess}, cmdRepo, mediaRepo, libAccessRepo
}

func nonAdminCtx(userID string) context.Context {
	return interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:   userID,
		Username: "viewer",
		IsAdmin:  false,
	})
}

func TestCollectionListCollections_IDOR_FiltersByLibraryAccess(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-lib1", "Private Lib1")
	seedCollection(cmdRepo, "col-lib2", "Lib2 Access OK")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m2", LibraryID: "lib-2", Title: "B", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-lib1", "m1", 0)
	cmdRepo.LinkItem(context.Background(), "col-lib2", "m2", 0)
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	resp, err := h.ListCollections(nonAdminCtx("user-1"), &eveyv1.ListCollectionsRequest{MinItems: 1})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 1 || resp.Collections[0].Id != "col-lib2" {
		t.Errorf("expected only col-lib2, got %+v", resp.Collections)
	}
}

// A saga you hold ONE title of is a title, not a collection. 230 of the 236
// auto-created TMDB sagas on the production server were exactly that
// (measured 2026-08-04), which is what made the collections page unusable:
// a wall of one-title stubs hiding the six real sagas. The listing hides them
// by default; min_items=1 is the curation escape hatch.
func TestCollectionListCollections_HidesOneTitleStubs(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-stub", "Alien - Saga")
	seedCollection(cmdRepo, "col-real", "Matrix - Saga")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "Alien", Type: "movie"})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m2", LibraryID: "lib-1", Title: "Matrix", Type: "movie"})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m3", LibraryID: "lib-1", Title: "Matrix Reloaded", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-stub", "m1", 0)
	cmdRepo.LinkItem(context.Background(), "col-real", "m2", 0)
	cmdRepo.LinkItem(context.Background(), "col-real", "m3", 0)

	resp, err := h.ListCollections(adminCtx(), &eveyv1.ListCollectionsRequest{})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 1 || resp.Collections[0].Id != "col-real" {
		t.Fatalf("expected only the 2-title saga, got %+v", resp.Collections)
	}

	all, err := h.ListCollections(adminCtx(), &eveyv1.ListCollectionsRequest{MinItems: 1})
	if err != nil {
		t.Fatalf("ListCollections(min_items=1): %v", err)
	}
	if len(all.Collections) != 2 {
		t.Errorf("min_items=1 must list every collection, got %d", len(all.Collections))
	}
}

// The stub threshold applies to AUTO-created sagas only. A collection someone
// built by hand stays listed while still empty - hiding it would leave them
// nothing to add titles to.
func TestCollectionListCollections_KeepsEmptyHandBuiltCollections(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-stub-saga", "Alien - Saga") // auto, 1 title
	cmdRepo.Upsert(context.Background(), &library.Collection{ID: "col-empty", Name: "Soirée pizza"})
	cmdRepo.Upsert(context.Background(), &library.Collection{
		ID: "col-smart-empty", Name: "Années 80",
		Rules: []library.CollectionRule{{Field: "year", Operator: "lt", Value: "1990"}},
	})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "Alien", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-stub-saga", "m1", 0)

	resp, err := h.ListCollections(adminCtx(), &eveyv1.ListCollectionsRequest{})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	ids := map[string]bool{}
	for _, c := range resp.Collections {
		ids[c.Id] = true
	}
	if !ids["col-empty"] || !ids["col-smart-empty"] {
		t.Errorf("hand-built and smart collections must stay listed while empty, got %v", ids)
	}
	if ids["col-stub-saga"] {
		t.Error("the one-title auto saga must still be hidden")
	}
}

// Every surface needs the member count ("4 films") and no client can compute
// it - it never receives the members of every collection.
func TestCollectionListCollections_ReportsItemCount(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-real", "Matrix - Saga")
	for _, id := range []string{"m1", "m2", "m3"} {
		mediaRepo.Create(context.Background(), &library.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib-1", Title: id, Type: "movie"})
		cmdRepo.LinkItem(context.Background(), "col-real", id, 0)
	}
	resp, err := h.ListCollections(adminCtx(), &eveyv1.ListCollectionsRequest{})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 1 {
		t.Fatalf("expected 1 collection, got %d", len(resp.Collections))
	}
	if got := resp.Collections[0].ItemCount; got != 3 {
		t.Errorf("item_count = %d, want 3", got)
	}
}

// The count must be what THIS caller can see, or it advertises titles they
// cannot reach - and turns the count into an existence oracle for a library
// they were denied.
func TestCollectionListCollections_ItemCountIsAccessFiltered(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-mixed", "Mixed")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "Hidden", Type: "movie"})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m2", LibraryID: "lib-2", Title: "Seen A", Type: "movie"})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m3", LibraryID: "lib-2", Title: "Seen B", Type: "movie"})
	for _, id := range []string{"m1", "m2", "m3"} {
		cmdRepo.LinkItem(context.Background(), "col-mixed", id, 0)
	}
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	resp, err := h.ListCollections(nonAdminCtx("user-1"), &eveyv1.ListCollectionsRequest{})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 1 {
		t.Fatalf("expected the collection to stay visible, got %d", len(resp.Collections))
	}
	if got := resp.Collections[0].ItemCount; got != 2 {
		t.Errorf("item_count = %d, want 2 (the lib-1 title must not be counted)", got)
	}
}

// kind lets a client badge a TMDB saga apart from a hand-built list without
// inferring it from the presence of tmdb_id.
func TestCollectionListCollections_ReportsKind(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-saga", "Matrix - Saga") // seedCollection stamps a TmdbID
	cmdRepo.Upsert(context.Background(), &library.Collection{ID: "col-manual", Name: "Soirée pizza"})
	cmdRepo.Upsert(context.Background(), &library.Collection{
		ID: "col-smart", Name: "Années 80",
		Rules: []library.CollectionRule{{Field: "year", Operator: "lt", Value: "1990"}},
	})
	for i, col := range []string{"col-saga", "col-manual", "col-smart"} {
		for j := range 2 {
			id := shared.MediaItemID(fmt.Sprintf("m%d%d", i, j))
			mediaRepo.Create(context.Background(), &library.MediaItem{ID: id, LibraryID: "lib-1", Title: string(id), Type: "movie"})
			cmdRepo.LinkItem(context.Background(), col, string(id), 0)
		}
	}
	resp, err := h.ListCollections(adminCtx(), &eveyv1.ListCollectionsRequest{})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	kinds := map[string]string{}
	for _, c := range resp.Collections {
		kinds[c.Id] = c.Kind
	}
	for id, want := range map[string]string{"col-saga": "saga", "col-manual": "manual", "col-smart": "smart"} {
		if kinds[id] != want {
			t.Errorf("kind[%s] = %q, want %q", id, kinds[id], want)
		}
	}
}

func TestCollectionGetCollection_IDOR_DeniesUnauthorizedLibrary(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-lib1", "Private")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-lib1", "m1", 0)
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	_, err := h.GetCollection(nonAdminCtx("user-1"), &eveyv1.GetCollectionRequest{Id: "col-lib1"})
	if err == nil {
		t.Error("expected NotFound for unauthorized collection")
	}
}

func TestCollectionGetCollection_IDOR_AllowsAuthorizedLibrary(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-lib2", "OK")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m2", LibraryID: "lib-2", Title: "B", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-lib2", "m2", 0)
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	resp, err := h.GetCollection(nonAdminCtx("user-1"), &eveyv1.GetCollectionRequest{Id: "col-lib2"})
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if resp.Collection.Id != "col-lib2" {
		t.Errorf("expected col-lib2, got %q", resp.Collection.Id)
	}
}

// IDOR regression: GetCollectionItems must NOT leak items from libraries
// the caller can't see, even when the collection itself is visible.
// A collection mixing lib-1 + lib-2 items, viewed by someone with only
// lib-2 access, should only return lib-2 items (and total adjusted).
func TestCollectionGetCollectionItems_IDOR_FiltersByLibraryAccess(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-mix", "Mixed")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m-priv", LibraryID: "lib-1", Title: "Private", Type: "movie"})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m-pub", LibraryID: "lib-2", Title: "Public", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-mix", "m-priv", 0)
	cmdRepo.LinkItem(context.Background(), "col-mix", "m-pub", 1)
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	resp, err := h.GetCollectionItems(nonAdminCtx("user-1"), &eveyv1.GetCollectionItemsRequest{Id: "col-mix", Limit: 100})
	if err != nil {
		t.Fatalf("GetCollectionItems: %v", err)
	}
	if len(resp.MediaItemIds) != 1 || resp.MediaItemIds[0] != "m-pub" {
		t.Errorf("expected only m-pub, got %v", resp.MediaItemIds)
	}
	if len(resp.Items) != 1 || resp.Items[0].Id != "m-pub" {
		t.Errorf("expected only m-pub in Items, got %+v", resp.Items)
	}
	if resp.Total != 1 {
		t.Errorf("Total should reflect filtered count, got %d", resp.Total)
	}
}

func TestCollectionCreateCollection_EmptyName_400(t *testing.T) {
	h, _ := newTestCollectionHandler()
	_, err := h.CreateCollection(context.Background(), &eveyv1.CreateCollectionRequest{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", got)
	}
}

func TestCollectionCreateCollection_OK(t *testing.T) {
	h, _ := newTestCollectionHandler()
	resp, err := h.CreateCollection(context.Background(), &eveyv1.CreateCollectionRequest{
		Name:   "New Collection",
		Smart:  false,
		SortBy: "title",
	})
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if resp.Collection == nil || resp.Collection.Id == "" {
		t.Fatal("expected a collection with id")
	}
	if resp.Collection.Name != "New Collection" {
		t.Errorf("expected name 'New Collection', got %q", resp.Collection.Name)
	}
	list, err := h.ListCollections(context.Background(), &eveyv1.ListCollectionsRequest{MinItems: 1})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	found := false
	for _, c := range list.Collections {
		if c.Id == resp.Collection.Id {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected created collection to be listed")
	}
}

func TestCollectionCreateCollection_Smart_OK(t *testing.T) {
	h, _ := newTestCollectionHandler()
	resp, err := h.CreateCollection(context.Background(), &eveyv1.CreateCollectionRequest{
		Name:  "Smart Col",
		Smart: true,
		Rules: []*eveyv1.CollectionRule{{Field: "genre", Operator: "equals", Value: "action"}},
	})
	if err != nil {
		t.Fatalf("CreateCollection smart: %v", err)
	}
	if resp.Collection == nil || resp.Collection.Id == "" {
		t.Fatal("expected a smart collection with id")
	}
	if len(resp.Collection.Rules) != 1 || resp.Collection.Rules[0].Field != "genre" {
		t.Errorf("expected rules to round-trip, got %+v", resp.Collection.Rules)
	}
}

func TestCollectionUpdateCollection_OK(t *testing.T) {
	h, cmdRepo := newTestCollectionHandler()
	seedCollection(cmdRepo, "col-1", "Old Name")
	resp, err := h.UpdateCollection(context.Background(), &eveyv1.UpdateCollectionRequest{
		Id:     "col-1",
		Name:   "New Name",
		SortBy: "title",
	})
	if err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	if resp.Collection == nil || resp.Collection.Name != "New Name" {
		t.Errorf("expected name 'New Name', got %+v", resp.Collection)
	}
}

func TestCollectionUpdateCollection_NotFound(t *testing.T) {
	h, _ := newTestCollectionHandler()
	_, err := h.UpdateCollection(context.Background(), &eveyv1.UpdateCollectionRequest{
		Id:   "nonexistent",
		Name: "X",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent collection")
	}
}

func TestCollectionUpdateCollection_MissingName_400(t *testing.T) {
	h, _ := newTestCollectionHandler()
	_, err := h.UpdateCollection(context.Background(), &eveyv1.UpdateCollectionRequest{
		Id:   "col-1",
		Name: "",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", got)
	}
}

func TestCollectionAddCollectionItem_OK(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-1", "C")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	_, err := h.AddCollectionItem(context.Background(), &eveyv1.AddCollectionItemRequest{
		Id:          "col-1",
		MediaItemId: "m1",
		SortOrder:   0,
	})
	if err != nil {
		t.Fatalf("AddCollectionItem: %v", err)
	}
}

func TestCollectionAddCollectionItem_MissingID_400(t *testing.T) {
	h, _ := newTestCollectionHandler()
	_, err := h.AddCollectionItem(context.Background(), &eveyv1.AddCollectionItemRequest{
		Id:          "",
		MediaItemId: "m1",
	})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", got)
	}
}

func TestCollectionAddCollectionItem_MissingMediaItemID_400(t *testing.T) {
	h, _ := newTestCollectionHandler()
	_, err := h.AddCollectionItem(context.Background(), &eveyv1.AddCollectionItemRequest{
		Id:          "col-1",
		MediaItemId: "",
	})
	if err == nil {
		t.Fatal("expected error for missing media_item_id")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", got)
	}
}

// Defense-in-depth IDOR: DeleteCollection must reject non-admin callers
// without library access on any item in the collection. RBAC already gates
// this RPC to admins, so this guards against future RBAC misconfiguration.
func TestCollectionDeleteCollection_IDOR_DeniesNonAdminWithoutAccess(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-priv", "Private")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-priv", "m1", 0)
	// User has access to a different library.
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	_, err := h.DeleteCollection(nonAdminCtx("user-1"), &eveyv1.DeleteCollectionRequest{Id: "col-priv"})
	if err == nil {
		t.Fatal("expected NotFound for non-admin without library access")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code: want NotFound, got %v", got)
	}
	// Collection must still exist (delete should not have proceeded). Use
	// an admin context to bypass the access check itself.
	if _, getErr := h.GetCollection(adminCtx(), &eveyv1.GetCollectionRequest{Id: "col-priv"}); getErr != nil {
		t.Fatalf("collection should still exist after blocked delete: %v", getErr)
	}
}

// Admin path on DeleteCollection must remain unaffected (short-circuit).
func TestCollectionDeleteCollection_AdminBypassAccess(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-priv", "Private")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-priv", "m1", 0)
	// Even with no library access grants, an admin context must succeed.
	_ = libAccessRepo

	_, err := h.DeleteCollection(adminCtx(), &eveyv1.DeleteCollectionRequest{Id: "col-priv"})
	if err != nil {
		t.Fatalf("admin DeleteCollection: %v", err)
	}
}

// Defense-in-depth IDOR: UpdateCollection must reject non-admin callers
// without library access on any item in the collection.
func TestCollectionUpdateCollection_IDOR_DeniesNonAdminWithoutAccess(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-priv", "Private")
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-priv", "m1", 0)
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	_, err := h.UpdateCollection(nonAdminCtx("user-1"), &eveyv1.UpdateCollectionRequest{
		Id:   "col-priv",
		Name: "Hijacked",
	})
	if err == nil {
		t.Fatal("expected NotFound for non-admin without library access")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code: want NotFound, got %v", got)
	}
	// Collection name must be unchanged.
	got, getErr := h.GetCollection(adminCtx(), &eveyv1.GetCollectionRequest{Id: "col-priv"})
	if getErr != nil {
		t.Fatalf("get collection: %v", getErr)
	}
	if got.Collection.Name == "Hijacked" {
		t.Fatal("collection name was mutated despite blocked update")
	}
}

// Admin path on UpdateCollection must remain unaffected.
func TestCollectionUpdateCollection_AdminBypassAccess(t *testing.T) {
	h, cmdRepo, _, _ := newTestCollectionHandlerWithAccess()
	seedCollection(cmdRepo, "col-priv", "Old")

	resp, err := h.UpdateCollection(adminCtx(), &eveyv1.UpdateCollectionRequest{
		Id:   "col-priv",
		Name: "New",
	})
	if err != nil {
		t.Fatalf("admin UpdateCollection: %v", err)
	}
	if resp.Collection == nil || resp.Collection.Name != "New" {
		t.Fatalf("expected name 'New', got %+v", resp.Collection)
	}
}

func TestCollectionDeleteCollection_EmptyID_400(t *testing.T) {
	h, _ := newTestCollectionHandler()
	_, err := h.DeleteCollection(context.Background(), &eveyv1.DeleteCollectionRequest{Id: ""})
	if err == nil {
		t.Fatal("expected error for empty id")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("code: want InvalidArgument, got %v", got)
	}
}

func TestCollectionGetCollectionItems_Empty(t *testing.T) {
	h, cmdRepo := newTestCollectionHandler()
	seedCollection(cmdRepo, "col-1", "Avengers")

	resp, err := h.GetCollectionItems(context.Background(), &eveyv1.GetCollectionItemsRequest{
		Id:    "col-1",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("GetCollectionItems: %v", err)
	}
	if len(resp.MediaItemIds) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.MediaItemIds))
	}
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
}

// newTestCollectionHandlerBatch constructs a handler and returns the underlying
// FakeCollectionQueryRepo so tests can inspect GetItemsByCollectionIDsCallCount.
func newTestCollectionHandlerBatch() (*collectionHandler, *libraryfakes.FakeCollectionCommandRepo, *libraryfakes.FakeMediaItemRepository, *libraryfakes.FakeCollectionQueryRepo) {
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemRepository()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libraryfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)
	return &collectionHandler{svc: svc, userAccess: newPermissiveTestUserAccess()}, cmdRepo, mediaRepo, queryRepo
}

// TestCollectionListCollections_BatchCalledOnce verifies that ListCollections
// issues exactly ONE call to GetItemsByCollectionIDs regardless of the number
// of collections, i.e. the N+1 has been eliminated.
func TestCollectionListCollections_BatchCalledOnce(t *testing.T) {
	h, cmdRepo, mediaRepo, queryRepo := newTestCollectionHandlerBatch()

	// Seed three collections, each with an item in the same library so they
	// are all visible to an admin context.
	for i, id := range []string{"col-a", "col-b", "col-c"} {
		seedCollection(cmdRepo, id, "Col "+id)
		mID := shared.MediaItemID("m-batch-" + id)
		_ = mediaRepo.Create(context.Background(), &library.MediaItem{
			ID: mID, LibraryID: "lib-1", Title: "T", Type: "movie",
		})
		_ = cmdRepo.LinkItem(context.Background(), id, string(mID), i)
	}

	resp, err := h.ListCollections(context.Background(), &eveyv1.ListCollectionsRequest{MinItems: 1})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 3 {
		t.Errorf("expected 3 collections, got %d", len(resp.Collections))
	}

	// The batch repo must have been called exactly once.
	if got := queryRepo.GetItemsByCollectionIDsCallCount; got != 1 {
		t.Errorf("GetItemsByCollectionIDs call count = %d, want 1", got)
	}
}

// TestCollectionListCollections_LibraryAccess_BatchCalledOnce proves the
// library-access filter still works correctly under the batched path, AND
// that the batch is issued only once even for a restricted caller.
func TestCollectionListCollections_LibraryAccess_BatchCalledOnce(t *testing.T) {
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemRepository()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libraryfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)
	libAccessRepo := userfakes.NewFakeLibraryAccessRepository()
	userAccess := appuser.NewAccessService(
		userfakes.NewFakeUserRepository(),
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
		userfakes.NewFakePasswordHasher(),
		sharedfakes.NewFakeEventBus(),
		appuser.WithLibraryAccess(libAccessRepo),
	)
	h := &collectionHandler{svc: svc, userAccess: userAccess}

	// col-allowed: item in lib-2 (user has access)
	// col-denied:  item in lib-1 (user has no access)
	seedCollection(cmdRepo, "col-allowed", "OK")
	seedCollection(cmdRepo, "col-denied", "Hidden")
	_ = mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m-ok", LibraryID: "lib-2", Title: "A", Type: "movie"})
	_ = mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m-no", LibraryID: "lib-1", Title: "B", Type: "movie"})
	_ = cmdRepo.LinkItem(context.Background(), "col-allowed", "m-ok", 0)
	_ = cmdRepo.LinkItem(context.Background(), "col-denied", "m-no", 0)
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	resp, err := h.ListCollections(nonAdminCtx("user-1"), &eveyv1.ListCollectionsRequest{MinItems: 1})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 1 || resp.Collections[0].Id != "col-allowed" {
		t.Errorf("expected only col-allowed, got %+v", resp.Collections)
	}
	if got := queryRepo.GetItemsByCollectionIDsCallCount; got != 1 {
		t.Errorf("GetItemsByCollectionIDs call count = %d, want 1", got)
	}
}

// TestCollectionListCollections_ParentalFilter_Batch verifies that collections
// whose items are all blocked by the active parental filter are hidden from the
// listing, while collections with at least one unblocked item remain visible.
// This proves the parental semantics are preserved under the batched path.
func TestCollectionListCollections_ParentalFilter_Batch(t *testing.T) {
	cmdRepo := libraryfakes.NewFakeCollectionCommandRepo()
	mediaRepo := libraryfakes.NewFakeMediaItemRepository()
	queryRepo := libraryfakes.NewFakeCollectionQueryRepo(cmdRepo, mediaRepo)
	smartQ := libraryfakes.NewFakeCollectionSmartQuerier()
	svc := appcollection.NewService(cmdRepo, queryRepo, smartQ, mediaRepo)

	// Kid profile blocking items tagged "violence".
	maxAge := 7
	kidProfile := &user.Profile{
		ID:     shared.ProfileID("prf-kid"),
		UserID: shared.UserID("usr-kid"),
		Name:   "Kid",
		IsKids: true,
		Parental: &user.ParentalSettings{
			Enabled:     true,
			MaxAge:      &maxAge,
			BlockedTags: []string{"violence"},
		},
	}
	profileRepo := userfakes.NewFakeProfileRepository()
	if err := profileRepo.Create(context.Background(), kidProfile); err != nil {
		t.Fatalf("seed kid profile: %v", err)
	}
	userQuery := appuser.NewQueryService(
		userfakes.NewFakeUserRepository(),
		profileRepo,
		userfakes.NewFakeAPIKeyRepository(),
		userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository(),
	)
	h := &collectionHandler{svc: svc, userQuery: userQuery, userAccess: newPermissiveTestUserAccess()}

	// col-clean:  item rated "G" (US, age 0) and no blocked tags → should appear
	// col-blocked: item rated "G" but tagged "violence" (blocked tag) → should be hidden
	//
	// Both items carry a "US: G" certification so they pass the age check
	// (age 0 ≤ maxAge 7). The distinction is the blocked tag on m-violent.
	// Items with no resolvable rating are fail-closed for kids (worstAge=-1
	// triggers the guard), so we must supply a rating for the visible item.
	seedCollection(cmdRepo, "col-clean", "Family Film")
	seedCollection(cmdRepo, "col-blocked", "Action Only")
	_ = mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "m-clean", LibraryID: "lib-1", Title: "Safe", Type: "movie",
		Catalog:        &library.MediaCatalog{Tags: []string{}},
		Classification: &library.MediaClassification{CertificationsByCountry: map[string]string{"US": "G"}},
	})
	_ = mediaRepo.Create(context.Background(), &library.MediaItem{
		ID: "m-violent", LibraryID: "lib-1", Title: "Action", Type: "movie",
		Catalog:        &library.MediaCatalog{Tags: []string{"violence"}},
		Classification: &library.MediaClassification{CertificationsByCountry: map[string]string{"US": "G"}},
	})
	_ = cmdRepo.LinkItem(context.Background(), "col-clean", "m-clean", 0)
	_ = cmdRepo.LinkItem(context.Background(), "col-blocked", "m-violent", 0)

	kidCtx := interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID:    "usr-kid",
		Username:  "kid",
		IsAdmin:   false,
		ProfileID: string(kidProfile.ID),
	})

	resp, err := h.ListCollections(kidCtx, &eveyv1.ListCollectionsRequest{MinItems: 1})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 1 || resp.Collections[0].Id != "col-clean" {
		t.Errorf("expected only col-clean, got %+v", resp.Collections)
	}
	// Batch issued exactly once even under parental filter path.
	if got := queryRepo.GetItemsByCollectionIDsCallCount; got != 1 {
		t.Errorf("GetItemsByCollectionIDs call count = %d, want 1", got)
	}
}

// AddCollectionItem shipped without a counterpart: a collection could only
// ever grow, so a wrong member was permanent and curation was write-only.
func TestCollectionRemoveCollectionItem_OK(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	cmdRepo.Upsert(context.Background(), &library.Collection{ID: "col-1", Name: "Ma liste"})
	for _, id := range []string{"m1", "m2"} {
		mediaRepo.Create(context.Background(), &library.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib-1", Title: id, Type: "movie"})
		cmdRepo.LinkItem(context.Background(), "col-1", id, 0)
	}

	if _, err := h.RemoveCollectionItem(adminCtx(), &eveyv1.RemoveCollectionItemRequest{Id: "col-1", MediaItemId: "m1"}); err != nil {
		t.Fatalf("RemoveCollectionItem: %v", err)
	}
	items, _, err := h.svc.GetItems(context.Background(), "col-1", 100, 0)
	if err != nil {
		t.Fatalf("GetItems: %v", err)
	}
	if len(items) != 1 || string(items[0].ID) != "m2" {
		t.Errorf("after removal, items = %+v, want only m2", items)
	}
}

// Removing what is not there is the state the caller asked for, not an error.
func TestCollectionRemoveCollectionItem_AbsentIsNotAnError(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	cmdRepo.Upsert(context.Background(), &library.Collection{ID: "col-1", Name: "Ma liste"})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-1", "m1", 0)

	if _, err := h.RemoveCollectionItem(adminCtx(), &eveyv1.RemoveCollectionItemRequest{Id: "col-1", MediaItemId: "never-there"}); err != nil {
		t.Fatalf("removing an absent member must succeed: %v", err)
	}
}

func TestCollectionRemoveCollectionItem_Validation(t *testing.T) {
	h, _, _, _ := newTestCollectionHandlerWithAccess()
	if _, err := h.RemoveCollectionItem(adminCtx(), &eveyv1.RemoveCollectionItemRequest{MediaItemId: "m1"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("missing id: want InvalidArgument, got %v", err)
	}
	if _, err := h.RemoveCollectionItem(adminCtx(), &eveyv1.RemoveCollectionItemRequest{Id: "col-1"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("missing media_item_id: want InvalidArgument, got %v", err)
	}
}

// IDOR: a collection the caller cannot see must stay indistinguishable from
// one that does not exist, and its members must stay untouched.
func TestCollectionRemoveCollectionItem_IDOR_DeniesForeignCollection(t *testing.T) {
	h, cmdRepo, mediaRepo, libAccessRepo := newTestCollectionHandlerWithAccess()
	cmdRepo.Upsert(context.Background(), &library.Collection{ID: "col-lib1", Name: "Private"})
	mediaRepo.Create(context.Background(), &library.MediaItem{ID: "m1", LibraryID: "lib-1", Title: "A", Type: "movie"})
	cmdRepo.LinkItem(context.Background(), "col-lib1", "m1", 0)
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2")

	_, err := h.RemoveCollectionItem(nonAdminCtx("user-1"), &eveyv1.RemoveCollectionItemRequest{Id: "col-lib1", MediaItemId: "m1"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
	items, _, _ := h.svc.GetItems(context.Background(), "col-lib1", 100, 0)
	if len(items) != 1 {
		t.Errorf("a denied removal must not touch the collection, got %d items", len(items))
	}
}

// Reordering writes the hand-set position; it is what a "manual" collection
// reads by, and the only way to express a chronology that differs from the
// release order (prequels).
func TestCollectionReorderCollectionItems_OK(t *testing.T) {
	h, cmdRepo, mediaRepo, _ := newTestCollectionHandlerWithAccess()
	cmdRepo.Upsert(context.Background(), &library.Collection{ID: "col-1", Name: "Star Wars chrono", SortBy: "manual"})
	for _, id := range []string{"m1", "m2", "m3"} {
		mediaRepo.Create(context.Background(), &library.MediaItem{ID: shared.MediaItemID(id), LibraryID: "lib-1", Title: id, Type: "movie"})
		cmdRepo.LinkItem(context.Background(), "col-1", id, 0)
	}
	if _, err := h.ReorderCollectionItems(adminCtx(), &eveyv1.ReorderCollectionItemsRequest{
		Id: "col-1", MediaItemIds: []string{"m3", "m1", "m2"},
	}); err != nil {
		t.Fatalf("ReorderCollectionItems: %v", err)
	}
}

func TestCollectionReorderCollectionItems_Validation(t *testing.T) {
	h, _, _, _ := newTestCollectionHandlerWithAccess()
	if _, err := h.ReorderCollectionItems(adminCtx(), &eveyv1.ReorderCollectionItemsRequest{MediaItemIds: []string{"m1"}}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("missing id: want InvalidArgument, got %v", err)
	}
	if _, err := h.ReorderCollectionItems(adminCtx(), &eveyv1.ReorderCollectionItemsRequest{Id: "col-1"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty order: want InvalidArgument, got %v", err)
	}
	tooMany := make([]string, library.MaxCollectionItems+1)
	for i := range tooMany {
		tooMany[i] = "m"
	}
	if _, err := h.ReorderCollectionItems(adminCtx(), &eveyv1.ReorderCollectionItemsRequest{Id: "col-1", MediaItemIds: tooMany}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("oversized order: want InvalidArgument, got %v", err)
	}
}
