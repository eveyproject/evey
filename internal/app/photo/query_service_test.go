package photo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	appphoto "github.com/eveyproject/evey/internal/app/photo"
	"github.com/eveyproject/evey/internal/domain/photo"
	photofakes "github.com/eveyproject/evey/internal/domain/photo/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
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

// Every photo list takes the client's page size within a 5000 ceiling: absent
// or oversized, it falls back to 200 instead of loading the whole library.
func TestQueryService_PageSizeIsClamped(t *testing.T) {
	repo := photofakes.NewFakeRepository()
	for i := range 5100 {
		seedAlbum(repo, fmt.Sprintf("p-%05d", i), "lib-1", "Vacation")
	}
	svc := appphoto.NewQueryService(repo)
	ctx := context.Background()
	lib := shared.LibraryID("lib-1")

	lists := map[string]func(limit int) (int, error){
		"ListPhotos": func(limit int) (int, error) {
			p, _, err := svc.ListPhotos(ctx, lib, limit, 0)
			return len(p), err
		},
		"ListPhotosByAlbum": func(limit int) (int, error) {
			p, _, err := svc.ListPhotosByAlbum(ctx, lib, "Vacation", limit, 0)
			return len(p), err
		},
		"ListPhotosCursor": func(limit int) (int, error) {
			p, err := svc.ListPhotosCursor(ctx, lib, nil, limit)
			return len(p), err
		},
		"ListPhotosByAlbumCursor": func(limit int) (int, error) {
			p, err := svc.ListPhotosByAlbumCursor(ctx, lib, "Vacation", nil, limit)
			return len(p), err
		},
	}
	for name, list := range lists {
		for _, c := range []struct{ limit, want int }{{0, 200}, {5000, 5000}, {5001, 200}} {
			got, err := list(c.limit)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if got != c.want {
				t.Errorf("%s(limit=%d) = %d photos, want %d", name, c.limit, got, c.want)
			}
		}
	}
}
