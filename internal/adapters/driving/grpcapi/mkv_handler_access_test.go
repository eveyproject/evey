package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmkv "github.com/evey-project/evey/internal/app/mkv"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	libraryfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	mkvfakes "github.com/evey-project/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newTestMKVHandlerWithAccess builds a handler wired with the path->item->library
// access check, plus a reader/media-query/lib-access trio seeded with one MKV at
// /media/movie.mkv belonging to lib-1. Mirrors the production wiring in server.go.
func newTestMKVHandlerWithAccess() (*mkvHandler, *userfakes.FakeLibraryAccessRepository) {
	reader := mkvfakes.NewFakeContainerReader()
	seedContainer(reader)
	svc := appmkv.NewService(reader)

	mediaQuery := libraryfakes.NewFakeMediaItemQueryRepo()
	mediaQuery.Seed(&library.MediaItem{
		ID:        "m1",
		LibraryID: "lib-1",
		Title:     "Movie",
		Type:      "movie",
		File:      &library.MediaFile{Path: "/media/movie.mkv"},
	})
	libQuery := applibrary.NewQueryService(
		libraryfakes.NewFakeLibraryQueryRepo(),
		mediaQuery,
		libraryfakes.NewFakeMediaStreamRepository(),
		libraryfakes.NewFakeSearchIndex(),
	)

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

	h := &mkvHandler{svc: svc, libQuery: libQuery, userAccess: userAccess}
	return h, libAccessRepo
}

// IDOR: a non-admin without access to the media's library must not be able to
// read its MKV chapters, even though ListChapters is not admin-gated at the RBAC
// layer (the player's skip-intro fallback uses it for regular users).
func TestMKVListChapters_IDOR_DeniesUnauthorizedLibrary(t *testing.T) {
	h, libAccessRepo := newTestMKVHandlerWithAccess()
	libAccessRepo.Grant(context.Background(), "user-1", "lib-2") // not lib-1

	_, err := h.ListChapters(nonAdminCtx("user-1"), &eveyv1.ListChaptersRequest{Path: "/media/movie.mkv"})
	if err == nil {
		t.Fatal("expected NotFound for chapters in an unauthorized library")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("expected NotFound, got %v", got)
	}
}

// A non-admin WITH access to the media's library reads chapters normally.
func TestMKVListChapters_IDOR_AllowsAuthorizedLibrary(t *testing.T) {
	h, libAccessRepo := newTestMKVHandlerWithAccess()
	libAccessRepo.Grant(context.Background(), "user-1", "lib-1")

	resp, err := h.ListChapters(nonAdminCtx("user-1"), &eveyv1.ListChaptersRequest{Path: "/media/movie.mkv"})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	if len(resp.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(resp.Chapters))
	}
}

// A path that resolves to no media item is denied for non-admins: this blocks
// reading chapter metadata of arbitrary MKV files anywhere on disk.
func TestMKVListChapters_IDOR_DeniesUnknownPath(t *testing.T) {
	h, libAccessRepo := newTestMKVHandlerWithAccess()
	libAccessRepo.Grant(context.Background(), "user-1", "lib-1")

	_, err := h.ListChapters(nonAdminCtx("user-1"), &eveyv1.ListChaptersRequest{Path: "/etc/secret.mkv"})
	if err == nil {
		t.Fatal("expected NotFound for a path with no matching media item")
	}
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("expected NotFound, got %v", got)
	}
}

// Admins bypass the access check (no library grants needed).
func TestMKVListChapters_AdminBypassesAccessCheck(t *testing.T) {
	h, _ := newTestMKVHandlerWithAccess()

	resp, err := h.ListChapters(adminCtx(), &eveyv1.ListChaptersRequest{Path: "/media/movie.mkv"})
	if err != nil {
		t.Fatalf("ListChapters (admin): %v", err)
	}
	if len(resp.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(resp.Chapters))
	}
}
