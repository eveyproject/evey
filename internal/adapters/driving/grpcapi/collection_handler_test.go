package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	appcollection "github.com/gravity-zero/evey/internal/app/collection"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/domain/library"
	libraryfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
	userfakes "github.com/gravity-zero/evey/internal/domain/user/fakes"
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
