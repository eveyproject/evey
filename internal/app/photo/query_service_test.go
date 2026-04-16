package photo_test

import (
	"context"
	"errors"
	"testing"

	appphoto "github.com/evey-project/evey/internal/app/photo"
	"github.com/evey-project/evey/internal/domain/photo"
	photofakes "github.com/evey-project/evey/internal/domain/photo/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
)

func seedAlbum(repo *photofakes.FakeRepository, id, libraryID, album string) {
	_ = repo.Upsert(context.Background(), photo.Photo{
		ID:        id,
		LibraryID: shared.LibraryID(libraryID),
		Album:     album,
		FilePath:  "/photos/" + id + ".jpg",
	})
}

func TestQueryService_ListPhotosByAlbum_HappyPath(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	seedAlbum(repo, "p-1", "lib-1", "Vacation")
	seedAlbum(repo, "p-2", "lib-1", "Vacation")
	seedAlbum(repo, "p-3", "lib-1", "Wedding")

	svc := appphoto.NewQueryService(repo)
	photos, total, err := svc.ListPhotosByAlbum(context.Background(), shared.LibraryID("lib-1"), "Vacation", 50, 0)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(photos) != 2 {
		t.Errorf("len(photos) = %d, want 2", len(photos))
	}
	for _, p := range photos {
		if p.Album != "Vacation" {
			t.Errorf("unexpected album: %s", p.Album)
		}
	}
}

func TestQueryService_ListPhotosByAlbum_EmptyResult(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	seedAlbum(repo, "p-1", "lib-1", "Vacation")

	svc := appphoto.NewQueryService(repo)
	photos, total, err := svc.ListPhotosByAlbum(context.Background(), shared.LibraryID("lib-1"), "NonExistent", 50, 0)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(photos) != 0 {
		t.Errorf("len(photos) = %d, want 0", len(photos))
	}
}

func TestQueryService_ListPhotosByAlbum_LimitClamped(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	for _, id := range []string{"p-1", "p-2", "p-3"} {
		seedAlbum(repo, id, "lib-1", "Vacation")
	}

	svc := appphoto.NewQueryService(repo)
	// limit <= 0 should default to 200 (not error)
	photos, total, err := svc.ListPhotosByAlbum(context.Background(), shared.LibraryID("lib-1"), "Vacation", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(photos) != 3 {
		t.Errorf("len(photos) = %d, want 3", len(photos))
	}

	// limit > 5000 should also be clamped (still returns everything available)
	photos, total, err = svc.ListPhotosByAlbum(context.Background(), shared.LibraryID("lib-1"), "Vacation", 10000, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("total after clamp = %d, want 3", total)
	}
	if len(photos) != 3 {
		t.Errorf("len(photos) after clamp = %d, want 3", len(photos))
	}
}

func TestQueryService_ListPhotosByAlbum_RepoError(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	repo.Err = errors.New("db down")

	svc := appphoto.NewQueryService(repo)
	_, _, err := svc.ListPhotosByAlbum(context.Background(), shared.LibraryID("lib-1"), "Vacation", 50, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
