package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appcollection "github.com/evey-project/evey/internal/app/collection"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libraryfakes "github.com/evey-project/evey/internal/domain/library/fakes"
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
	return &collectionHandler{svc: svc}, cmdRepo
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
	h, cmdRepo := newTestCollectionHandler()
	seedCollection(cmdRepo, "col-1", "Marvel Cinematic Universe")
	seedCollection(cmdRepo, "col-2", "DC Extended Universe")

	resp, err := h.ListCollections(context.Background(), &eveyv1.ListCollectionsRequest{})
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

	resp, err := h.ListCollections(nonAdminCtx("user-1"), &eveyv1.ListCollectionsRequest{})
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Collections) != 1 || resp.Collections[0].Id != "col-lib2" {
		t.Errorf("expected only col-lib2, got %+v", resp.Collections)
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
	list, err := h.ListCollections(context.Background(), &eveyv1.ListCollectionsRequest{})
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
